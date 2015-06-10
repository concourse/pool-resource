package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

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

	Context("when the config is incomplete", func() {
		var session *gexec.Session

		BeforeEach(func() {
			outRequest = out.OutRequest{}

			session = runOut(outRequest, sourceDir)
			Eventually(session).Should(gexec.Exit(1))

		})

		It("returns all config errors", func() {
			errorMessages := string(session.Err.Contents())

			Ω(errorMessages).Should(ContainSubstring("invalid payload (missing uri)"))
			Ω(errorMessages).Should(ContainSubstring("invalid payload (missing pool)"))
			Ω(errorMessages).Should(ContainSubstring("invalid payload (missing branch)"))
			Ω(errorMessages).Should(ContainSubstring("invalid payload (missing acquire or release)"))
		})

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

			session := runOut(outRequest, sourceDir)
			Eventually(session).Should(gexec.Exit(0))

			err := json.Unmarshal(session.Out.Contents(), &outResponse)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("moves a lock to claimed", func() {
			version := getVersion(bareGitRepo)

			reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
			Ω(err).ShouldNot(HaveOccurred())

			defer os.RemoveAll(reCloneRepo)

			reClone := exec.Command("git", "clone", bareGitRepo, ".")
			reClone.Dir = reCloneRepo
			err = reClone.Run()
			Ω(err).ShouldNot(HaveOccurred())

			claimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "claimed"))
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
				Version: version,
				Metadata: []out.MetadataPair{
					{Name: "lock_name", Value: lockFile},
					{Name: "pool_name", Value: "lock-pool"},
				},
			}))
		})
	})

	Context("when there are no locks to be claimed", func() {
		var session *gexec.Session
		var claimAllLocksDir string

		BeforeEach(func() {
			var err error

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

			claimAllLocksDir, err = ioutil.TempDir("", "claiming-locks")
			Ω(err).ShouldNot(HaveOccurred())

			claimAllLocks := exec.Command("bash", "-e", "-c", fmt.Sprintf(`
				git clone %s .
				git mv lock-pool/unclaimed/* lock-pool/claimed/
				git commit -am "claiming all locks"
				git push
			`, bareGitRepo))

			claimAllLocks.Dir = claimAllLocksDir

			err = claimAllLocks.Run()
			Ω(err).ShouldNot(HaveOccurred())

			session = runOut(outRequest, sourceDir)
		})

		AfterEach(func() {
			err := os.RemoveAll(claimAllLocksDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("retries until a lock can be claimed", func() {
			Consistently(session, 11*time.Second).ShouldNot(gexec.Exit(0))

			releaseLock := exec.Command("bash", "-e", "-c", fmt.Sprint(`
				git mv lock-pool/claimed/some-lock lock-pool/unclaimed/some-lock
				git commit -am "unclaiming some-lock"
				git push
			`))

			releaseLock.Dir = claimAllLocksDir

			err := releaseLock.Run()
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(session, 20*time.Second).Should(gexec.Exit(0))

			err = json.Unmarshal(session.Out.Contents(), &outResponse)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(outResponse).Should(Equal(out.OutResponse{
				Version: out.Version{
					Ref: outResponse.Version.Ref,
				},
				Metadata: []out.MetadataPair{
					{Name: "lock_name", Value: "some-lock"},
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

			session := runOut(outRequest, sourceDir)
			Eventually(session).Should(gexec.Exit(0))

			err := json.Unmarshal(session.Out.Contents(), &outResponse)
			Ω(err).ShouldNot(HaveOccurred())
		})

		JustBeforeEach(func() {
			var err error

			myLocksGetDir, err = ioutil.TempDir("", "my-locks")
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
				}`, bareGitRepo, string(outResponse.Version.Ref))

			runIn(jsonIn, filepath.Join(myLocksGetDir, "lock-step-name"), 0)

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

			session := runOut(outReleaseRequest, myLocksGetDir)
			Eventually(session).Should(gexec.Exit(0))

			err = json.Unmarshal(session.Out.Contents(), &outReleaseResponse)
			Ω(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(myLocksGetDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("moves the lock to unclaimed", func() {
			version := getVersion(bareGitRepo)

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
				Version: version,
				Metadata: []out.MetadataPair{
					{Name: "lock_name", Value: releasedLockName},
					{Name: "pool_name", Value: "lock-pool"},
				},
			}))
		})
	})
})
