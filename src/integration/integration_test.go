package integration_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"models"
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
	generatePath, err := gexec.Build("generate")
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
				{
					Name:    "garden-linux",
					Version: "0.305.0",
				},
			},
		},
		{
			Name: "diego-vizzini",
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
				{
					Name:    "garden-linux",
					Version: "0.305.0",
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
				{
					Name:    "garden-linux",
					Version: "0.305.0",
				},
			},
		},
	}
}

func ExpectedContent(args models.InstallerArguments) string {
	content := `msiexec /passive /norestart /i %~dp0\DiegoWindows.msi ^{{ if .BbsRequireSsl }}
  BBS_CA_FILE=%~dp0\bbs_ca.crt ^
  BBS_CLIENT_CERT_FILE=%~dp0\bbs_client.crt ^
  BBS_CLIENT_KEY_FILE=%~dp0\bbs_client.key ^{{ end }}
  CONSUL_IPS=consul1.foo.bar ^
  CF_ETCD_CLUSTER=http://etcd1.foo.bar:4001 ^
  STACK=windows2012R2 ^
  REDUNDANCY_ZONE=windows ^
  LOGGREGATOR_SHARED_SECRET=secret123 ^{{ if .SyslogHostIP }}
  SYSLOG_HOST_IP=logs2.test.com ^
  SYSLOG_PORT=11111 ^{{ end }}
  CONSUL_ENCRYPT_FILE=%~dp0\consul_encrypt.key ^
  CONSUL_CA_FILE=%~dp0\consul_ca.crt ^
  CONSUL_AGENT_CERT_FILE=%~dp0\consul_agent.crt ^
  CONSUL_AGENT_KEY_FILE=%~dp0\consul_agent.key

msiexec /passive /norestart /i %~dp0\GardenWindows.msi ^
  ADMIN_USERNAME={{.Username}} ^
  ADMIN_PASSWORD={{.Password}}{{ if .SyslogHostIP }}^
  SYSLOG_HOST_IP=logs2.test.com ^
  SYSLOG_PORT=11111{{ end }}`
	content = strings.Replace(content, "\n", "\r\n", -1)
	temp := template.Must(template.New("").Parse(content))
	buf := bytes.NewBufferString("")
	err := temp.Execute(buf, args)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

var _ = Describe("Generate", func() {
	var outputDir string

	AfterEach(func() {
		Expect(os.RemoveAll(outputDir)).To(Succeed())
	})

	Describe("Success scenarios", func() {
		Context("when the deployment has syslog", func() {
			var session *gexec.Session
			var script string

			BeforeEach(func() {
				server := CreateServer("syslog_manifest.yml", DefaultIndexDeployment())
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(0))
				content, err := ioutil.ReadFile(path.Join(outputDir, "install.bat"))
				Expect(err).NotTo(HaveOccurred())
				script = strings.TrimSpace(string(content))
			})

			It("contains all the MSI parameters", func() {
				expectedContent := ExpectedContent(models.InstallerArguments{
					SyslogHostIP:  "logs2.test.com",
					BbsRequireSsl: true,
					Username:      "admin",
					Password:      "password",
				})
				Expect(script).To(Equal(expectedContent))
			})
		})

		Context("when the deployment has a string port in the syslog", func() {
			var session *gexec.Session
			var script string

			BeforeEach(func() {
				server := CreateServer("syslog_with_string_port_manifest.yml", DefaultIndexDeployment())
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(0))
				content, err := ioutil.ReadFile(path.Join(outputDir, "install.bat"))
				Expect(err).NotTo(HaveOccurred())
				script = strings.TrimSpace(string(content))
			})

			It("contains all the MSI parameters", func() {
				expectedContent := ExpectedContent(models.InstallerArguments{
					SyslogHostIP:  "logs2.test.com",
					BbsRequireSsl: true,
					Username:      "admin",
					Password:      "password",
				})
				Expect(script).To(Equal(expectedContent))
			})
		})

		Context("when the deployment has a null address and port in the syslog", func() {
			var session *gexec.Session
			var script string

			BeforeEach(func() {
				server := CreateServer("syslog_with_null_address_and_port.yml", DefaultIndexDeployment())
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(0))
				content, err := ioutil.ReadFile(path.Join(outputDir, "install.bat"))
				Expect(err).NotTo(HaveOccurred())
				script = strings.TrimSpace(string(content))
			})

			It("contains all the MSI parameters", func() {
				expectedContent := ExpectedContent(models.InstallerArguments{
					BbsRequireSsl: true,
					Username:      "admin",
					Password:      "password",
				})
				Expect(script).To(Equal(expectedContent))
			})
		})

		Context("when the server returns a one zone manifest", func() {
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

			Context("consul files", func() {
				It("generates the certificate authority cert", func() {
					cert, err := ioutil.ReadFile(path.Join(outputDir, "consul_ca.crt"))
					Expect(err).NotTo(HaveOccurred())
					Expect(cert).To(BeEquivalentTo("CONSUL_CA_CERT"))
				})

				It("generates the agent cert", func() {
					cert, err := ioutil.ReadFile(path.Join(outputDir, "consul_agent.crt"))
					Expect(err).NotTo(HaveOccurred())
					Expect(cert).To(BeEquivalentTo("CONSUL_AGENT_CERT"))
				})

				It("generates the agent key", func() {
					cert, err := ioutil.ReadFile(path.Join(outputDir, "consul_agent.key"))
					Expect(err).NotTo(HaveOccurred())
					Expect(cert).To(BeEquivalentTo("CONSUL_AGENT_KEY"))
				})

				It("generates the encrypt key", func() {
					cert, err := ioutil.ReadFile(path.Join(outputDir, "consul_encrypt.key"))
					Expect(err).NotTo(HaveOccurred())
					Expect(cert).To(BeEquivalentTo("CONSUL_ENCRYPT"))
				})
			})

			Describe("the lines of the batch script", func() {
				var script string

				BeforeEach(func() {
					content, err := ioutil.ReadFile(path.Join(outputDir, "install.bat"))
					Expect(err).NotTo(HaveOccurred())
					script = strings.TrimSpace(string(content))
				})

				It("contains all the MSI parameters", func() {
					expectedContent := ExpectedContent(models.InstallerArguments{
						BbsRequireSsl: true,
						Username:      "admin",
						Password:      "password",
					})
					Expect(script).To(Equal(expectedContent))
				})
			})
		})

		Describe("user provides arguments with special characters", func() {
			var server *ghttp.Server
			var script string

			BeforeEach(func() {
				var err error
				outputDir, err = ioutil.TempDir("", "XXXXXXX")
				Expect(err).NotTo(HaveOccurred())
				server = CreateServer("one_zone_manifest.yml", DefaultIndexDeployment())
				var session *gexec.Session
				session = StartGeneratorWithArgs(
					"-boshUrl", server.URL(),
					"-outputDir", outputDir,
					"-windowsUsername", "%admin",
					"-windowsPassword", "pass^word",
				)
				Eventually(session).Should(gexec.Exit(-1))
				content, err := ioutil.ReadFile(path.Join(outputDir, "install.bat"))
				Expect(err).NotTo(HaveOccurred())
				script = strings.TrimSpace(string(content))
			})

			It("escapes them", func() {
				Expect(server.ReceivedRequests()).To(HaveLen(2))
				expectedContent := ExpectedContent(models.InstallerArguments{
					BbsRequireSsl: true,
					Username:      "^%admin",
					Password:      "pass^^word",
				})
				Expect(script).To(Equal(expectedContent))
			})
		})

		Context("when the deployment has no bbs certs", func() {
			var session *gexec.Session
			var script string

			BeforeEach(func() {
				server := CreateServer("no_bbs_cert_manifest.yml", DefaultIndexDeployment())
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(0))
				content, err := ioutil.ReadFile(path.Join(outputDir, "install.bat"))
				Expect(err).NotTo(HaveOccurred())
				script = strings.TrimSpace(string(content))
			})

			It("does not contain bbs parameters", func() {
				expectedContent := ExpectedContent(models.InstallerArguments{
					BbsRequireSsl: false,
					Username:      "admin",
					Password:      "password",
				})
				Expect(script).To(Equal(expectedContent))
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

		Context("when the deployment has no consul certs", func() {
			var session *gexec.Session

			BeforeEach(func() {
				server := CreateServer("no_consul_cert_manifest.yml", DefaultIndexDeployment())
				session, outputDir = StartGeneratorWithURL(server.URL())
				Eventually(session).Should(gexec.Exit(1))
			})

			It("displays the reponse error to the user", func() {
				Expect(session.Err).Should(gbytes.Say("Failed to extract cert from deployment"))
			})
		})
	})
})
