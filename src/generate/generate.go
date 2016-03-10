package main

import (
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"text/template"
	"time"

	"golang.org/x/crypto/pbkdf2"
	"gopkg.in/yaml.v2"

	"models"
)

const (
	installShTemplate = `NATS_ADDRESS={{.NatsIPs}}
NATS_USERNAME={{.NatsUser}}
NATS_PASSWORD={{.NatsPassword}}
ETCD_SERVER={{.EtcdCluster}}
CONSUL_AGENT_ENCRYPT={{.ConsulEncrypt}}
CONSUL_AGENT_CRT="{{.ConsulAgentCert}}"
CONSUL_AGENT_KEY="{{.ConsulAgentKey}}"
CONSUL_CA_CRT="{{.ConsulCaCert}}"
BBS_CA_CRT="{{.BBSCACert}}"
BBS_CLIENT_CRT="{{.BBSClientCert}}"
BBS_CLIENT_KEY="{{.BBSClientKey}}"
LOGGREGATOR_SHARED_SECRET={{.SharedSecret}}


echo $CONSUL_AGENT_CRT > /var/vcap/jobs/consul_agent/config/certs/agent.crt
echo $CONSUL_AGENT_KEY > /var/vcap/jobs/consul_agent/config/certs/agent.key
echo $CONSUL_CA_CRT > /var/vcap/jobs/consul_agent/config/certs/ca.crt
echo $BBS_CA_CRT > /var/vcap/jobs/receptor/config/certs/bbs/ca.crt
echo $BBS_CLIENT_CRT > /var/vcap/jobs/receptor/config/certs/bbs/client.crt
echo $BBS_CLIENT_KEY > /var/vcap/jobs/receptor/config/certs/bbs/client.key

IP_ADDRESS=${IP_ADDRESS:-` + "`ip addr | grep 'inet .*global' | cut -f 6 -d ' ' | cut -f1 -d '/' | head -n 1`" + `}
sed -i "s/10.10.130.104/{{.ConsulIPs}}/g" /var/vcap/jobs/consul_agent/config/config.json
sed -i "s/10.10.30.120/$IP_ADDRESS/g" /var/vcap/jobs/consul_agent/config/config.json
sed -i "s/10.10.130.104/{{.ConsulIPs}}/g" /var/vcap/jobs/consul_agent/bin/agent_ctl
sed -i "s/10.10.130.120/$IP_ADDRESS/g" /var/vcap/jobs/consul_agent/bin/agent_ctl
sed -i "s/10.10.130.120/$IP_ADDRESS/g" /var/vcap/jobs/metron_agent/config/syslog_forwarder.conf
sed -i "s/10.10.130.105/{{.EtcdCluster}}/g" /var/vcap/jobs/metron_agent/config/metron_agent.json
sed -i "s/10.10.130.103/$NATS_ADDRESS/g" /var/vcap/jobs/receptor/bin/receptor_ctl
sed -i "s/-natsUsername=nats/-natsUsername=$NATS_USERNAME/g" /var/vcap/jobs/receptor/bin/receptor_ctl
sed -i "s/-natsPassword=b6945a6105c1cf5ce66b/-natsPassword=$NATS_PASSWORD/g" /var/vcap/jobs/receptor/bin/receptor_ctl
sed -i "s/0mTX0RfWX0zxgUVnMimkPw==/$CONSUL_AGENT_ENCRYPT/g" /var/vcap/jobs/consul_agent/config/config.json
sed -i "s/c12f13df0b192bb19980/$LOGGREGATOR_SHARED_SECRET/g" /var/vcap/jobs/metron_agent/config/metron_agent.json

mkdir -p /var/vcap/sys/log/{consul_agent,metron_agent,monit,receptor,registry}
mkdir -p /var/vcap/sys/run/{consul_agent,metron_agent,receptor,registry}
mkdir -p /var/vcap/data/registry

chown root:root /var/vcap/bosh/bin/monit
chown root:root /var/vcap/bosh/etc/monitrc
chmod 0700 /var/vcap/bosh/etc/monitrc

chmod +x /var/vcap/packages/confab/bin/*
chmod +x /var/vcap/packages/consul/bin/*
chmod -R +x /var/vcap/packages/metron_agent/*
chmod +x /var/vcap/packages/receptor/bin/*
chmod +x /var/vcap/packages/registry/*

chmod +x /var/vcap/jobs/consul_agent/bin/*
chmod +x /var/vcap/jobs/metron_agent/bin/*
chmod +x /var/vcap/jobs/receptor/bin/*
chmod +x /var/vcap/jobs/registry/bin/*

# ADD USER
sudo useradd syslog
sudo usermod -a -G adm syslog
sudo useradd -m vcap

# RUNIT
pushd /tmp
wget http://smarden.org/runit/runit-2.1.2.tar.gz
tar -zxf runit-2.1.2.tar.gz
rm -fr runit-2.1.2.tar.gz
cd admin/runit-2.1.2
sudo ./package/install
sudo cp /usr/local/bin/chpst /sbin/
rm runit-2.1.2.tar.gz
popd

# DOCKER
sudo apt-get update
sudo apt-get install wget
wget -qO- https://get.docker.com/ | sh
sudo usermod -a -G docker vcap
`

)

func main() {
	boshServerUrl := flag.String("boshUrl", "", "Bosh URL (https://admin:admin@bosh.example:25555)")
	outputDir := flag.String("outputDir", "", "Output directory (/tmp/scripts)")

	flag.Parse()
	if *boshServerUrl == "" || *outputDir == "" {
		fmt.Fprintf(os.Stderr, "Usage of generate:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	_, err := os.Stat(*outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(*outputDir, 0755)
		}
	}

	response := NewBoshRequest(*boshServerUrl + "/deployments")
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, err := buf.ReadFrom(response.Body)
		if err != nil {
			fmt.Printf("Could not read response from BOSH director.")
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "Unexpected BOSH director response: %v, %v", response.StatusCode, buf.String())
		os.Exit(1)
	}

	deployments := []models.IndexDeployment{}
	json.NewDecoder(response.Body).Decode(&deployments)
	idx := GetDiegoDeployment(deployments)
	if idx == -1 {
		fmt.Fprintf(os.Stderr, "BOSH Director does not have exactly one deployment containing a cf and diego release.")
		os.Exit(1)
	}

	response = NewBoshRequest(*boshServerUrl + "/deployments/" + deployments[idx].Name)
	defer response.Body.Close()

	deployment := models.ShowDeployment{}
	json.NewDecoder(response.Body).Decode(&deployment)
	buf := bytes.NewBufferString(deployment.Manifest)
	var manifest models.Manifest
	err = yaml.Unmarshal(buf.Bytes(), &manifest)
	if err != nil {
		FailOnError(err)
	}

	args := models.InstallerArguments{}

	fillNats(&args, manifest)
	fillEtcdCluster(&args, manifest)
	fillSharedSecret(&args, manifest)
	fillSyslog(&args, manifest)
	fillConsul(&args, manifest, *outputDir)
	fillBBS(&args, manifest, *outputDir)
	generateInstallShScript(*outputDir, args)
}

func fillSharedSecret(args *models.InstallerArguments, manifest models.Manifest) {
	repJob := firstRepJob(manifest)
	properties := repJob.Properties
	if properties.LoggregatorEndpoint == nil {
		properties = manifest.Properties
	}
	args.SharedSecret = properties.LoggregatorEndpoint.SharedSecret
}

func fillNats(args *models.InstallerArguments, manifest models.Manifest) {
	repJob := firstRepJob(manifest)
	properties := repJob.Properties
	if properties.Nats == nil {
		properties = manifest.Properties	
	}
	args.NatsIPs = strings.Join(properties.Nats.Machines, ",")
	args.NatsUser = properties.Nats.User
	args.NatsPassword = properties.Nats.Password
	args.NatsPort = properties.Nats.Port
}

func fillSyslog(args *models.InstallerArguments, manifest models.Manifest) {
	repJob := firstRepJob(manifest)
	properties := repJob.Properties
	// TODO: this is broken on ops manager:
	//   1. there are no global properties section
	//   2. none of the diego jobs (including rep) has syslog_daemon_config
	if properties.Syslog == nil && manifest.Properties != nil {
		properties = manifest.Properties
	}

	if properties.Syslog == nil {
		return
	}

	args.SyslogHostIP = properties.Syslog.Address
	args.SyslogPort = properties.Syslog.Port
}

func fillBBS(args *models.InstallerArguments, manifest models.Manifest, outputDir string) {
	repJob := firstRepJob(manifest)
	properties := repJob.Properties
	if properties.Diego.Rep.BBS == nil {
		properties = manifest.Properties
	}

	requireSSL := properties.Diego.Rep.BBS.RequireSSL
	// missing requireSSL implies true
	if requireSSL == nil || *requireSSL {
		args.BbsRequireSsl = true
	}

	args.BBSCACert = properties.Diego.Rep.BBS.CACert
	args.BBSClientCert = properties.Diego.Rep.BBS.ClientCert
	args.BBSClientKey = properties.Diego.Rep.BBS.ClientKey
}

func stringToEncryptKey(str string) string {
	decodedStr, err := base64.StdEncoding.DecodeString(str)
	if err == nil && len(decodedStr) == 16 {
		return str
	}

	key := pbkdf2.Key([]byte(str), nil, 20000, 16, sha1.New)
	return base64.StdEncoding.EncodeToString(key)
}

func fillConsul(args *models.InstallerArguments, manifest models.Manifest, outputDir string) {
	repJob := firstRepJob(manifest)
	properties := repJob.Properties
	if properties.Consul == nil {
		properties = manifest.Properties
	}

	// missing requireSSL implies true
	requireSSL := properties.Consul.RequireSSL
	if requireSSL == nil || *requireSSL {
		args.ConsulRequireSSL = true
	}

	consuls := properties.Consul.Agent.Servers.Lan

	args.ConsulIPs = strings.Join(consuls, ",")
	args.ConsulCaCert = properties.Consul.CACert
	args.ConsulAgentCert = properties.Consul.AgentCert
	args.ConsulAgentKey = properties.Consul.AgentKey

	encryptKey := stringToEncryptKey(properties.Consul.EncryptKeys[0])
	args.ConsulEncrypt = encryptKey
}

func fillEtcdCluster(args *models.InstallerArguments, manifest models.Manifest) {
	repJob := firstRepJob(manifest)
	properties := repJob.Properties
	if properties.Loggregator == nil {
		properties = manifest.Properties
	}

	args.EtcdCluster = properties.Loggregator.Etcd.Machines[0]
}

func firstRepJob(manifest models.Manifest) models.Job {
	jobs := manifest.Jobs

	for _, job := range jobs {
		if job.Properties.Diego != nil && job.Properties.Diego.Rep != nil {
			return job
		}

	}
	panic("no rep jobs found")
}

func FailOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func generateInstallShScript(outputDir string, args models.InstallerArguments) {
	//This is windows line, so removed.
        //content := strings.Replace(installShTemplate, "\n", "\r\n", -1)
	temp := template.Must(template.New("").Parse(installShTemplate))
	filename := "install.sh"
	file, err := os.OpenFile(path.Join(outputDir, filename), os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	err = temp.Execute(file, args)
	if err != nil {
		log.Fatal(err)
	}
}

func GetDiegoDeployment(deployments []models.IndexDeployment) int {
	deploymentIndex := -1

	for i, deployment := range deployments {
		releases := map[string]bool{}
		for _, rel := range deployment.Releases {
			releases[rel.Name] = true
		}

		if releases["cf"] && releases["diego"] && releases["garden-linux"] {
			if deploymentIndex != -1 {
				return -1
			}

			deploymentIndex = i
		}

	}

	return deploymentIndex
}

func NewBoshRequest(endpoint string) *http.Response {
	request, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Fatal(err)
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	http.DefaultClient.Timeout = 10 * time.Second
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatalln("Unable to establish connection to BOSH Director.", err)
	}
	return response
}

func validateCredentials(username, password string) {
	pattern := regexp.MustCompile("^[a-zA-Z0-9]+$")
	if !pattern.Match([]byte(password)) {
		log.Fatalln("Invalid windowsPassword, must be alphanumeric")
	}

	if !pattern.Match([]byte(username)) {
		log.Fatalln("Invalid windowsUsername, must be alphanumeric")
	}
}
