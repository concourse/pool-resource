package integration_test

import (
	"io/ioutil"
	"os"
)

var _ = Describe("Check", func() {
	var (
		gitRepo string
		output  inResponse
	)

	BeforeEach(func() {
		var err error

		gitRepo, err = ioutil.TempDir("", "git-repo")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err = os.RemoveAll(gitRepo)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when the repo is at HEAD", func() {
		It("succesfully checks", func() {
		})
	})

	Context("when the repo is at a particular ref", func() {
		It("succesfully checks", func() {
		})
	})

	Context("when provided a bogus SHA", func() {
		It("succesfully checks", func() {
		})
	})

	Context("when given a particular pool", func() {
		It("succesfully checks", func() {
		})
	})

	Context("when the repo cannot not be fast-fowarded", func() {
		It("succesfully checks", func() {
		})
	})

	Context("when provided with credentials", func() {
		It("adds to the .netrc to successfully check", func() {
		})
	})

	Context("when credentials no longer required", func() {
		It("clears out the .netrc", func() {
		})
	})
})
