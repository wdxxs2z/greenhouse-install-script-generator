package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/cloudfoundry-incubator/candiedyaml"
	"github.com/cloudfoundry-incubator/greenhouse-install-script-generator/models"
)

const (
	installBatTemplate = `msiexec /passive /norestart /i %~dp0\diego.msi ^
  ADMIN_USERNAME={{.Username}} ^
  ADMIN_PASSWORD={{.Password}} ^
  CONSUL_IPS={{.ConsulIPs}} ^
  CF_ETCD_CLUSTER=http://{{.EtcdCluster}}:4001 ^
  STACK=windows2012R2 ^
  REDUNDANCY_ZONE={{.Zone}} ^
  LOGGREGATOR_SHARED_SECRET={{.SharedSecret}} ^{{ if .SyslogHostIP }}
  SYSLOG_HOST_IP={{.SyslogHostIP}} ^
  SYSLOG_PORT={{.SyslogPort}} ^{{ end }}
  CONSUL_ENCRYPT_FILE=%~dp0\consul_encrypt.key ^
  CONSUL_CA_FILE=%~dp0\consul_ca.crt ^
  CONSUL_AGENT_CERT_FILE=%~dp0\consul_agent.crt ^
  CONSUL_AGENT_KEY_FILE=%~dp0\consul_agent.key
  `
)

type InstallerArguments struct {
	ConsulIPs    string
	EtcdCluster  string
	Zone         string
	SharedSecret string
	Username     string
	Password     string
	SyslogHostIP string
	SyslogPort   string
}

func main() {
	boshServerUrl := flag.String("boshUrl", "", "Bosh URL (https://admin:admin@bosh.example:25555)")
	outputDir := flag.String("outputDir", "", "Output directory (/tmp/scripts)")
	windowsUsername := flag.String("windowsUsername", "", "Windows username")
	windowsPassword := flag.String("windowsPassword", "", "Windows password")
	awsSubnet := flag.String("awsSubnet", "", "(optional) AWS Subnet")

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

	*windowsUsername = EscapeSpecialCharacters(*windowsUsername)
	*windowsPassword = EscapeSpecialCharacters(*windowsPassword)

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
	var manifest interface{}
	candiedyaml.NewDecoder(buf).Decode(&manifest)

	for key, filename := range map[string]string{
		"properties.consul.agent_cert":     "consul_agent.crt",
		"properties.consul.agent_key":      "consul_agent.key",
		"properties.consul.ca_cert":        "consul_ca.crt",
		"properties.consul.encrypt_keys.0": "consul_encrypt.key",
	} {
		err = extractCert(manifest, *outputDir, filename, key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v", err)
			os.Exit(1)
		}
	}

	result, err := GetIn(manifest, "jobs")
	FailOnError(err)
	jobs := result.([]interface{})
	var repJobs []interface{}

	for _, job := range jobs {
		jopHashRep, err := GetIn(job, "properties", "diego", "rep")
		FailOnError(err)
		if jopHashRep != nil {
			repJobs = append(repJobs, job)
		}
	}

	zones := map[string]struct{}{}
	for _, job := range repJobs {
		zone, err := GetIn(job, "properties", "diego", "rep", "zone")
		FailOnError(err)

		if zone != nil {
			zones[zone.(string)] = struct{}{}
		}
	}

	if *awsSubnet != "" {
		networks, err := GetIn(manifest, "networks")
		FailOnError(err)
		subnetNetworkName := getSubnetNetworkName(networks.([]interface{}), *awsSubnet)
		subnetNetworkZone := getSubnetNetworkZone(repJobs, subnetNetworkName)

		if subnetNetworkZone == "" {
			fmt.Fprintf(os.Stderr, "Failed to find zone for subnet: %v", *awsSubnet)
			os.Exit(1)
		}

		generateInstallScriptWrapperForZone(*outputDir, subnetNetworkZone)
	}

	consuls, err := GetIn(manifest, "properties", "consul", "agent", "servers", "lan")
	FailOnError(err)
	consulIPs := []string{}
	for _, c := range consuls.([]interface{}) {
		consulIPs = append(consulIPs, c.(string))
	}
	joinedConsulIPs := strings.Join(consulIPs, ",")
	result, err = GetIn(manifest, "properties", "etcd", "machines", 0)
	FailOnError(err)
	etcdCluster := result.(string)
	result, err = GetIn(manifest, "properties", "loggregator_endpoint", "shared_secret")
	FailOnError(err)
	sharedSecret := result.(string)
	result, err = GetIn(manifest, "properties", "syslog_daemon_config", "address")
	FailOnError(err)
	syslogHostIP, _ := result.(string)
	result, err = GetIn(manifest, "properties", "syslog_daemon_config", "port")
	FailOnError(err)
	syslogPort := fmt.Sprintf("%v", result)

	args := InstallerArguments{
		ConsulIPs:    joinedConsulIPs,
		EtcdCluster:  etcdCluster,
		SharedSecret: sharedSecret,
		Username:     *windowsUsername,
		Password:     *windowsPassword,
		SyslogHostIP: syslogHostIP,
		SyslogPort:   syslogPort,
	}
	for zone, _ := range zones {
		args.Zone = zone
		generateInstallScript(*outputDir, args)
	}
}

func EscapeSpecialCharacters(str string) string {
	specialCharacters := []string{"^", "%", "(", ")", `"`, "<", ">", "&", "!", "|"}
	for _, c := range specialCharacters {
		str = strings.Replace(str, c, "^"+c, -1)
	}
	return str
}

func FailOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func generateInstallScriptWrapperForZone(outputDir, subnetNetworkZone string) {
	file, err := os.OpenFile(path.Join(outputDir, "install.bat"), os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	filenameContents := fmt.Sprintf("%%~dp0\\install_%s.bat", subnetNetworkZone)
	file.WriteString(filenameContents)
}

func getSubnetNetworkName(networks []interface{}, awsSubnet string) string {
	for _, network := range networks {
		networkName := network.(map[interface{}]interface{})["name"]

		result, err := GetIn(network, "subnets")
		FailOnError(err)
		subnets := result.([]interface{})

		for _, subnetProperties := range subnets {
			result, err = GetIn(subnetProperties, "cloud_properties", "subnet")
			FailOnError(err)
			subnet := result.(string)
			if subnet == awsSubnet {
				return networkName.(string)
			}
		}
	}
	return ""
}

func getSubnetNetworkZone(repJobs []interface{}, subnetNetworkName string) string {
	for _, job := range repJobs {
		jobNetworks, err := GetIn(job, "networks")
		FailOnError(err)
		zone, err := GetIn(job, "properties", "diego", "rep", "zone")
		FailOnError(err)
		for _, jobNetwork := range jobNetworks.([]interface{}) {
			networkName := jobNetwork.(map[interface{}]interface{})["name"]
			if networkName == subnetNetworkName {
				return zone.(string)
			}
		}
	}
	return ""
}

func generateInstallScript(outputDir string, args InstallerArguments) {
	content := strings.Replace(installBatTemplate, "\n", "\r\n", -1)
	temp := template.Must(template.New("").Parse(content))

	filename := fmt.Sprintf("install_%s.bat", args.Zone)
	file, err := os.OpenFile(path.Join(outputDir, filename), os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	err = temp.Execute(file, args)
	if err != nil {
		log.Fatal(err)
	}
}

func extractCert(manifest interface{}, outputDir, filename, pathString string) error {
	manifestPath := []interface{}{}
	for _, s := range strings.Split(pathString, ".") {
		manifestPath = append(manifestPath, s)
	}
	result, err := GetIn(manifest, manifestPath...)
	FailOnError(err)
	if result == nil {
		return errors.New("Failed to extract cert from deployment: " + pathString)
	}
	cert := result.(string)
	ioutil.WriteFile(path.Join(outputDir, filename), []byte(cert), 0644)
	return nil
}

func GetDiegoDeployment(deployments []models.IndexDeployment) int {
	deploymentIndex := -1

	for i, deployment := range deployments {
		releases := map[string]bool{}
		for _, rel := range deployment.Releases {
			releases[rel.Name] = true
		}

		if releases["cf"] && releases["diego"] {
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

// Similar to https://clojuredocs.org/clojure.core/get-in
//
// if the path element is a string we assume obj is a map,
// otherwise if it's an integer we assume obj is a slice
//
// Example:
//    GetIn(obj, []string{"consul", "agent", ...})
func GetIn(obj interface{}, path ...interface{}) (interface{}, error) {
	if len(path) == 0 {
		return obj, nil
	}

	switch x := obj.(type) {
	case map[interface{}]interface{}:
		obj = x[path[0]]
		if obj == nil {
			return nil, nil
		}
	case []interface{}:
		var index int
		var err error
		index, ok := path[0].(int)
		if !ok {
			index, err = strconv.Atoi(path[0].(string))
			if err != nil {
				return nil, err
			}
		}

		obj = x[index]
		if obj == nil {
			return nil, nil
		}
	default:
		return nil, nil
	}

	return GetIn(obj, path[1:]...)
}
