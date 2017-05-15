package out_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/concourse/pool-resource/out"
	fakes "github.com/concourse/pool-resource/out/fakes"
)

var _ = Describe("Lock Pool", func() {
	var lockPool out.LockPool
	var fakeLockHandler *fakes.FakeLockHandler
	var output *gbytes.Buffer

	ValidateSharedBehaviorDuringBroadcastFailures := func(operationUnderTest func() error, additionalValidation func(int)) {

		ValidateInteractionsWithLockHandler := func(expectedNumberOfInteractions int) {
			Expect(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(expectedNumberOfInteractions))
			additionalValidation(expectedNumberOfInteractions)
		}

		Context("when broadcasting fails with ", func() {
			Context("for an unexpected reason", func() {
				BeforeEach(func() {
					called := false

					fakeLockHandler.BroadcastLockPoolStub = func() ([]byte, error) {
						// succeed on second call
						if !called {
							called = true
							return nil, errors.New("disaster")
						} else {
							return nil, nil
						}
					}
				})

				It("logs an error as it retries", func() {
					err := operationUnderTest()
					Expect(err).ShouldNot(HaveOccurred())

					Expect(output).Should(gbytes.Say("err"))

					ValidateInteractionsWithLockHandler(2)
				})

				Context("more than 5 times", func() {
					BeforeEach(func() {
						fakeLockHandler.BroadcastLockPoolReturns([]byte("some git message"), errors.New("disaster"))
					})

					It("shows the underlying git error", func() {
						err := operationUnderTest()
						Expect(err).Should(HaveOccurred())

						Expect(output).Should(gbytes.Say("some git message"))

						ValidateInteractionsWithLockHandler(5)
					})
				})
			})

			Context("for an expected reason", func() {
				BeforeEach(func() {
					called := false

					fakeLockHandler.BroadcastLockPoolStub = func() ([]byte, error) {
						// succeed on second call
						if !called {
							called = true
							return nil, out.ErrLockConflict
						} else {
							return nil, nil
						}
					}
				})

				It("does not log an error as it retries", func() {
					err := operationUnderTest()
					Expect(err).ShouldNot(HaveOccurred())

					// no logging for expected errors
					Expect(output).ShouldNot(gbytes.Say("err"))

					ValidateInteractionsWithLockHandler(2)
				})
			})
		})
	}

	BeforeEach(func() {
		fakeLockHandler = new(fakes.FakeLockHandler)

		output = gbytes.NewBuffer()

		lockPool = out.LockPool{
			Source: out.Source{
				URI:        "some-uri",
				Pool:       "my-pool",
				Branch:     "some-branch",
				RetryDelay: 100 * time.Millisecond,
			},
			Output:      output,
			LockHandler: fakeLockHandler,
		}
	})

	Context("Removing a lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = ioutil.TempDir("", "lock-dir")
			Expect(err).ShouldNot(HaveOccurred())

		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Expect(err).ShouldNot(HaveOccurred())
		})

		Context("when a name file doesn't exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.RemoveLock(lockDir)
				Expect(err).Should(HaveOccurred())
			})
		})

		Context("when a name file does exist", func() {
			BeforeEach(func() {
				err := ioutil.WriteFile(filepath.Join(lockDir, "name"), []byte("some-remove-lock"), 0755)
				Expect(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.RemoveLock(lockDir)
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to reset the lock state", func() {
					_, _, err := lockPool.RemoveLock(lockDir)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(fakeLockHandler.ResetLockCallCount()).Should(Equal(1))
				})

				Context("when resetting the lock state fails", func() {
					BeforeEach(func() {
						fakeLockHandler.ResetLockReturns(errors.New("some-error"))
					})

					It("returns an error", func() {
						_, _, err := lockPool.RemoveLock(lockDir)
						Expect(err).Should(HaveOccurred())
					})
				})

				Context("when resetting the lock state succeeds", func() {
					It("tries to remove the lock it found in the name file", func() {
						_, _, err := lockPool.RemoveLock(lockDir)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.RemoveLockCallCount()).Should(Equal(1))
						lockName := fakeLockHandler.RemoveLockArgsForCall(0)
						Expect(lockName).Should(Equal("some-remove-lock"))
					})

					Context("when removing the lock fails", func() {
						BeforeEach(func() {
							fakeLockHandler.RemoveLockReturns("", errors.New("disaster"))
						})

						It("returns an error", func() {
							_, _, err := lockPool.RemoveLock(lockDir)
							Expect(err).Should(HaveOccurred())
							Expect(fakeLockHandler.RemoveLockCallCount()).Should(Equal(1))
						})
					})

					Context("when removing the lock succeeds", func() {
						BeforeEach(func() {
							fakeLockHandler.RemoveLockReturns("some-ref", nil)
						})

						It("tries to broadcast to the lock pool", func() {
							_, _, err := lockPool.RemoveLock(lockDir)
							Expect(err).ShouldNot(HaveOccurred())

							Expect(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
						})

						ValidateSharedBehaviorDuringBroadcastFailures(
							func() error {
								_, _, err := lockPool.RemoveLock(lockDir)
								return err
							}, func(expectedNumberOfInteractions int) {
								Expect(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
								Expect(fakeLockHandler.RemoveLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							})

						Context("when broadcasting succeeds", func() {
							It("returns the lockname, and a version", func() {
								lockName, version, err := lockPool.RemoveLock(lockDir)

								Expect(err).ShouldNot(HaveOccurred())
								Expect(lockName).Should(Equal("some-remove-lock"))
								Expect(version).Should(Equal(out.Version{
									Ref: "some-ref",
								}))
							})
						})
					})
				})
			})
		})
	})

	Context("Acquiring a lock", func() {
		Context("when setup fails", func() {
			BeforeEach(func() {
				fakeLockHandler.SetupReturns(errors.New("some-error"))
			})

			It("returns an error", func() {
				_, _, err := lockPool.AcquireLock()
				Expect(err).Should(HaveOccurred())
			})
		})

		Context("when setup succeeds", func() {

			Context("when resetting the lock fails", func() {
				BeforeEach(func() {
					fakeLockHandler.ResetLockReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.AcquireLock()
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("when resetting the lock succeeds", func() {
				It("tries to acquire an available lock", func() {
					_, _, err := lockPool.AcquireLock()
					Expect(err).ShouldNot(HaveOccurred())

					Expect(fakeLockHandler.GrabAvailableLockCallCount()).Should(Equal(1))
				})

				Context("when grabbing an available lock fails", func() {
					BeforeEach(func() {
						called := false

						fakeLockHandler.GrabAvailableLockStub = func() (string, string, error) {
							// succeed on second call
							if !called {
								called = true
								return "", "", errors.New("disaster")
							} else {
								return "", "", nil
							}
						}
					})

					It("retries", func() {
						_, _, err := lockPool.AcquireLock()
						Expect(err).ShouldNot(HaveOccurred())
						Expect(fakeLockHandler.GrabAvailableLockCallCount()).Should(Equal(2))
					})
				})

				Context("when grabbing an available lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.GrabAvailableLockReturns("some-lock", "some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.AcquireLock()
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.AcquireLock()
							return err
						}, func(expectedNumberOfInteractions int) {
							Expect(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							Expect(fakeLockHandler.GrabAvailableLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.AcquireLock()

							Expect(err).ShouldNot(HaveOccurred())
							Expect(lockName).Should(Equal("some-lock"))
							Expect(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})

	Context("Claiming a lock", func() {
		Context("when setup fails", func() {
			BeforeEach(func() {
				fakeLockHandler.SetupReturns(errors.New("some-error"))
			})

			It("returns an error", func() {
				_, err := lockPool.ClaimLock("some-lock")
				Expect(err).Should(HaveOccurred())
			})
		})

		Context("when setup succeeds", func() {

			Context("when resetting the lock fails", func() {
				BeforeEach(func() {
					fakeLockHandler.ResetLockReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, err := lockPool.ClaimLock("some-lock")
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("when resetting the lock succeeds", func() {
				It("tries to claim the specific lock", func() {
					_, err := lockPool.ClaimLock("some-lock")
					Expect(err).ShouldNot(HaveOccurred())

					Expect(fakeLockHandler.ClaimLockCallCount()).Should(Equal(1))
					Expect(fakeLockHandler.ClaimLockArgsForCall(0)).Should(Equal("some-lock"))
				})

				Context("when attempting to claim a lock fails", func() {
					BeforeEach(func() {
						called := false

						fakeLockHandler.ClaimLockStub = func(lock string) (string, error) {
							// succeed on second call
							if !called {
								called = true
								return "", errors.New("disaster")
							} else {
								return "", nil
							}
						}
					})

					It("retries", func() {
						_, err := lockPool.ClaimLock("some-lock")
						Expect(err).ShouldNot(HaveOccurred())
						Expect(fakeLockHandler.ClaimLockCallCount()).Should(Equal(2))
					})
				})

				Context("when claiming a specific lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.ClaimLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, err := lockPool.ClaimLock("some-lock")
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, err := lockPool.ClaimLock("some-lock")
							return err
						}, func(expectedNumberOfInteractions int) {
							Expect(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							Expect(fakeLockHandler.ClaimLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the a version", func() {
							version, err := lockPool.ClaimLock("some-lock")

							Expect(err).ShouldNot(HaveOccurred())
							Expect(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})

	Context("Releasing a lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = ioutil.TempDir("", "lock-dir")
			Expect(err).ShouldNot(HaveOccurred())

		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Expect(err).ShouldNot(HaveOccurred())
		})

		Context("when a name file doesn't exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.ReleaseLock(lockDir)
				Expect(err).Should(HaveOccurred())
			})
		})

		Context("when a name file does exist", func() {
			BeforeEach(func() {
				err := ioutil.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Expect(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.ReleaseLock(lockDir)
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to unclaim the lock it found in the name file", func() {
					_, _, err := lockPool.ReleaseLock(lockDir)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(1))
					lockName := fakeLockHandler.UnclaimLockArgsForCall(0)
					Expect(lockName).Should(Equal("some-lock"))
				})

				Context("when unclaiming the lock fails", func() {
					BeforeEach(func() {
						fakeLockHandler.UnclaimLockReturns("", errors.New("disaster"))
					})

					It("returns an error", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Expect(err).Should(HaveOccurred())
						Expect(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(1))
					})
				})

				Context("when unclaiming the lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.UnclaimLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.ReleaseLock(lockDir)
							return err
						}, func(expectedNumberOfInteractions int) {
							Expect(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							Expect(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.ReleaseLock(lockDir)

							Expect(err).ShouldNot(HaveOccurred())
							Expect(lockName).Should(Equal("some-lock"))
							Expect(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})

	Context("adding an initially unclaimed lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = ioutil.TempDir("", "lock-dir")
			Expect(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Expect(err).ShouldNot(HaveOccurred())
		})

		Context("when no files exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.AddUnclaimedLock(lockDir)
				Expect(err).Should(HaveOccurred())
			})
		})

		Context("when a name and metadata file does exist", func() {
			BeforeEach(func() {
				err := ioutil.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Expect(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(lockDir, "metadata"), []byte("lock-contents"), 0755)
				Expect(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.AddUnclaimedLock(lockDir)
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to add the lock it found in the name file", func() {
					_, _, err := lockPool.AddUnclaimedLock(lockDir)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(fakeLockHandler.AddLockCallCount()).Should(Equal(1))
					lockName, lockContents, initiallyClaimed := fakeLockHandler.AddLockArgsForCall(0)
					Expect(lockName).Should(Equal("some-lock"))
					Expect(string(lockContents)).Should(Equal("lock-contents"))
					Expect(initiallyClaimed).Should(BeFalse())
				})

				Context("when adding the lock fails", func() {
					BeforeEach(func() {
						called := false

						fakeLockHandler.AddLockStub = func(_ string, _ []byte, _ bool) (string, error) {
							// succeed on second call
							if !called {
								called = true
								return "", errors.New("disaster")
							} else {
								return "some-ref", nil
							}
						}
					})

					It("does not return an error as it retries", func() {
						_, _, err := lockPool.AddUnclaimedLock(lockDir)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.AddLockCallCount()).Should(Equal(2))
					})
				})

				Context("when adding the lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.AddLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.AddUnclaimedLock(lockDir)
							return err
						}, func(expectedNumberOfInteractions int) {
							Expect(fakeLockHandler.AddLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.AddUnclaimedLock(lockDir)

							Expect(err).ShouldNot(HaveOccurred())
							Expect(lockName).Should(Equal("some-lock"))
							Expect(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})

	Context("adding an initially claimed lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = ioutil.TempDir("", "lock-dir")
			Expect(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Expect(err).ShouldNot(HaveOccurred())
		})

		Context("when no files exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.AddClaimedLock(lockDir)
				Expect(err).Should(HaveOccurred())
			})
		})

		Context("when a name and metadata file does exist", func() {
			BeforeEach(func() {
				err := ioutil.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Expect(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(lockDir, "metadata"), []byte("lock-contents"), 0755)
				Expect(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.AddClaimedLock(lockDir)
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to add the lock it found in the name file", func() {
					_, _, err := lockPool.AddClaimedLock(lockDir)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(fakeLockHandler.AddLockCallCount()).Should(Equal(1))
					lockName, lockContents, initiallyClaimed := fakeLockHandler.AddLockArgsForCall(0)
					Expect(lockName).Should(Equal("some-lock"))
					Expect(string(lockContents)).Should(Equal("lock-contents"))
					Expect(initiallyClaimed).Should(BeTrue())
				})

				Context("when adding the lock fails", func() {
					BeforeEach(func() {
						called := false

						fakeLockHandler.AddLockStub = func(_ string, _ []byte, _ bool) (string, error) {
							// succeed on second call
							if !called {
								called = true
								return "", errors.New("disaster")
							} else {
								return "some-ref", nil
							}
						}
					})

					It("does not return an error as it retries", func() {
						_, _, err := lockPool.AddClaimedLock(lockDir)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.AddLockCallCount()).Should(Equal(2))
					})
				})

				Context("when adding the lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.AddLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.AddClaimedLock(lockDir)
							return err
						}, func(expectedNumberOfInteractions int) {
							Expect(fakeLockHandler.AddLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.AddClaimedLock(lockDir)

							Expect(err).ShouldNot(HaveOccurred())
							Expect(lockName).Should(Equal("some-lock"))
							Expect(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})
})
