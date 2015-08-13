package integration_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry-incubator/greenhouse-install-script-generator/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
)

func DefaultServer() *ghttp.Server {
	return CreateServer("one_zone_manifest.yml", DefaultIndexDeployment())
}

func CreateServer(manifest string, deployments []models.IndexDeployment) *ghttp.Server {
	yaml, err := ioutil.ReadFile(manifest)
	Expect(err).ToNot(HaveOccurred())

	diegoDeployment := models.ShowDeployment{
		Manifest: string(yaml),
	}

	server := ghttp.NewServer()
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/deployments"),
			ghttp.RespondWithJSONEncoded(200, deployments),
		),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/deployments/cf-warden-diego"),
			ghttp.RespondWithJSONEncoded(200, diegoDeployment),
		),
	)

	return server
}

func Create401Server() *ghttp.Server {
	server := ghttp.NewServer()
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/deployments"),
			ghttp.RespondWith(401, "Not authorized"),
		),
	)

	return server
}

func StartGeneratorWithURL(serverUrl string) (*gexec.Session, string) {
	var err error
	outputDir, err := ioutil.TempDir("", "XXXXXXX")
	Expect(err).NotTo(HaveOccurred())

	return StartGeneratorWithArgs(
		"-boshUrl", serverUrl,
		"-outputDir", outputDir,
		"-windowsUsername", "admin",
		"-windowsPassword", "password",
	), outputDir
}

func StartGeneratorWithArgs(args ...string) *gexec.Session {
	generatePath, err := gexec.Build("github.com/cloudfoundry-incubator/greenhouse-install-script-generator/generate")
	Expect(err).NotTo(HaveOccurred())
	command := exec.Command(generatePath, args...)
	session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	return session
}

func DefaultIndexDeployment() []models.IndexDeployment {
	return []models.IndexDeployment{
		{
			Name: "cf-warden",
			Releases: []models.Release{
				{
					Name:    "cf",
					Version: "213+dev.2",
				},
			},
		},
		{
			Name: "cf-warden-diego",
			Releases: []models.Release{
				{
					Name:    "cf",
					Version: "213+dev.2",
				},
				{
					Name:    "diego",
					Version: "0.1366.0+dev.2",
				},
			},
		},
	}
}

func AmbiguousIndexDeployment() []models.IndexDeployment {
	return []models.IndexDeployment{
		{
			Name: "cf-warden",
			Releases: []models.Release{
				{
					Name:    "cf",
					Version: "213+dev.2",
				},
			},
		},
		{
			Name: "cf-warden-diego",
			Releases: []models.Release{
				{
					Name:    "cf",
					Version: "213+dev.2",
				},
				{
					Name:    "diego",
					Version: "0.1366.0+dev.2",
				},
			},
		},
		{
			Name: "cf-warden-diego-2",
			Releases: []models.Release{
				{
					Name:    "cf",
					Version: "213+dev.2",
				},
				{
					Name:    "diego",
					Version: "0.1366.0+dev.2",
				},
			},
		},
	}
}

var _ = Describe("Generate", func() {
	var outputDir string

	AfterEach(func() {
		Expect(os.RemoveAll(outputDir)).To(Succeed())
	})

	Describe("Success scenarios", func() {
		Context("when the server returns a one zone manifest, this includes syslog", func() {
			var server *ghttp.Server

			BeforeEach(func() {
				server = CreateServer("one_zone_manifest.yml", DefaultIndexDeployment())
				var session *gexec.Session
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(-1))
			})

			It("sends get requests to get the deployments", func() {
				Expect(server.ReceivedRequests()).To(HaveLen(2))
			})

			It("generates the certificate authority cert", func() {
				cert, err := ioutil.ReadFile(path.Join(outputDir, "ca.crt"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cert).To(BeEquivalentTo("CA_CERT"))
			})

			It("generates the client cert", func() {
				cert, err := ioutil.ReadFile(path.Join(outputDir, "client.crt"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cert).To(BeEquivalentTo("CLIENT_CERT"))
			})

			It("generates the client key", func() {
				cert, err := ioutil.ReadFile(path.Join(outputDir, "client.key"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cert).To(BeEquivalentTo("CLIENT_KEY"))
			})

			It("generates only one file", func() {
				matches, err := filepath.Glob(path.Join(outputDir, "install_*.bat"))
				Expect(err).NotTo(HaveOccurred())
				Expect(matches).To(HaveLen(1))
				Expect(path.Join(outputDir, "install_zone1.bat")).To(BeAnExistingFile())
			})

			Describe("the lines of the batch script", func() {
				var lines []string
				var script string

				BeforeEach(func() {
					content, err := ioutil.ReadFile(path.Join(outputDir, "install_zone1.bat"))
					Expect(err).NotTo(HaveOccurred())
					script = strings.TrimSpace(string(content))
					lines = strings.Split(string(script), "\r\n")
				})

				It("contains all the MSI parameters", func() {
					expectedContent := `msiexec /passive /norestart /i %~dp0\diego.msi ^
  ADMIN_USERNAME=admin ^
  ADMIN_PASSWORD=password ^
  CONSUL_IPS=consul1.foo.bar ^
  CF_ETCD_CLUSTER=http://etcd1.foo.bar:4001 ^
  STACK=windows2012R2 ^
  REDUNDANCY_ZONE=zone1 ^
  LOGGREGATOR_SHARED_SECRET=secret123 ^
  SYSLOG_HOST_IP=logs2.test.com ^
  SYSLOG_PORT=11111 ^
  ETCD_CA_FILE=%~dp0\ca.crt ^
  ETCD_CERT_FILE=%~dp0\client.crt ^
  ETCD_KEY_FILE=%~dp0\client.key`
					expectedContent = strings.Replace(expectedContent, "\n", "\r\n", -1)
					Expect(script).To(Equal(expectedContent))
				})
			})
		})

		Context("when the server returns a two zone manifest, this does not include syslog", func() {
			var server *ghttp.Server

			BeforeEach(func() {
				server = CreateServer("two_zone_manifest.yml", DefaultIndexDeployment())
				var session *gexec.Session
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(0))
			})

			It("generates one file per zone", func() {
				matches, err := filepath.Glob(path.Join(outputDir, "install_*.bat"))
				Expect(err).NotTo(HaveOccurred())
				Expect(matches).To(HaveLen(2))
				Expect(path.Join(outputDir, "install_zone1.bat")).To(BeAnExistingFile())
				Expect(path.Join(outputDir, "install_zone2.bat")).To(BeAnExistingFile())
			})
		})
	})

	Describe("Failure scenarios", func() {
		Context("when ran without params", func() {
			var session *gexec.Session
			BeforeEach(func() {
				session = StartGeneratorWithArgs()
			})

			It("prints an error message", func() {
				Eventually(session).Should(gexec.Exit(1))
				Expect(session.Err).Should(gbytes.Say("Usage of generate:"))
			})

			Context("when the server is not reachable", func() {
				var session *gexec.Session

				BeforeEach(func() {
					session, outputDir = StartGeneratorWithURL("http://1.2.3.4:5555")
					Eventually(session, "15s", "1s").Should(gexec.Exit(1))
				})

				It("displays the reponse error to the user", func() {
					Expect(session.Err).Should(gbytes.Say("Unable to establish connection to BOSH Director"))
				})
			})

			Context("when the server returns an unauthorized error", func() {
				var server *ghttp.Server
				var session *gexec.Session

				BeforeEach(func() {
					server = Create401Server()
					session, outputDir = StartGeneratorWithURL(server.URL())
					Eventually(session).Should(gexec.Exit(1))
				})

				It("displays the reponse error to the user", func() {
					Expect(session.Err).Should(gbytes.Say("Not authorized"))
				})
			})

			Context("when the server returns an ambiguous number of deployments", func() {
				var server *ghttp.Server
				var session *gexec.Session

				BeforeEach(func() {
					server = CreateServer("one_zone_manifest.yml", AmbiguousIndexDeployment())
					session, outputDir = StartGeneratorWithURL(server.URL())
					Eventually(session).Should(gexec.Exit(1))
				})

				It("displays the reponse error to the user", func() {
					Expect(session.Err).Should(gbytes.Say("BOSH Director does not have exactly one deployment containing a cf and diego release."))
				})
			})
		})

		Context("when ran with an ouputDir param that points to a dir that doesn't exist", func() {
			var session *gexec.Session
			var nonExistingDir string
			BeforeEach(func() {

				outputDir, err := ioutil.TempDir("", "XXXXXXX")
				nonExistingDir = path.Join(outputDir, "does_not_exist")
				Expect(err).NotTo(HaveOccurred())
				server := DefaultServer()
				session = StartGeneratorWithArgs(
					"-boshUrl", server.URL(),
					"-outputDir", nonExistingDir,
					"-windowsUsername", "admin",
					"-windowsPassword", "password",
				)
			})

			It("creates the directory", func() {
				Eventually(session).Should(gexec.Exit(0))
				_, err := os.Stat(nonExistingDir)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the deployment has no certs", func() {
			var session *gexec.Session

			BeforeEach(func() {
				server := CreateServer("no_cert_manifest.yml", DefaultIndexDeployment())
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(1))
			})

			It("displays the reponse error to the user", func() {
				Expect(session.Err).Should(gbytes.Say("Failed to extract cert from deployment"))
			})
		})
	})
})
