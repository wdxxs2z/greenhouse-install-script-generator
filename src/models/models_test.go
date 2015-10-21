package models_test

import (
	"io/ioutil"
	"models"
	"os"

	"gopkg.in/yaml.v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Models", func() {
	It("can parse the yaml properly", func() {
		f, err := os.Open("../integration/job_override_manifest.yml")
		defer f.Close()
		manifest := models.Manifest{}
		content, err := ioutil.ReadAll(f)
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(content, &manifest)
		// err = yaml.NewDecoder(f).Decode(&manifest)
		Expect(err).NotTo(HaveOccurred())

		Expect(manifest.Jobs).To(HaveLen(2))
	})

	It("no bbs test", func() {
		f, err := os.Open("../integration/no_bbs_cert_manifest.yml")
		defer f.Close()
		manifest := models.Manifest{}
		content, err := ioutil.ReadAll(f)
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(content, &manifest)
		Expect(err).NotTo(HaveOccurred())
	})
})
