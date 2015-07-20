package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/cloudfoundry-incubator/candiedyaml"
	"github.com/pivotal-cf/create-install-bat/models"
)

const (
	installBatTemplate = `msiexec /norestart /i diego.msi ^
  ADMIN_USERNAME=[USERNAME] ^
  ADMIN_PASSWORD=[PASSWORD] ^
  CONSUL_IPS={{.ConsulIPs}} ^
  CF_ETCD_CLUSTER=http://{{.EtcdCluster}}:4001 ^
  STACK=windows2012R2 ^
  REDUNDANCY_ZONE={{.Zone}} ^
  LOGGREGATOR_SHARED_SECRET={{.SharedSecret}} ^
  ETCD_CA_FILE=%cd%\ca.crt ^
  ETCD_CERT_FILE=%cd%\client.crt ^
  ETCD_KEY_FILE=%cd%\client.key
`
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr,
			`missing required parameters. Usage: %s bosh-director-url output-dir

e.g. %s http://bosh.foo.bar.com /tmp/scripts\n`, os.Args[0], os.Args[0])
		os.Exit(1)
	}

	boshServerUrl := os.Args[1]
	outputDir := os.Args[2]
	response := NewBoshRequest(boshServerUrl + "/deployments")
	defer response.Body.Close()
	deployments := []models.IndexDeployment{}
	json.NewDecoder(response.Body).Decode(&deployments)
	idx := GetDiegoDeployment(deployments)

	response = NewBoshRequest(boshServerUrl + "/deployments/" + deployments[idx].Name)
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
		extractCert(manifest, outputDir, k, f)
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

	for zone, _ := range zones {
		generateInstallScript(outputDir, zone, joinedConsulIPs, etcdCluster, sharedSecret)
	}
}

func generateInstallScript(outputDir, zone, consulIPs, etcdCluster, sharedSecret string) {
	content := strings.Replace(installBatTemplate, "\n", "\r\n", -1)
	temp, err := template.New("").Parse(content)
	if err != nil {
		log.Fatal(err)
	}

	filename := fmt.Sprintf("install_%s.bat", zone)
	f, err := os.OpenFile(path.Join(outputDir, filename), os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	data := struct {
		ConsulIPs    string
		EtcdCluster  string
		Zone         string
		SharedSecret string
	}{
		ConsulIPs:    consulIPs,
		EtcdCluster:  etcdCluster,
		Zone:         zone,
		SharedSecret: sharedSecret,
	}
	err = temp.Execute(f, data)
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
	request.SetBasicAuth("admin", "admin")
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
