package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/concourse/pool-resource/out"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("Out", func() {

	itWorksWithBranch("master")
	itWorksWithBranch("another-branch")
})

func itWorksWithBranch(branchName string) {
	Context("when the branch name is "+branchName, func() {
		var gitRepo string
		var bareGitRepo string
		var sourceDir string

		var outResponse out.OutResponse
		var outRequest out.OutRequest

		BeforeEach(func() {
			var err error
			gitRepo, err = ioutil.TempDir("", "git-repo")
			Expect(err).ShouldNot(HaveOccurred())

			bareGitRepo, err = ioutil.TempDir("", "bare-git-repo")
			Expect(err).ShouldNot(HaveOccurred())

			sourceDir, err = ioutil.TempDir("", "source-dir")
			Expect(err).ShouldNot(HaveOccurred())

			setupGitRepo(gitRepo)
			addLockToPool(gitRepo, "lock-pool", "some-lock", branch "master")
			addLockToPool(gitRepo, "lock-pool", "some-lock", branch "another-branch")

			bareGitSetup := exec.Command("bash", "-e", "-c", fmt.Sprintf(`
			git clone %s --bare .
		`, gitRepo))
			bareGitSetup.Dir = bareGitRepo

			err = bareGitSetup.Run()
			Expect(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(bareGitRepo)
			Expect(err).ShouldNot(HaveOccurred())

			err = os.RemoveAll(gitRepo)
			Expect(err).ShouldNot(HaveOccurred())

			err = os.RemoveAll(sourceDir)
			Expect(err).ShouldNot(HaveOccurred())
		})

		Context("when the config is incomplete", func() {
			var session *gexec.Session

			BeforeEach(func() {
				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        bareGitRepo,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 100 * time.Millisecond,
					},
					Params: out.OutParams{
						Acquire: true,
					},
				}
			})

			JustBeforeEach(func() {
				session = runOut(outRequest, sourceDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(1))
			})

			Context("when the uri isn't set", func() {
				BeforeEach(func() {
					outRequest.Source.URI = ""
				})

				It("complains about it", func() {
					errorMessages := string(session.Err.Contents())

					Expect(errorMessages).Should(ContainSubstring("invalid payload (missing uri)"))
				})
			})

			Context("when the pool isn't set", func() {
				BeforeEach(func() {
					outRequest.Source.Pool = ""
				})

				It("complains about it", func() {
					errorMessages := string(session.Err.Contents())

					Expect(errorMessages).Should(ContainSubstring("invalid payload (missing pool)"))
				})
			})

			Context("when the branch isn't set", func() {
				BeforeEach(func() {
					outRequest.Source.Branch = ""
				})

				It("complains about it", func() {
					errorMessages := string(session.Err.Contents())

					Expect(errorMessages).Should(ContainSubstring("invalid payload (missing branch)"))
				})
			})

			Context("when the params aren't set", func() {
				BeforeEach(func() {
					outRequest.Params = out.OutParams{}
				})

				It("complains about it", func() {
					errorMessages := string(session.Err.Contents())

					Expect(errorMessages).Should(ContainSubstring("invalid payload (missing acquire, release, remove, claim, add, or add_claimed)"))
				})
			})
		})

		Context("when acquiring a lock", func() {
			BeforeEach(func() {
				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        bareGitRepo,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 100 * time.Millisecond,
					},
					Params: out.OutParams{
						Acquire: true,
					},
				}

				session := runOut(outRequest, sourceDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err := json.Unmarshal(session.Out.Contents(), &outResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			FIt("moves a lock to claimed", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Expect(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				reClone.Dir = reCloneRepo
				err = reClone.Run()
				Expect(err).ShouldNot(HaveOccurred())

				claimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "claimed"))
				Expect(err).ShouldNot(HaveOccurred())

				Expect(len(claimedFiles)).Should(Equal(2))

				var lockFile string
				for _, file := range claimedFiles {
					filename := filepath.Base(file.Name())
					if filename != ".gitkeep" {
						lockFile = filename
					}
				}

				Expect(outResponse).Should(Equal(out.OutResponse{
					Version: version,
					Metadata: []out.MetadataPair{
						{Name: "lock_name", Value: lockFile},
						{Name: "pool_name", Value: "lock-pool"},
					},
				}))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Expect(session).Should(gbytes.Say("pipeline-name/job-name build 42 claiming: " + outResponse.Metadata[0].Value))
			})
		})

		Context("when there are no locks to be claimed", func() {
			var session *gexec.Session
			var claimAllLocksDir string

			BeforeEach(func() {
				var err error

				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        bareGitRepo,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 1 * time.Second,
					},
					Params: out.OutParams{
						Acquire: true,
					},
				}

				claimAllLocksDir, err = ioutil.TempDir("", "claiming-locks")
				Expect(err).ShouldNot(HaveOccurred())

				claimAllLocks := exec.Command("bash", "-e", "-c", fmt.Sprintf(`
				git clone --branch %s %s .

				git config user.email "ginkgo@localhost"
				git config user.name "Ginkgo Local"

				git mv lock-pool/unclaimed/* lock-pool/claimed/
				git commit -am "claiming all locks"
				git push
			`, branchName, bareGitRepo))

				claimAllLocks.Stdout = GinkgoWriter
				claimAllLocks.Stderr = GinkgoWriter
				claimAllLocks.Dir = claimAllLocksDir

				err = claimAllLocks.Run()
				Expect(err).ShouldNot(HaveOccurred())

				session = runOut(outRequest, sourceDir)
			})

			AfterEach(func() {
				err := os.RemoveAll(claimAllLocksDir)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("retries until a lock can be claimed", func() {
				Consistently(session, 2*time.Second).ShouldNot(gexec.Exit(0))

				releaseLock := exec.Command("bash", "-e", "-c", fmt.Sprint(`
				git mv lock-pool/claimed/some-lock lock-pool/unclaimed/some-lock
				git commit -am "unclaiming some-lock"
				git push
			`))

				releaseLock.Dir = claimAllLocksDir

				err := releaseLock.Run()
				Expect(err).ShouldNot(HaveOccurred())

				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err = json.Unmarshal(session.Out.Contents(), &outResponse)
				Expect(err).ShouldNot(HaveOccurred())

				Expect(outResponse).Should(Equal(out.OutResponse{
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

		Context("when claiming a specific lock", func() {
			BeforeEach(func() {
				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        bareGitRepo,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 100 * time.Millisecond,
					},
					Params: out.OutParams{
						Claim: "some-lock",
					},
				}

				session := runOut(outRequest, sourceDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err := json.Unmarshal(session.Out.Contents(), &outResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("moves the specific lock to claimed", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Expect(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				reClone.Dir = reCloneRepo
				err = reClone.Run()
				Expect(err).ShouldNot(HaveOccurred())

				_, err = ioutil.ReadFile(filepath.Join(reCloneRepo, "lock-pool", "claimed", "some-lock"))
				Expect(err).ShouldNot(HaveOccurred())

				Expect(outResponse).Should(Equal(out.OutResponse{
					Version: version,
					Metadata: []out.MetadataPair{
						{Name: "lock_name", Value: "some-lock"},
						{Name: "pool_name", Value: "lock-pool"},
					},
				}))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Expect(session).Should(gbytes.Say("pipeline-name/job-name build 42 claiming: some-lock"))
			})

			Context("when the specific lock has already been claimed", func() {
				var unclaimLockDir string
				BeforeEach(func() {
					var err error
					unclaimLockDir, err = ioutil.TempDir("", "claiming-locks")
					Expect(err).ShouldNot(HaveOccurred())
				})

				AfterEach(func() {
					err := os.RemoveAll(unclaimLockDir)
					Expect(err).ShouldNot(HaveOccurred())
				})

				It("continues to acquire the same lock", func() {
					claimSession := runOut(outRequest, sourceDir)
					Consistently(claimSession).ShouldNot(gexec.Exit(0))

					unclaimLock := exec.Command("bash", "-e", "-c", fmt.Sprintf(`
						git clone --branch %s %s .

						git config user.email "ginkgo@localhost"
						git config user.name "Ginkgo Local"

						git mv lock-pool/claimed/some-lock lock-pool/unclaimed/
						git commit -am "unclaim some-lock"
						git push
					`, branchName, bareGitRepo))

					unclaimLock.Stdout = GinkgoWriter
					unclaimLock.Stderr = GinkgoWriter
					unclaimLock.Dir = unclaimLockDir

					err := unclaimLock.Run()
					Expect(err).ShouldNot(HaveOccurred())

					<-claimSession.Exited
					Expect(claimSession.ExitCode()).To(Equal(0))
				})
			})
		})

		Context("when removing a lock", func() {
			var myLocksGetDir string
			var outRemoveRequest out.OutRequest
			var outRemoveResponse out.OutResponse

			var session *gexec.Session
			BeforeEach(func() {
				outRequest = out.OutRequest{
					Source: out.Source{
						URI:    bareGitRepo,
						Branch: branchName,
						Pool:   "lock-pool",
					},
					Params: out.OutParams{
						Acquire: true,
					},
				}

				session = runOut(outRequest, sourceDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err := json.Unmarshal(session.Out.Contents(), &outResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				session.Kill()
			})

			JustBeforeEach(func() {
				var err error

				myLocksGetDir, err = ioutil.TempDir("", "my-locks")
				Expect(err).ShouldNot(HaveOccurred())

				jsonIn := fmt.Sprintf(`
				{
					"source": {
						"uri": "%s",
						"branch": "%s",
						"pool": "lock-pool"
					},
					"version": {
						"ref": "%s"
					}
				}`, bareGitRepo, branchName, string(outResponse.Version.Ref))

				runIn(jsonIn, filepath.Join(myLocksGetDir, "lock-step-name"), 0)

				outRemoveRequest = out.OutRequest{
					Source: out.Source{
						URI:    bareGitRepo,
						Branch: branchName,
						Pool:   "lock-pool",
					},
					Params: out.OutParams{
						Remove: "lock-step-name",
					},
				}

				session := runOut(outRemoveRequest, myLocksGetDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err = json.Unmarshal(session.Out.Contents(), &outRemoveResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(myLocksGetDir)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("removes the lock from the pool", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Expect(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				reClone.Dir = reCloneRepo
				err = reClone.Run()
				Expect(err).ShouldNot(HaveOccurred())

				claimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "claimed"))
				Expect(err).ShouldNot(HaveOccurred())

				Expect(len(claimedFiles)).Should(Equal(1))

				unclaimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "unclaimed"))
				Expect(err).ShouldNot(HaveOccurred())

				Expect(len(unclaimedFiles)).Should(Equal(2))

				var removedLockName string
				for _, metaDataPair := range outResponse.Metadata {
					if metaDataPair.Name == "lock_name" {
						removedLockName = metaDataPair.Value
					}
				}

				Expect(outRemoveResponse).Should(Equal(out.OutResponse{
					Version: version,
					Metadata: []out.MetadataPair{
						{Name: "lock_name", Value: removedLockName},
						{Name: "pool_name", Value: "lock-pool"},
					},
				}))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outRemoveResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Expect(session).Should(gbytes.Say("pipeline-name/job-name build 42 removing: " + outRemoveResponse.Metadata[0].Value))
			})
		})

		Context("when releasing a lock", func() {
			var myLocksGetDir string
			var outReleaseRequest out.OutRequest
			var outReleaseResponse out.OutResponse

			BeforeEach(func() {
				outRequest = out.OutRequest{
					Source: out.Source{
						URI:    bareGitRepo,
						Branch: branchName,
						Pool:   "lock-pool",
					},
					Params: out.OutParams{
						Acquire: true,
					},
				}

				session := runOut(outRequest, sourceDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err := json.Unmarshal(session.Out.Contents(), &outResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			JustBeforeEach(func() {
				var err error

				myLocksGetDir, err = ioutil.TempDir("", "my-locks")
				Expect(err).ShouldNot(HaveOccurred())

				jsonIn := fmt.Sprintf(`
				{
					"source": {
						"uri": "%s",
						"branch": "%s",
						"pool": "lock-pool"
					},
					"version": {
						"ref": "%s"
					}
				}`, bareGitRepo, branchName, string(outResponse.Version.Ref))

				runIn(jsonIn, filepath.Join(myLocksGetDir, "lock-step-name"), 0)

				outReleaseRequest = out.OutRequest{
					Source: out.Source{
						URI:    bareGitRepo,
						Branch: branchName,
						Pool:   "lock-pool",
					},
					Params: out.OutParams{
						Release: "lock-step-name",
					},
				}

				session := runOut(outReleaseRequest, myLocksGetDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err = json.Unmarshal(session.Out.Contents(), &outReleaseResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(myLocksGetDir)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("moves the lock to unclaimed", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Expect(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				reClone.Dir = reCloneRepo
				err = reClone.Run()
				Expect(err).ShouldNot(HaveOccurred())

				claimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "claimed"))
				Expect(err).ShouldNot(HaveOccurred())

				Expect(len(claimedFiles)).Should(Equal(1))

				unclaimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "unclaimed"))
				Expect(err).ShouldNot(HaveOccurred())

				Expect(len(unclaimedFiles)).Should(Equal(3))

				var releasedLockName string
				for _, metaDataPair := range outResponse.Metadata {
					if metaDataPair.Name == "lock_name" {
						releasedLockName = metaDataPair.Value
					}
				}

				Expect(outReleaseResponse).Should(Equal(out.OutResponse{
					Version: version,
					Metadata: []out.MetadataPair{
						{Name: "lock_name", Value: releasedLockName},
						{Name: "pool_name", Value: "lock-pool"},
					},
				}))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outReleaseResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Expect(session).Should(gbytes.Say("pipeline-name/job-name build 42 unclaiming: " + outReleaseResponse.Metadata[0].Value))
			})
		})

		Context("when adding an initially unclaimed lock to the pool", func() {
			var lockToAddDir string
			var cloneDir string

			BeforeEach(func() {
				lockToAddDir, err := ioutil.TempDir("", "lock-to-add")
				Expect(err).ShouldNot(HaveOccurred())

				cloneDir, err = ioutil.TempDir("", "clone")
				Expect(err).ShouldNot(HaveOccurred())

				taskDir := filepath.Join(lockToAddDir, "task-name")
				err = os.Mkdir(taskDir, 0755)

				err = ioutil.WriteFile(filepath.Join(taskDir, "metadata"), []byte("hello"), 0555)
				Expect(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(taskDir, "name"), []byte("added-lock-name"), 0555)
				Expect(err).ShouldNot(HaveOccurred())

				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        bareGitRepo,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 100 * time.Millisecond,
					},
					Params: out.OutParams{
						Add: "task-name",
					},
				}

				session := runOut(outRequest, lockToAddDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err = json.Unmarshal(session.Out.Contents(), &outResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(lockToAddDir)
				Expect(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(cloneDir)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("adds the new lock in an unclaimed state", func() {
				clone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				clone.Dir = cloneDir
				err := clone.Run()
				Expect(err).ShouldNot(HaveOccurred())

				lockPath := filepath.Join(cloneDir, "lock-pool", "unclaimed", "added-lock-name")

				Expect(lockPath).Should(BeARegularFile())
				contents, err := ioutil.ReadFile(lockPath)
				Expect(err).ShouldNot(HaveOccurred())

				Expect(string(contents)).Should(Equal("hello"))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Expect(session).Should(gbytes.Say("pipeline-name/job-name build 42 adding unclaimed: " + outResponse.Metadata[0].Value))
			})
		})

		Context("when adding an initially claimed lock to the pool", func() {
			var lockToAddDir string
			var cloneDir string

			BeforeEach(func() {
				lockToAddDir, err := ioutil.TempDir("", "lock-to-add")
				Expect(err).ShouldNot(HaveOccurred())

				cloneDir, err = ioutil.TempDir("", "clone")
				Expect(err).ShouldNot(HaveOccurred())

				taskDir := filepath.Join(lockToAddDir, "task-name")
				err = os.Mkdir(taskDir, 0755)

				err = ioutil.WriteFile(filepath.Join(taskDir, "metadata"), []byte("hello"), 0555)
				Expect(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(taskDir, "name"), []byte("claimed-lock-name"), 0555)
				Expect(err).ShouldNot(HaveOccurred())

				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        bareGitRepo,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 100 * time.Millisecond,
					},
					Params: out.OutParams{
						AddClaimed: "task-name",
					},
				}

				session := runOut(outRequest, lockToAddDir)
				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

				err = json.Unmarshal(session.Out.Contents(), &outResponse)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(lockToAddDir)
				Expect(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(cloneDir)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("adds the new lock in a claimed state", func() {
				clone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				clone.Dir = cloneDir
				err := clone.Run()
				Expect(err).ShouldNot(HaveOccurred())

				lockPath := filepath.Join(cloneDir, "lock-pool", "claimed", "claimed-lock-name")

				Expect(lockPath).Should(BeARegularFile())
				contents, err := ioutil.ReadFile(lockPath)
				Expect(err).ShouldNot(HaveOccurred())

				Expect(string(contents)).Should(Equal("hello"))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Expect(session).Should(gbytes.Say("pipeline-name/job-name build 42 adding claimed: " + outResponse.Metadata[0].Value))
			})
		})

		Context("when 2 processes are acquiring a lock at the same time", func() {
			var sessionOne *gexec.Session
			var sessionTwo *gexec.Session
			var gitServerSession *gexec.Session

			var sessionOneDir string
			var sessionTwoDir string
			var claimLockDir string

			var exited <-chan struct{}

			BeforeEach(func() {
				var err error
				sessionOneDir, err = ioutil.TempDir("", "session-one")
				Expect(err).ShouldNot(HaveOccurred())

				sessionTwoDir, err = ioutil.TempDir("", "session-two")
				Expect(err).ShouldNot(HaveOccurred())

				claimLockDir, err = ioutil.TempDir("", "claiming-locks")
				Expect(err).ShouldNot(HaveOccurred())

				gitPort := GinkgoParallelNode() + 9418
				gitURI := fmt.Sprintf("git://localhost:%d/", gitPort)

				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        gitURI,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 1 * time.Second,
					},
					Params: out.OutParams{
						Acquire: true,
					},
				}

				gitServerCommand := exec.Command("git", "daemon",
					"--verbose", "--export-all",
					"--strict-paths", "--reuseaddr",
					"--base-path="+bareGitRepo,
					"--enable=receive-pack",
					"--max-connections=1",
					"--port="+strconv.Itoa(gitPort),
					bareGitRepo+"/",
				)

				gitServerSession, err = gexec.Start(gitServerCommand, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())

				<-gitServerSession.Err.Detect("Ready to rumble")
			})

			AfterEach(func() {
				err := os.RemoveAll(claimLockDir)
				Expect(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(sessionOneDir)
				Expect(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(sessionTwoDir)
				Expect(err).ShouldNot(HaveOccurred())

				gitServerSession.Terminate().Wait()
			})

			JustBeforeEach(func() {
				trigger := make(chan struct{})
				oneReady := make(chan struct{})
				twoReady := make(chan struct{})

				e := make(chan struct{})
				exited = e

				go func() {
					defer GinkgoRecover()

					<-trigger

					sessionOne = runOut(outRequest, sessionOneDir)
					close(oneReady)

					e <- <-sessionOne.Exited
				}()

				go func() {
					defer GinkgoRecover()

					<-trigger

					sessionTwo = runOut(outRequest, sessionTwoDir)
					close(twoReady)

					e <- <-sessionTwo.Exited
				}()

				close(trigger)

				<-oneReady
				<-twoReady
			})

			Context("when another lock is acquired in the same pool at the same time", func() {
				It("does not output an error message", func() {
					<-exited
					<-exited

					sessionOne.Terminate().Wait()
					sessionTwo.Terminate().Wait()

					Expect(sessionOne.Err).ShouldNot(gbytes.Say("err"))
					Expect(sessionTwo.Err).ShouldNot(gbytes.Say("err"))
				})
			})

			Context("when acquiring locks in 2 places within the same second", func() {

				BeforeEach(func() {
					var err error
					claimOneLock := exec.Command("bash", "-e", "-c", fmt.Sprintf(`
				git clone --branch %s %s .

				git config user.email "ginkgo@localhost"
				git config user.name "Ginkgo Local"

				git mv lock-pool/unclaimed/some-lock lock-pool/claimed/
				git commit -am "claiming a lock"
				git push
			`, branchName, bareGitRepo))

					claimOneLock.Stdout = GinkgoWriter
					claimOneLock.Stderr = GinkgoWriter
					claimOneLock.Dir = claimLockDir

					err = claimOneLock.Run()
					Expect(err).ShouldNot(HaveOccurred())

				})

				It("does not acquire the same lock", func() {
					<-exited
					Consistently(exited, 2*time.Second).ShouldNot(Receive())

					sessionOne.Terminate().Wait()
					sessionTwo.Terminate().Wait()

					Expect(sessionOne.Err).ShouldNot(gbytes.Say("err"))
					Expect(sessionTwo.Err).ShouldNot(gbytes.Say("err"))
				})
			})
		})
	})
}
