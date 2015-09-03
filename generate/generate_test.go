package main_test

import (
	"bytes"

	"github.com/cloudfoundry-incubator/candiedyaml"
	. "github.com/cloudfoundry-incubator/greenhouse-install-script-generator/generate"

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
			result, err := GetIn(obj, "foo", "bar", "baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEquivalentTo([]interface{}{"one", "two"}))
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
			result, err := GetIn(obj, "foo", "bar", "baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
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
			result, err := GetIn(obj, "foo", "bar", "baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
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
			result, err := GetIn(obj, "foo", "bar", 0, "baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEquivalentTo(5))
		})

		It("casts the key to an int when accessing an array", func() {
			result, err := GetIn(obj, "foo", "bar", "0", "baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEquivalentTo(5))
		})

		It("errors when a non-integer is used with an array", func() {
			result, err := GetIn(obj, "foo", "bar", "bad key")
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("invalid syntax"))
		})
	})
})

var _ = Describe("EscapeSpecialCharacters", func() {
	// (, ), %, !, ^, ", <, >, &, and |.
	It(`escapes all kinds of special characters`, func() {
		Expect(EscapeSpecialCharacters("%hi guys%")).To(Equal("^%hi guys^%"))
		Expect(EscapeSpecialCharacters("((hi guys)")).To(Equal("^(^(hi guys^)"))
		Expect(EscapeSpecialCharacters(`"hello"`)).To(Equal(`^"hello^"`))
		Expect(EscapeSpecialCharacters(`<hello>`)).To(Equal(`^<hello^>`))
		Expect(EscapeSpecialCharacters(`&hi!`)).To(Equal(`^&hi^!`))
		Expect(EscapeSpecialCharacters(`|hi!`)).To(Equal(`^|hi^!`))
		Expect(EscapeSpecialCharacters(`^so meta -_-`)).To(Equal(`^^so meta -_-`))
	})
})
