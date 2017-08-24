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
	"uri": "file://%s",
	"branch": "master",
	"pool": "lock-pool"
	}
}`

const versionRequest = `
{
	"source": {
	"uri": "file://%s",
	"branch": "master",
	"pool": "lock-pool"
	},
	"version": {"ref": %q}
}`

const poolRequest = `
{
	"source": {
	"uri": "file://%s",
	"branch": "master",
	"pool": %q
	}
}`

type locksResponse map[string]string

var _ = Describe("Check", func() {
	var gitRepo string

	BeforeEach(func() {
		var err error

		gitRepo, err = ioutil.TempDir("", "git-repo")
		Expect(err).NotTo(HaveOccurred())

		setupGitRepo(gitRepo)
	})

	AfterEach(func() {
		err := os.RemoveAll(gitRepo)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when the repo is at HEAD", func() {
		It("succesfully checks", func() {
			addLockCommit(gitRepo)

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
			Expect(result[0]["ref"]).To(Equal(sha))
		})
	})

	Context("when the repo is at a particular ref", func() {
		It("succesfully checks", func() {
			addLockCommit(gitRepo)
			refTwo := addLockCommit(gitRepo)
			refThree := addLockCommit(gitRepo)

			checkData := fmt.Sprintf(versionRequest, gitRepo, refTwo)
			cmd := exec.Command(builtCheck)
			cmd.Stdin = bytes.NewBufferString(checkData)

			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			<-session.Exited

			Expect(session.ExitCode()).To(Equal(0))

			var result []locksResponse
			err = json.NewDecoder(bytes.NewReader(session.Out.Contents())).Decode(&result)
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(HaveLen(2))
			Expect(result[0]["ref"]).To(Equal(refTwo))
			Expect(result[1]["ref"]).To(Equal(refThree))
		})
	})

	Context("when provided a bogus SHA", func() {
		It("succesfully checks", func() {
			addLockCommit(gitRepo)
			lastRef := addLockCommit(gitRepo)

			checkData := fmt.Sprintf(versionRequest, gitRepo, "bogus reference")
			cmd := exec.Command(builtCheck)
			cmd.Stdin = bytes.NewBufferString(checkData)

			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			<-session.Exited

			Expect(session.ExitCode()).To(Equal(0))

			var result []locksResponse
			err = json.NewDecoder(bytes.NewReader(session.Out.Contents())).Decode(&result)
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(HaveLen(1))
			Expect(result[0]["ref"]).To(Equal(lastRef))
		})
	})

	Context("when given a particular pool", func() {
		It("succesfully checks", func() {
			setupPool(gitRepo, "other-pool")
			ref := addLockToPool(gitRepo, "other-pool", "some-lock", "master")
			addLockCommit(gitRepo)

			checkData := fmt.Sprintf(poolRequest, gitRepo, "other-pool")
			cmd := exec.Command(builtCheck)
			cmd.Stdin = bytes.NewBufferString(checkData)

			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			<-session.Exited

			Expect(session.ExitCode()).To(Equal(0))

			var result []locksResponse
			err = json.NewDecoder(bytes.NewReader(session.Out.Contents())).Decode(&result)
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(HaveLen(1))
			Expect(result[0]["ref"]).To(Equal(ref))
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
