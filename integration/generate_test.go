package integration_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-cf/greenhouse-install-script-generator/models"
)

var _ = Describe("Generate", func() {
	var server *ghttp.Server
	var session *gexec.Session
	var outputDir string
	var manifest string
	var generateCommand *exec.Cmd
	var generatePath string

	BeforeEach(func() {
		server = ghttp.NewServer()
		var err error
		generatePath, err = gexec.Build("github.com/pivotal-cf/greenhouse-install-script-generator/generate")
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		deployments := []models.IndexDeployment{
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

		yaml, err := ioutil.ReadFile(manifest)
		Expect(err).ToNot(HaveOccurred())
		diegoDeployment := models.ShowDeployment{
			Manifest: string(yaml),
		}

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

		session, err = gexec.Start(generateCommand, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {
		manifest = "one_zone_manifest.yml"
	})

	AfterEach(func() {
		Expect(os.RemoveAll(outputDir)).To(Succeed())
	})

	Context("when ran without params", func() {
		BeforeEach(func() {
			generateCommand = exec.Command(generatePath)
		})

		It("prints an error message", func() {
			Eventually(session).Should(gexec.Exit(1))
			Expect(session.Err).Should(gbytes.Say("Usage of generate:"))
		})
	})

	Context("when all required params are given", func() {
		BeforeEach(func() {
			var err error
			outputDir, err = ioutil.TempDir("", "XXXXXXX")
			Expect(err).NotTo(HaveOccurred())

			generateCommand = exec.Command(generatePath,
				"-boshUrl", server.URL(),
				"-outputDir", outputDir,
				"-windowsUsername", "admin",
				"-windowsPassword", "password",
			)
		})

		JustBeforeEach(func() {
			Eventually(session).Should(gexec.Exit(0))
		})

		It("sends get requests to get the deployments", func() {
			Expect(server.ReceivedRequests()).To(HaveLen(2))
		})

		Context("when one deployment is returned with CF+diego", func() {
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

			Describe("the lines of the batch script", func() {
				var lines []string
				var script string

				JustBeforeEach(func() {
					content, err := ioutil.ReadFile(path.Join(outputDir, "install_zone1.bat"))
					Expect(err).NotTo(HaveOccurred())
					script = strings.TrimSpace(string(content))
					lines = strings.Split(string(script), "\r\n")
				})

				It("contains all the MSI parameters", func() {
					expectedContent := `msiexec /norestart /i %~dp0\diego.msi ^
  ADMIN_USERNAME=admin ^
  ADMIN_PASSWORD=password ^
  CONSUL_IPS=consul1.foo.bar ^
  CF_ETCD_CLUSTER=http://etcd1.foo.bar:4001 ^
  STACK=windows2012R2 ^
  REDUNDANCY_ZONE=zone1 ^
  LOGGREGATOR_SHARED_SECRET=secret123 ^
  ETCD_CA_FILE=%~dp0\ca.crt ^
  ETCD_CERT_FILE=%~dp0\client.crt ^
  ETCD_KEY_FILE=%~dp0\client.key`
					expectedContent = strings.Replace(expectedContent, "\n", "\r\n", -1)
					Expect(script).To(Equal(expectedContent))
				})
			})

			Context("when there is one redundancy zone", func() {
				It("generates only one file", func() {
					matches, err := filepath.Glob(path.Join(outputDir, "install_*.bat"))
					Expect(err).NotTo(HaveOccurred())
					Expect(matches).To(HaveLen(1))
					Expect(path.Join(outputDir, "install_zone1.bat")).To(BeAnExistingFile())
				})
			})

			Context("when there is more than one redundancy zone", func() {
				BeforeEach(func() {
					manifest = "two_zone_manifest.yml"
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
	})
})
