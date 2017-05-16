package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const simpleRequest = `
{
	"source": {
	"uri": "%s",
	"branch": "master",
	"pool": "lock-pool"
	}
}`

type locksResponse map[string]string

var _ = Describe("Check", func() {
	var (
		gitRepo string
	)

	BeforeEach(func() {
		var err error

		gitRepo, err = ioutil.TempDir("", "git-repo")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(gitRepo)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when the repo is at HEAD", func() {
		FIt("succesfully checks", func() {
			setupGitRepo(gitRepo)

			cmd := exec.Command(builtCheck)
			cmd.Stdin = bytes.NewBufferString(fmt.Sprintf(simpleRequest, gitRepo))

			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			<-session.Exited

			Expect(session.ExitCode()).To(Equal(0))

			var result []locksResponse
			err = json.NewDecoder(bytes.NewReader(session.Out.Contents())).Decode(&result)
			Expect(err).NotTo(HaveOccurred())

			gitVersion := exec.Command("git", "rev-parse", "HEAD")
			gitVersion.Dir = gitRepo

			shaStr, err := gitVersion.Output()
			Expect(err).NotTo(HaveOccurred())

			sha := strings.TrimSpace(string(shaStr))

			Expect(result).To(HaveLen(1))
			Expect(result[0]["version"]).To(Equal(sha))
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
