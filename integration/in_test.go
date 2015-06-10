package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("In", func() {
	var inDestination string
	var gitRepo string

	var sha []byte
	var output inResponse

	BeforeEach(func() {
		var err error
		inDestination, err = ioutil.TempDir("", "in-destination")
		gitRepo, err = ioutil.TempDir("", "git-repo")
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(inDestination)
		Ω(err).ShouldNot(HaveOccurred())

		err = os.RemoveAll(gitRepo)
		Ω(err).ShouldNot(HaveOccurred())
	})

	Context("when the config is incomplete", func() {
		var session *gexec.Session

		BeforeEach(func() {
			session = runIn("{}", inDestination, 1)
		})

		It("returns all config errors", func() {
			errorMessages := string(session.Err.Contents())

			Ω(errorMessages).Should(ContainSubstring("invalid payload (missing uri)"))
			Ω(errorMessages).Should(ContainSubstring("invalid payload (missing branch)"))
			Ω(errorMessages).Should(ContainSubstring("invalid payload (missing pool)"))
		})
	})

	Context("When a previous version is given", func() {
		BeforeEach(func() {
			var err error

			setupGitRepo(gitRepo)

			claimLock := exec.Command("bash", "-e", "-c", `
				git mv  lock-pool/unclaimed/some-lock  lock-pool/claimed/some-lock
				git add .
				git commit -m 'claiming some-lock'
			`)

			claimLock.Dir = gitRepo

			err = claimLock.Run()
			Ω(err).ShouldNot(HaveOccurred())

			gitVersion := exec.Command("git", "rev-parse", "HEAD")

			gitVersion.Dir = gitRepo

			sha, err = gitVersion.Output()
			Ω(err).ShouldNot(HaveOccurred())
			jsonIn := fmt.Sprintf(`
				{
					"source": {
						"uri": "%s",
						"branch": "master",
						"pool": "lock-pool"
					},
					"version": {
						"ref": "%s"
					}
				}`, gitRepo, string(sha))

			session := runIn(jsonIn, inDestination, 0)

			err = json.Unmarshal(session.Out.Contents(), &output)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("outputs the metadata for the environment", func() {
			metaDataFile := filepath.Join(inDestination, "metadata")
			Ω(metaDataFile).Should(BeARegularFile())

			fileContents, err := ioutil.ReadFile(metaDataFile)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fileContents).Should(MatchJSON(`{"some":"json"}`))

			lockNameFile := filepath.Join(inDestination, "name")
			Ω(lockNameFile).Should(BeARegularFile())

			fileContents, err = ioutil.ReadFile(lockNameFile)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(strings.TrimSpace(string(fileContents))).Should(Equal("some-lock"))

			Ω(output).Should(Equal(inResponse{
				Version: version{
					Ref: string(strings.TrimSpace(string(sha))),
				},
				Metadata: []metadataPair{
					{Name: "lock_name", Value: "some-lock"},
					{Name: "pool_name", Value: "lock-pool"},
				},
			}))
		})
	})
})
