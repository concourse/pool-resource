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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/concourse/pool-resource/out"
)

var _ = Describe("Out", func() {

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

					Ω(errorMessages).Should(ContainSubstring("invalid payload (missing uri)"))
				})
			})

			Context("when the pool isn't set", func() {
				BeforeEach(func() {
					outRequest.Source.Pool = ""
				})

				It("complains about it", func() {
					errorMessages := string(session.Err.Contents())

					Ω(errorMessages).Should(ContainSubstring("invalid payload (missing pool)"))
				})
			})

			Context("when the branch isn't set", func() {
				BeforeEach(func() {
					outRequest.Source.Branch = ""
				})

				It("complains about it", func() {
					errorMessages := string(session.Err.Contents())

					Ω(errorMessages).Should(ContainSubstring("invalid payload (missing branch)"))
				})
			})

			Context("when the params aren't set", func() {
				BeforeEach(func() {
					outRequest.Params = out.OutParams{}
				})

				It("complains about it", func() {
					errorMessages := string(session.Err.Contents())

					Ω(errorMessages).Should(ContainSubstring("invalid payload (missing acquire, release, remove, claim, add, or add_claimed)"))
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
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("moves a lock to claimed", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Ω(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
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

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Ω(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 claiming: " + outResponse.Metadata[0].Value))
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
				Ω(err).ShouldNot(HaveOccurred())

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
				Ω(err).ShouldNot(HaveOccurred())

				session = runOut(outRequest, sourceDir)
			})

			AfterEach(func() {
				err := os.RemoveAll(claimAllLocksDir)
				Ω(err).ShouldNot(HaveOccurred())
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
				Ω(err).ShouldNot(HaveOccurred())

				<-session.Exited
				Expect(session.ExitCode()).To(Equal(0))

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
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("moves the specific lock to claimed", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Ω(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				reClone.Dir = reCloneRepo
				err = reClone.Run()
				Ω(err).ShouldNot(HaveOccurred())

				_, err = ioutil.ReadFile(filepath.Join(reCloneRepo, "lock-pool", "claimed", "some-lock"))
				Ω(err).ShouldNot(HaveOccurred())

				Ω(outResponse).Should(Equal(out.OutResponse{
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
				Ω(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 claiming: some-lock"))
			})

			Context("when the specific lock has already been claimed", func() {
				var unclaimLockDir string
				BeforeEach(func() {
					var err error
					unclaimLockDir, err = ioutil.TempDir("", "claiming-locks")
					Ω(err).ShouldNot(HaveOccurred())
				})

				AfterEach(func() {
					err := os.RemoveAll(unclaimLockDir)
					Ω(err).ShouldNot(HaveOccurred())
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
					Ω(err).ShouldNot(HaveOccurred())

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
				Ω(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				session.Kill()
			})

			JustBeforeEach(func() {
				var err error

				myLocksGetDir, err = ioutil.TempDir("", "my-locks")
				Ω(err).ShouldNot(HaveOccurred())

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
				Ω(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(myLocksGetDir)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("removes the lock from the pool", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Ω(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				reClone.Dir = reCloneRepo
				err = reClone.Run()
				Ω(err).ShouldNot(HaveOccurred())

				claimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "claimed"))
				Ω(err).ShouldNot(HaveOccurred())

				Ω(len(claimedFiles)).Should(Equal(1))

				unclaimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "unclaimed"))
				Ω(err).ShouldNot(HaveOccurred())

				Ω(len(unclaimedFiles)).Should(Equal(2))

				var removedLockName string
				for _, metaDataPair := range outResponse.Metadata {
					if metaDataPair.Name == "lock_name" {
						removedLockName = metaDataPair.Value
					}
				}

				Ω(outRemoveResponse).Should(Equal(out.OutResponse{
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
				Ω(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 removing: " + outRemoveResponse.Metadata[0].Value))
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
				Ω(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(myLocksGetDir)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("moves the lock to unclaimed", func() {
				version := getVersion(bareGitRepo, "origin/"+branchName)

				reCloneRepo, err := ioutil.TempDir("", "git-version-repo")
				Ω(err).ShouldNot(HaveOccurred())

				defer os.RemoveAll(reCloneRepo)

				reClone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				reClone.Dir = reCloneRepo
				err = reClone.Run()
				Ω(err).ShouldNot(HaveOccurred())

				claimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "claimed"))
				Ω(err).ShouldNot(HaveOccurred())

				Ω(len(claimedFiles)).Should(Equal(1))

				unclaimedFiles, err := ioutil.ReadDir(filepath.Join(reCloneRepo, "lock-pool", "unclaimed"))
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

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outReleaseResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Ω(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 unclaiming: " + outReleaseResponse.Metadata[0].Value))
			})
		})

		Context("when adding an initially unclaimed lock to the pool", func() {
			var lockToAddDir string
			var cloneDir string

			BeforeEach(func() {
				lockToAddDir, err := ioutil.TempDir("", "lock-to-add")
				Ω(err).ShouldNot(HaveOccurred())

				cloneDir, err = ioutil.TempDir("", "clone")
				Ω(err).ShouldNot(HaveOccurred())

				taskDir := filepath.Join(lockToAddDir, "task-name")
				err = os.Mkdir(taskDir, 0755)

				err = ioutil.WriteFile(filepath.Join(taskDir, "metadata"), []byte("hello"), 0555)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(taskDir, "name"), []byte("added-lock-name"), 0555)
				Ω(err).ShouldNot(HaveOccurred())

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
				Ω(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(lockToAddDir)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(cloneDir)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("adds the new lock in an unclaimed state", func() {
				clone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				clone.Dir = cloneDir
				err := clone.Run()
				Ω(err).ShouldNot(HaveOccurred())

				lockPath := filepath.Join(cloneDir, "lock-pool", "unclaimed", "added-lock-name")

				Ω(lockPath).Should(BeARegularFile())
				contents, err := ioutil.ReadFile(lockPath)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(string(contents)).Should(Equal("hello"))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Ω(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 adding unclaimed: " + outResponse.Metadata[0].Value))
			})
		})

		Context("when adding an initially claimed lock to the pool", func() {
			var lockToAddDir string
			var cloneDir string

			BeforeEach(func() {
				lockToAddDir, err := ioutil.TempDir("", "lock-to-add")
				Ω(err).ShouldNot(HaveOccurred())

				cloneDir, err = ioutil.TempDir("", "clone")
				Ω(err).ShouldNot(HaveOccurred())

				taskDir := filepath.Join(lockToAddDir, "task-name")
				err = os.Mkdir(taskDir, 0755)

				err = ioutil.WriteFile(filepath.Join(taskDir, "metadata"), []byte("hello"), 0555)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(taskDir, "name"), []byte("claimed-lock-name"), 0555)
				Ω(err).ShouldNot(HaveOccurred())

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
				Ω(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := os.RemoveAll(lockToAddDir)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(cloneDir)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("adds the new lock in a claimed state", func() {
				clone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
				clone.Dir = cloneDir
				err := clone.Run()
				Ω(err).ShouldNot(HaveOccurred())

				lockPath := filepath.Join(cloneDir, "lock-pool", "claimed", "claimed-lock-name")

				Ω(lockPath).Should(BeARegularFile())
				contents, err := ioutil.ReadFile(lockPath)
				Ω(err).ShouldNot(HaveOccurred())

				Ω(string(contents)).Should(Equal("hello"))
			})

			It("commits with a descriptive message", func() {
				log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
				log.Dir = bareGitRepo

				session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
				Ω(err).ShouldNot(HaveOccurred())

				<-session.Exited

				Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 adding claimed: " + outResponse.Metadata[0].Value))
			})
		})

		Context("when updating a lock", func() {
			var lockToAddDir string
			var cloneDir string

			BeforeEach(func() {
				var err error

				lockToAddDir, err = ioutil.TempDir("", "lock-to-add")
				Ω(err).ShouldNot(HaveOccurred())

				cloneDir, err = ioutil.TempDir("", "clone")
				Ω(err).ShouldNot(HaveOccurred())

				taskDir := filepath.Join(lockToAddDir, "task-name")
				err = os.Mkdir(taskDir, 0755)

				err = ioutil.WriteFile(filepath.Join(taskDir, "metadata"), []byte("hello"), 0555)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(taskDir, "name"), []byte("some-lock"), 0555)
				Ω(err).ShouldNot(HaveOccurred())

				outRequest = out.OutRequest{
					Source: out.Source{
						URI:        bareGitRepo,
						Branch:     branchName,
						Pool:       "lock-pool",
						RetryDelay: 100 * time.Millisecond,
					},
					Params: out.OutParams{
						Update: "task-name",
					},
				}
			})

			AfterEach(func() {
				err := os.RemoveAll(lockToAddDir)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(cloneDir)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when the old lock is not present in the pool", func() {
				BeforeEach(func() {
					lockNameFile := filepath.Join(lockToAddDir, "task-name", "name")

					Ω(os.Remove(lockNameFile)).Should(Succeed())

					err := ioutil.WriteFile(lockNameFile, []byte("added-lock-name"), 0555)
					Ω(err).ShouldNot(HaveOccurred())

					session := runOut(outRequest, lockToAddDir)
					<-session.Exited
					Expect(session.ExitCode()).To(Equal(0))

					err = json.Unmarshal(session.Out.Contents(), &outResponse)
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("adds the new lock in an unclaimed state", func() {
					clone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
					clone.Dir = cloneDir
					err := clone.Run()
					Ω(err).ShouldNot(HaveOccurred())

					lockPath := filepath.Join(cloneDir, "lock-pool", "unclaimed", "added-lock-name")

					Ω(lockPath).Should(BeARegularFile())
					contents, err := ioutil.ReadFile(lockPath)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(string(contents)).Should(Equal("hello"))
				})

				It("commits with a descriptive message", func() {
					log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
					log.Dir = bareGitRepo

					session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
					Ω(err).ShouldNot(HaveOccurred())

					<-session.Exited

					Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 adding unclaimed: " + outResponse.Metadata[0].Value))
				})
			})

			Context("when the old lock is in the claimed state", func() {
				var unclaimLockDir string

				BeforeEach(func() {
					var err error
					unclaimLockDir, err = ioutil.TempDir("", "claiming-locks")
					Ω(err).ShouldNot(HaveOccurred())

					outRequest := out.OutRequest{
						Source: out.Source{
							URI:    bareGitRepo,
							Branch: branchName,
							Pool:   "lock-pool",
						},
						Params: out.OutParams{
							Claim: "some-lock",
						},
					}

					session := runOut(outRequest, sourceDir)
					<-session.Exited
					Expect(session.ExitCode()).To(Equal(0))

					err = json.Unmarshal(session.Out.Contents(), &outResponse)
					Ω(err).ShouldNot(HaveOccurred())
				})

				AfterEach(func() {
					err := os.RemoveAll(unclaimLockDir)
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("continues to update the same lock", func() {
					claimSession := runOut(outRequest, lockToAddDir)
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
					Ω(err).ShouldNot(HaveOccurred())

					<-claimSession.Exited
					Expect(claimSession.ExitCode()).To(Equal(0))

					err = json.Unmarshal(claimSession.Out.Contents(), &outResponse)
					Ω(err).ShouldNot(HaveOccurred())

					clone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
					clone.Dir = cloneDir
					err = clone.Run()
					Ω(err).ShouldNot(HaveOccurred())

					lockPath := filepath.Join(cloneDir, "lock-pool", "unclaimed", "some-lock")

					Ω(lockPath).Should(BeARegularFile())
					contents, err := ioutil.ReadFile(lockPath)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(string(contents)).Should(Equal("hello"))
				})
			})

			Context("when the old lock is in the unclaimed state", func() {
				BeforeEach(func() {
					session := runOut(outRequest, lockToAddDir)
					<-session.Exited
					Expect(session.ExitCode()).To(Equal(0))

					err := json.Unmarshal(session.Out.Contents(), &outResponse)
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("updates the old lock with the new metadata", func() {
					clone := exec.Command("git", "clone", "--branch", branchName, bareGitRepo, ".")
					clone.Dir = cloneDir
					err := clone.Run()
					Ω(err).ShouldNot(HaveOccurred())

					lockPath := filepath.Join(cloneDir, "lock-pool", "unclaimed", "some-lock")

					Ω(lockPath).Should(BeARegularFile())
					contents, err := ioutil.ReadFile(lockPath)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(string(contents)).Should(Equal("hello"))
				})

				It("commits with a descriptive message", func() {
					log := exec.Command("git", "log", "--oneline", "-1", outResponse.Version.Ref)
					log.Dir = bareGitRepo

					session, err := gexec.Start(log, GinkgoWriter, GinkgoWriter)
					Ω(err).ShouldNot(HaveOccurred())

					<-session.Exited

					Ω(session).Should(gbytes.Say("team-name/pipeline-name/job-name build 42 updating: " + outResponse.Metadata[0].Value))
				})
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
				Ω(err).ShouldNot(HaveOccurred())

				sessionTwoDir, err = ioutil.TempDir("", "session-two")
				Ω(err).ShouldNot(HaveOccurred())

				claimLockDir, err = ioutil.TempDir("", "claiming-locks")
				Ω(err).ShouldNot(HaveOccurred())

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
				Ω(err).ShouldNot(HaveOccurred())

				<-gitServerSession.Err.Detect("Ready to rumble")
			})

			AfterEach(func() {
				err := os.RemoveAll(claimLockDir)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(sessionOneDir)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.RemoveAll(sessionTwoDir)
				Ω(err).ShouldNot(HaveOccurred())

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

					Ω(sessionOne.Err).ShouldNot(gbytes.Say("err"))
					Ω(sessionTwo.Err).ShouldNot(gbytes.Say("err"))
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
					Ω(err).ShouldNot(HaveOccurred())

				})

				It("does not acquire the same lock", func() {
					<-exited
					Consistently(exited, 2*time.Second).ShouldNot(Receive())

					sessionOne.Terminate().Wait()
					sessionTwo.Terminate().Wait()

					Ω(sessionOne.Err).ShouldNot(gbytes.Say("err"))
					Ω(sessionTwo.Err).ShouldNot(gbytes.Say("err"))
				})
			})
		})
	})
}
