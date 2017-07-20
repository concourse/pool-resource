package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("In", func() {
	var inDestination string
	var gitRepo string

	var output inResponse

	BeforeEach(func() {
		var err error
		inDestination, err = ioutil.TempDir("", "in-destination")
		gitRepo, err = ioutil.TempDir("", "git-repo")
		Expect(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := os.RemoveAll(inDestination)
		Expect(err).ShouldNot(HaveOccurred())

		err = os.RemoveAll(gitRepo)
		Expect(err).ShouldNot(HaveOccurred())
	})

	Context("when the config is incomplete", func() {
		var session *gexec.Session

		BeforeEach(func() {
			session = runIn("{}", inDestination, 1)
		})

		It("returns all config errors", func() {
			errorMessages := string(session.Err.Contents())

			Expect(errorMessages).Should(ContainSubstring("invalid payload (missing uri)"))
			Expect(errorMessages).Should(ContainSubstring("invalid payload (missing branch)"))
			Expect(errorMessages).Should(ContainSubstring("invalid payload (missing pool)"))
		})
	})

	Context("when given version removed a lock", func() {
		BeforeEach(func() {
			var err error

			setupGitRepo(gitRepo)
			addLockToPool(gitRepo, "lock-pool", "some-lock")

			claimLock := exec.Command("bash", "-e", "-c", `
				git mv lock-pool/unclaimed/some-lock lock-pool/claimed/some-lock
				git commit -m 'claiming some-lock'

				git rm lock-pool/claimed/some-lock
				git commit -m 'removing some-lock'
			`)
			claimLock.Dir = gitRepo

			err = claimLock.Run()
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("return succesfully", func() {
			gitVersion := exec.Command("git", "rev-parse", "HEAD")
			gitVersion.Dir = gitRepo
			sha, err := gitVersion.Output()
			Expect(err).ShouldNot(HaveOccurred())
			shaStr := strings.TrimSpace(string(sha))

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
				}`, gitRepo, shaStr)

			session := runIn(jsonIn, inDestination, 0)

			err = json.Unmarshal(session.Out.Contents(), &output)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).Should(Equal(inResponse{
				Version: version{
					Ref: shaStr,
				},
				Metadata: []metadataPair{
					{Name: "lock_name", Value: "some-lock"},
					{Name: "pool_name", Value: "lock-pool"},
				},
			}))
		})
	})

	Context("when a previous version is given", func() {
		BeforeEach(func() {
			var err error

			setupGitRepo(gitRepo)
			addLockToPool(gitRepo, "lock-pool", "some-lock")

			claimLock := exec.Command("bash", "-e", "-c", `
				git mv lock-pool/unclaimed/some-lock lock-pool/claimed/some-lock
				git commit -m 'claiming some-lock'
			`)
			claimLock.Dir = gitRepo

			err = claimLock.Run()
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("outputs the metadata for the environment", func() {
			gitVersion := exec.Command("git", "rev-parse", "HEAD")
			gitVersion.Dir = gitRepo
			sha, err := gitVersion.Output()
			Expect(err).ShouldNot(HaveOccurred())
			shaStr := strings.TrimSpace(string(sha))

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
				}`, gitRepo, shaStr)

			session := runIn(jsonIn, inDestination, 0)

			err = json.Unmarshal(session.Out.Contents(), &output)
			Expect(err).ShouldNot(HaveOccurred())

			metaDataFile := filepath.Join(inDestination, "metadata")
			Expect(metaDataFile).Should(BeARegularFile())

			fileContents, err := ioutil.ReadFile(metaDataFile)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(fileContents).Should(MatchJSON(`{"some":"json"}`))

			lockNameFile := filepath.Join(inDestination, "name")
			Expect(lockNameFile).Should(BeARegularFile())

			fileContents, err = ioutil.ReadFile(lockNameFile)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(strings.TrimSpace(string(fileContents))).Should(Equal("some-lock"))

			Expect(output).Should(Equal(inResponse{
				Version: version{
					Ref: shaStr,
				},
				Metadata: []metadataPair{
					{Name: "lock_name", Value: "some-lock"},
					{Name: "pool_name", Value: "lock-pool"},
				},
			}))
		})

		Context("when the lock from the previous version has been released and we are trying to run it again", func() {
			var shaStr string

			Context("when the given commit claimed the lock but the lock was unclaimed afterwards", func() {
				BeforeEach(func() {
					var err error
					gitVersion := exec.Command("git", "rev-parse", "HEAD")
					gitVersion.Dir = gitRepo
					sha, err := gitVersion.Output()
					Expect(err).ShouldNot(HaveOccurred())
					shaStr = strings.TrimSpace(string(sha))

					unclaimLock := exec.Command("bash", "-e", "-c", `
						git mv lock-pool/claimed/some-lock lock-pool/unclaimed/some-lock
						git commit -m 'unclaiming: some-lock'
					`)
					unclaimLock.Dir = gitRepo

					err = unclaimLock.Run()
					Expect(err).ShouldNot(HaveOccurred())
				})

				It("fails with a useful error message because the lock is no longer safe to use", func() {
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
						}`, gitRepo, shaStr)

					session := runIn(jsonIn, inDestination, 1)

					Expect(session.Err).Should(gbytes.Say("error: lock instance is no longer acquired"))
				})

				Context("when the lock is acquired again but by another pipeline run and is run", func() {
					BeforeEach(func() {
						claimLock := exec.Command("bash", "-e", "-c", `
							git mv lock-pool/unclaimed/some-lock lock-pool/claimed/some-lock
							git commit -m 'claiming some-lock'
						`)
						claimLock.Dir = gitRepo

						err := claimLock.Run()
						Expect(err).ShouldNot(HaveOccurred())
					})

					It("fails with a useful error message because the lock has been acquired by another pipeline run", func() {
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
						}`, gitRepo, shaStr)

						session := runIn(jsonIn, inDestination, 1)

						Expect(session.Err).Should(gbytes.Say("error: lock instance is no longer acquired"))
					})
				})
			})

			Context("when the commit itself unclaimed the lock", func() {
				var shaStr string
				BeforeEach(func() {
					var err error

					unclaimLock := exec.Command("bash", "-e", "-c", `
						git mv lock-pool/claimed/some-lock lock-pool/unclaimed/some-lock
						git commit -m 'unclaiming: some-lock'
					`)
					unclaimLock.Dir = gitRepo

					err = unclaimLock.Run()
					Expect(err).ShouldNot(HaveOccurred())

					gitVersion := exec.Command("git", "rev-parse", "HEAD")
					gitVersion.Dir = gitRepo

					sha, err := gitVersion.Output()
					Expect(err).ShouldNot(HaveOccurred())
					shaStr = strings.TrimSpace(string(sha))

					someOtherCommit := exec.Command("bash", "-e", "-c", `
						git mv lock-pool/unclaimed/some-lock lock-pool/claimed/some-lock
						git commit -m 'doing something some-lock'
					`)
					someOtherCommit.Dir = gitRepo

					err = someOtherCommit.Run()
					Expect(err).ShouldNot(HaveOccurred())
				})

				It("outputs the metadata for the environment", func() {
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
							}`, gitRepo, shaStr)

					session := runIn(jsonIn, inDestination, 0)

					Consistently(session.Err).ShouldNot(gbytes.Say("error: lock instance is no longer acquired"))
					err := json.Unmarshal(session.Out.Contents(), &output)
					Expect(err).ShouldNot(HaveOccurred())

					metaDataFile := filepath.Join(inDestination, "metadata")
					Expect(metaDataFile).Should(BeARegularFile())

					fileContents, err := ioutil.ReadFile(metaDataFile)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(fileContents).Should(MatchJSON(`{"some":"json"}`))

					lockNameFile := filepath.Join(inDestination, "name")
					Expect(lockNameFile).Should(BeARegularFile())

					fileContents, err = ioutil.ReadFile(lockNameFile)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(strings.TrimSpace(string(fileContents))).Should(Equal("some-lock"))

					Expect(output).Should(Equal(inResponse{
						Version: version{
							Ref: shaStr,
						},
						Metadata: []metadataPair{
							{Name: "lock_name", Value: "some-lock"},
							{Name: "pool_name", Value: "lock-pool"},
						},
					}))
				})
			})
		})
	})
})
