package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/cloudfoundry-incubator/candiedyaml"
	"github.com/pivotal-cf/greenhouse-install-script-generator/models"
)

const (
	installBatTemplate = `msiexec /norestart /i %~dp0\diego.msi ^
  ADMIN_USERNAME={{.Username}} ^
  ADMIN_PASSWORD={{.Password}} ^
  CONSUL_IPS={{.ConsulIPs}} ^
  CF_ETCD_CLUSTER=http://{{.EtcdCluster}}:4001 ^
  STACK=windows2012R2 ^
  REDUNDANCY_ZONE={{.Zone}} ^
  LOGGREGATOR_SHARED_SECRET={{.SharedSecret}} ^
  ETCD_CA_FILE=%~dp0\ca.crt ^
  ETCD_CERT_FILE=%~dp0\client.crt ^
  ETCD_KEY_FILE=%~dp0\client.key
`
)

type InstallerArguments struct {
	ConsulIPs    string
	EtcdCluster  string
	Zone         string
	SharedSecret string
	Username     string
	Password     string
}

func main() {
	boshServerUrl := flag.String("boshUrl", "", "Bosh URL (https://admin:admin@bosh.example:25555)")
	outputDir := flag.String("outputDir", "", "Output directory (/tmp/scripts)")
	windowsUsername := flag.String("windowsUsername", "", "Windows username")
	windowsPassword := flag.String("windowsPassword", "", "Windows password")

	flag.Parse()
	if *boshServerUrl == "" || *outputDir == "" {
		fmt.Fprintf(os.Stderr, "Usage of generate:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	fmt.Println(*windowsUsername, *windowsPassword)

	response := NewBoshRequest(*boshServerUrl + "/deployments")
	defer response.Body.Close()
	
	if (response.StatusCode != http.StatusOK) {
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

	response = NewBoshRequest(*boshServerUrl + "/deployments/" + deployments[idx].Name)
	defer response.Body.Close()
	
	deployment := models.ShowDeployment{}
	json.NewDecoder(response.Body).Decode(&deployment)
	buf := bytes.NewBufferString(deployment.Manifest)
	var manifest interface{}
	candiedyaml.NewDecoder(buf).Decode(&manifest)

	for k, f := range map[string]string{
		"client_cert": "client.crt",
		"client_key":  "client.key",
		"ca_cert":     "ca.crt",
	} {
		extractCert(manifest, *outputDir, k, f)
	}

	zones := map[string]struct{}{}
	jobs := GetIn(manifest, "jobs").([]interface{})
	for _, job := range jobs {
		zone := GetIn(job, "properties", "diego", "rep", "zone")
		if zone == nil {
			continue
		}
		zones[zone.(string)] = struct{}{}
	}

	consuls := GetIn(manifest, "properties", "consul", "agent", "servers", "lan")
	consulIPs := []string{}
	for _, c := range consuls.([]interface{}) {
		consulIPs = append(consulIPs, c.(string))
	}
	joinedConsulIPs := strings.Join(consulIPs, ",")
	etcdCluster := GetIn(manifest, "properties", "etcd", "machines", 0).(string)
	sharedSecret := GetIn(manifest, "properties", "loggregator_endpoint", "shared_secret").(string)

	args := InstallerArguments{
		ConsulIPs:    joinedConsulIPs,
		EtcdCluster:  etcdCluster,
		SharedSecret: sharedSecret,
		Username:     *windowsUsername,
		Password:     *windowsPassword,
	}
	for zone, _ := range zones {
		args.Zone = zone
		generateInstallScript(*outputDir, args)
	}
}

func generateInstallScript(outputDir string, args InstallerArguments) {
	content := strings.Replace(installBatTemplate, "\n", "\r\n", -1)
	temp, err := template.New("").Parse(content)
	if err != nil {
		log.Fatal(err)
	}

	filename := fmt.Sprintf("install_%s.bat", args.Zone)
	f, err := os.OpenFile(path.Join(outputDir, filename), os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	err = temp.Execute(f, args)
	if err != nil {
		log.Fatal(err)
	}
}

func extractCert(manifest interface{}, outputDir, key, filename string) {
	caCert := GetIn(manifest, "properties", "diego", "etcd", key).(string)
	ioutil.WriteFile(path.Join(outputDir, filename), []byte(caCert), 0644)
}

func GetDiegoDeployment(deployments []models.IndexDeployment) int {
	for i, deployment := range deployments {
		if len(deployment.Releases) != 2 {
			continue
		}

		releases := map[string]bool{}
		for _, rel := range deployment.Releases {
			releases[rel.Name] = true
		}

		if releases["cf"] && releases["diego"] {
			return i
		}

	}

	return -1
}

func NewBoshRequest(endpoint string) *http.Response {
	request, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Fatal(err)
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Fatal(err)
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
func GetIn(obj interface{}, path ...interface{}) interface{} {
	if len(path) == 0 {
		return obj
	}

	switch x := obj.(type) {
	case map[interface{}]interface{}:
		obj = x[path[0]]
		if obj == nil {
			return nil
		}
	case []interface{}:
		obj = x[path[0].(int)]
		if obj == nil {
			return nil
		}
	default:
		return nil
	}

	return GetIn(obj, path[1:]...)
}
