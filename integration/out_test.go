package integration_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/concourse/pool-resource/out"
)

var _ = Describe("Out", func() {
	var gitRepo string
	var bareGitRepo string
	var sourceDir string

	var outResponse out.OutResponse
	var outRequest out.OutRequest

	BeforeEach(func() {
		var err error
		gitRepo, err = ioutil.TempDir("", "git-repo")
		Ω(err).ShouldNot(HaveOccurred())

		bareGitRepo, err = ioutil.TempDir("", "bare-git-repo")
		Ω(err).ShouldNot(HaveOccurred())

		sourceDir, err = ioutil.TempDir("", "source-dir")
		Ω(err).ShouldNot(HaveOccurred())

		setupGitRepo(gitRepo)

		bareGitSetup := exec.Command("bash", "-e", "-c", fmt.Sprintf(`
			git clone %s --bare .
		`, gitRepo))
		bareGitSetup.Dir = bareGitRepo

		err = bareGitSetup.Run()
		Ω(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(bareGitRepo)
		Ω(err).ShouldNot(HaveOccurred())

		err = os.RemoveAll(gitRepo)
		Ω(err).ShouldNot(HaveOccurred())

		err = os.RemoveAll(sourceDir)
		Ω(err).ShouldNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		outResponse = runOut(outRequest, sourceDir)
	})

	Context("When acquiring a lock", func() {
		BeforeEach(func() {
			outRequest = out.OutRequest{
				Source: out.Source{
					URI:    bareGitRepo,
					Branch: "master",
					Pool:   "lock-pool",
				},
				Params: out.OutParams{
					Acquire: true,
				},
			}
		})

		It("moves a lock to claimed", func() {
			gitSetup := exec.Command("git", "pull", bareGitRepo)
			gitSetup.Dir = gitRepo
			err := gitSetup.Run()
			Ω(err).ShouldNot(HaveOccurred())

			gitVersion := exec.Command("git", "rev-parse", "HEAD")
			gitVersion.Dir = gitRepo
			sha, err := gitVersion.Output()
			Ω(err).ShouldNot(HaveOccurred())

			claimedFiles, err := ioutil.ReadDir(filepath.Join(gitRepo, "lock-pool", "claimed"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(len(claimedFiles)).Should(Equal(2))

			var lockFile string
			for _, file := range claimedFiles {
				filename := filepath.Base(file.Name())
				if filename != ".gitkeep" {
					lockFile = filename
				}
			}

			Ω(outResponse).Should(Equal(out.OutResponse{
				Version: out.Version{
					Ref: strings.TrimSpace(string(sha)),
				},
				Metadata: []out.MetadataPair{
					{Name: "lock_name", Value: lockFile},
					{Name: "pool_name", Value: "lock-pool"},
				},
			}))
		})
	})

	Context("When releasing a lock", func() {
		var myLocksGetDir string
		var outReleaseRequest out.OutRequest
		var outReleaseResponse out.OutResponse

		BeforeEach(func() {
			outRequest = out.OutRequest{
				Source: out.Source{
					URI:    bareGitRepo,
					Branch: "master",
					Pool:   "lock-pool",
				},
				Params: out.OutParams{
					Acquire: true,
				},
			}
		})

		JustBeforeEach(func() {
			var err error

			myLocksGetDir, err = ioutil.TempDir("", "my-locks")
			Ω(err).ShouldNot(HaveOccurred())

			jsonIn := fmt.Sprintf(`
				{
					"source": {
						"uri": "%s",
						"branch": "master"
					},
					"version": {
						"ref": "%s"
					}
				}`, bareGitRepo, string(outResponse.Version.Ref))

			runIn(jsonIn, filepath.Join(myLocksGetDir, "lock-step-name"))

			outReleaseRequest = out.OutRequest{
				Source: out.Source{
					URI:    bareGitRepo,
					Branch: "master",
					Pool:   "lock-pool",
				},
				Params: out.OutParams{
					Release: "lock-step-name",
				},
			}

			outReleaseResponse = runOut(outReleaseRequest, myLocksGetDir)
		})

		AfterEach(func() {
			err := os.RemoveAll(myLocksGetDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("moves the lock to unclaimed", func() {
			gitSetup := exec.Command("git", "pull", bareGitRepo)
			gitSetup.Dir = gitRepo
			err := gitSetup.Run()
			Ω(err).ShouldNot(HaveOccurred())

			gitVersion := exec.Command("git", "rev-parse", "HEAD")
			gitVersion.Dir = gitRepo
			sha, err := gitVersion.Output()
			Ω(err).ShouldNot(HaveOccurred())

			claimedFiles, err := ioutil.ReadDir(filepath.Join(gitRepo, "lock-pool", "claimed"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(len(claimedFiles)).Should(Equal(1))

			unclaimedFiles, err := ioutil.ReadDir(filepath.Join(gitRepo, "lock-pool", "unclaimed"))
			Ω(err).ShouldNot(HaveOccurred())

			Ω(len(unclaimedFiles)).Should(Equal(3))

			var releasedLockName string
			for _, metaDataPair := range outResponse.Metadata {
				if metaDataPair.Name == "lock_name" {
					releasedLockName = metaDataPair.Value
				}
			}

			Ω(outReleaseResponse).Should(Equal(out.OutResponse{
				Version: out.Version{
					Ref: strings.TrimSpace(string(sha)),
				},
				Metadata: []out.MetadataPair{
					{Name: "lock_name", Value: releasedLockName},
					{Name: "pool_name", Value: "lock-pool"},
				},
			}))
		})
	})

})
