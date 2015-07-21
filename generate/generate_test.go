package main_test

import (
	"bytes"

	"github.com/cloudfoundry-incubator/candiedyaml"
	. "github.com/pivotal-cf/greenhouse-install-script-generator/generate"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetIn", func() {
	var yml string
	var obj interface{}

	JustBeforeEach(func() {
		buf := bytes.NewBufferString(yml)
		Expect(candiedyaml.NewDecoder(buf).Decode(&obj)).To(Succeed())
	})

	Context("when the path exists in the given yaml", func() {
		BeforeEach(func() {
			yml = `
---
foo:
  bar:
    other: 5
    baz:
      - one
      - two
`
		})

		It("returns the element", func() {
			Expect(GetIn(obj, "foo", "bar", "baz")).To(BeEquivalentTo([]interface{}{"one", "two"}))
		})
	})

	Context("the path does not exist", func() {
		BeforeEach(func() {
			yml = `
---
foo:
  bar:
    other: 5
`
		})

		It("returns nil", func() {
			Expect(GetIn(obj, "foo", "bar", "baz")).To(BeNil())
		})
	})

	Context("an intermediate key doesn't have a map value", func() {
		BeforeEach(func() {
			yml = `
---
foo:
  bar:
    5
`
		})

		It("returns nil", func() {
			Expect(GetIn(obj, "foo", "bar", "baz")).To(BeNil())
		})
	})

	Context("when there are arrays", func() {
		BeforeEach(func() {
			yml = `
---
foo:
  bar:
    - baz: 5
`
		})

		It("can access items by index", func() {
			Expect(GetIn(obj, "foo", "bar", 0, "baz")).To(BeEquivalentTo(5))
		})
	})
})
