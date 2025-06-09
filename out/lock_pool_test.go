package out_test

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/ebroberson/pool-resource/out"
	fakes "github.com/ebroberson/pool-resource/out/outfakes"
)

var _ = Describe("Lock Pool", func() {
	var lockPool out.LockPool
	var fakeLockHandler *fakes.FakeLockHandler
	var output *gbytes.Buffer

	ValidateSharedBehaviorDuringBroadcastFailures := func(operationUnderTest func() error, additionalValidation func(int)) {

		ValidateInteractionsWithLockHandler := func(expectedNumberOfInteractions int) {
			Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(expectedNumberOfInteractions))
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
					Ω(err).ShouldNot(HaveOccurred())

					Ω(output).Should(gbytes.Say("err"))

					ValidateInteractionsWithLockHandler(2)
				})

				Context("more than 5 times", func() {
					BeforeEach(func() {
						fakeLockHandler.BroadcastLockPoolReturns([]byte("some git message"), errors.New("disaster"))
					})

					It("shows the underlying git error", func() {
						err := operationUnderTest()
						Ω(err).Should(HaveOccurred())

						Ω(output).Should(gbytes.Say("some git message"))

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
					Ω(err).ShouldNot(HaveOccurred())

					// no logging for expected errors
					Ω(output).ShouldNot(gbytes.Say("err"))

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
			lockDir, err = os.MkdirTemp("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())

		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when a name file doesn't exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.RemoveLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name file does exist", func() {
			BeforeEach(func() {
				err := os.WriteFile(filepath.Join(lockDir, "name"), []byte("some-remove-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.RemoveLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to reset the lock state", func() {
					_, _, err := lockPool.RemoveLock(lockDir)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeLockHandler.ResetLockCallCount()).Should(Equal(1))
				})

				Context("when resetting the lock state fails", func() {
					BeforeEach(func() {
						fakeLockHandler.ResetLockReturns(errors.New("some-error"))
					})

					It("returns an error", func() {
						_, _, err := lockPool.RemoveLock(lockDir)
						Ω(err).Should(HaveOccurred())
					})
				})

				Context("when resetting the lock state succeeds", func() {
					It("tries to remove the lock it found in the name file", func() {
						_, _, err := lockPool.RemoveLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.RemoveLockCallCount()).Should(Equal(1))
						lockName := fakeLockHandler.RemoveLockArgsForCall(0)
						Ω(lockName).Should(Equal("some-remove-lock"))
					})

					Context("when removing the lock fails", func() {
						BeforeEach(func() {
							fakeLockHandler.RemoveLockReturns("", errors.New("disaster"))
						})

						It("returns an error", func() {
							_, _, err := lockPool.RemoveLock(lockDir)
							Ω(err).Should(HaveOccurred())
							Ω(fakeLockHandler.RemoveLockCallCount()).Should(Equal(1))
						})
					})

					Context("when removing the lock succeeds", func() {
						BeforeEach(func() {
							fakeLockHandler.RemoveLockReturns("some-ref", nil)
						})

						It("tries to broadcast to the lock pool", func() {
							_, _, err := lockPool.RemoveLock(lockDir)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
						})

						ValidateSharedBehaviorDuringBroadcastFailures(
							func() error {
								_, _, err := lockPool.RemoveLock(lockDir)
								return err
							}, func(expectedNumberOfInteractions int) {
								Ω(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
								Ω(fakeLockHandler.RemoveLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							})

						Context("when broadcasting succeeds", func() {
							It("returns the lockname, and a version", func() {
								lockName, version, err := lockPool.RemoveLock(lockDir)

								Ω(err).ShouldNot(HaveOccurred())
								Ω(lockName).Should(Equal("some-remove-lock"))
								Ω(version).Should(Equal(out.Version{
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
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when setup succeeds", func() {

			Context("when resetting the lock fails", func() {
				BeforeEach(func() {
					fakeLockHandler.ResetLockReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.AcquireLock()
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when resetting the lock succeeds", func() {
				It("tries to acquire an available lock", func() {
					_, _, err := lockPool.AcquireLock()
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeLockHandler.GrabAvailableLockCallCount()).Should(Equal(1))
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
						Ω(err).ShouldNot(HaveOccurred())
						Ω(fakeLockHandler.GrabAvailableLockCallCount()).Should(Equal(2))
					})
				})

				Context("when grabbing an available lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.GrabAvailableLockReturns("some-lock", "some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.AcquireLock()
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.AcquireLock()
							return err
						}, func(expectedNumberOfInteractions int) {
							Ω(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							Ω(fakeLockHandler.GrabAvailableLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.AcquireLock()

							Ω(err).ShouldNot(HaveOccurred())
							Ω(lockName).Should(Equal("some-lock"))
							Ω(version).Should(Equal(out.Version{
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
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when setup succeeds", func() {

			Context("when resetting the lock fails", func() {
				BeforeEach(func() {
					fakeLockHandler.ResetLockReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, err := lockPool.ClaimLock("some-lock")
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when resetting the lock succeeds", func() {
				It("tries to claim the specific lock", func() {
					_, err := lockPool.ClaimLock("some-lock")
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeLockHandler.ClaimLockCallCount()).Should(Equal(1))
					Ω(fakeLockHandler.ClaimLockArgsForCall(0)).Should(Equal("some-lock"))
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
						Ω(err).ShouldNot(HaveOccurred())
						Ω(fakeLockHandler.ClaimLockCallCount()).Should(Equal(2))
					})
				})

				Context("when claiming a specific lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.ClaimLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, err := lockPool.ClaimLock("some-lock")
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, err := lockPool.ClaimLock("some-lock")
							return err
						}, func(expectedNumberOfInteractions int) {
							Ω(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							Ω(fakeLockHandler.ClaimLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the a version", func() {
							version, err := lockPool.ClaimLock("some-lock")

							Ω(err).ShouldNot(HaveOccurred())
							Ω(version).Should(Equal(out.Version{
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
			lockDir, err = os.MkdirTemp("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())

		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when a name file doesn't exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.ReleaseLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name file does exist", func() {
			BeforeEach(func() {
				err := os.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.ReleaseLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to unclaim the lock it found in the name file", func() {
					_, _, err := lockPool.ReleaseLock(lockDir)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(1))
					lockName := fakeLockHandler.UnclaimLockArgsForCall(0)
					Ω(lockName).Should(Equal("some-lock"))
				})

				Context("when unclaiming the lock fails", func() {
					BeforeEach(func() {
						fakeLockHandler.UnclaimLockReturns("", errors.New("disaster"))
					})

					It("returns an error", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Ω(err).Should(HaveOccurred())
						Ω(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(1))
					})
				})

				Context("when unclaiming the lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.UnclaimLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.ReleaseLock(lockDir)
							return err
						}, func(expectedNumberOfInteractions int) {
							Ω(fakeLockHandler.ResetLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							Ω(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.ReleaseLock(lockDir)

							Ω(err).ShouldNot(HaveOccurred())
							Ω(lockName).Should(Equal("some-lock"))
							Ω(version).Should(Equal(out.Version{
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
			lockDir, err = os.MkdirTemp("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when no files exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.AddUnclaimedLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name and metadata file does exist", func() {
			BeforeEach(func() {
				err := os.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.WriteFile(filepath.Join(lockDir, "metadata"), []byte("lock-contents"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.AddUnclaimedLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to add the lock it found in the name file", func() {
					_, _, err := lockPool.AddUnclaimedLock(lockDir)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(1))
					lockName, lockContents, initiallyClaimed := fakeLockHandler.AddLockArgsForCall(0)
					Ω(lockName).Should(Equal("some-lock"))
					Ω(string(lockContents)).Should(Equal("lock-contents"))
					Ω(initiallyClaimed).Should(BeFalse())
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
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(2))
					})
				})

				Context("when adding the lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.AddLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.AddUnclaimedLock(lockDir)
							return err
						}, func(expectedNumberOfInteractions int) {
							Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.AddUnclaimedLock(lockDir)

							Ω(err).ShouldNot(HaveOccurred())
							Ω(lockName).Should(Equal("some-lock"))
							Ω(version).Should(Equal(out.Version{
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
			lockDir, err = os.MkdirTemp("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when no files exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.AddClaimedLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name and metadata file does exist", func() {
			BeforeEach(func() {
				err := os.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.WriteFile(filepath.Join(lockDir, "metadata"), []byte("lock-contents"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.AddClaimedLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to add the lock it found in the name file", func() {
					_, _, err := lockPool.AddClaimedLock(lockDir)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(1))
					lockName, lockContents, initiallyClaimed := fakeLockHandler.AddLockArgsForCall(0)
					Ω(lockName).Should(Equal("some-lock"))
					Ω(string(lockContents)).Should(Equal("lock-contents"))
					Ω(initiallyClaimed).Should(BeTrue())
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
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(2))
					})
				})

				Context("when adding the lock succeeds", func() {
					BeforeEach(func() {
						fakeLockHandler.AddLockReturns("some-ref", nil)
					})

					It("tries to broadcast to the lock pool", func() {
						_, _, err := lockPool.ReleaseLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					ValidateSharedBehaviorDuringBroadcastFailures(
						func() error {
							_, _, err := lockPool.AddClaimedLock(lockDir)
							return err
						}, func(expectedNumberOfInteractions int) {
							Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(expectedNumberOfInteractions))
						})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.AddClaimedLock(lockDir)

							Ω(err).ShouldNot(HaveOccurred())
							Ω(lockName).Should(Equal("some-lock"))
							Ω(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})

	Context("Updating a lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = os.MkdirTemp("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when no files exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.UpdateLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name and metadata file does exist", func() {
			BeforeEach(func() {
				err := os.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = os.WriteFile(filepath.Join(lockDir, "metadata"), []byte("lock-contents"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.UpdateLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				Context("when resetting the lock fails", func() {
					BeforeEach(func() {
						fakeLockHandler.ResetLockReturns(errors.New("some-error"))
					})

					It("returns an error", func() {
						_, _, err := lockPool.UpdateLock("some-lock")
						Ω(err).Should(HaveOccurred())
					})
				})

				Context("when resetting the lock succeeds", func() {
					It("tries to update the lock it found in the name file", func() {
						_, _, err := lockPool.UpdateLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.UpdateLockCallCount()).Should(Equal(1))
						lockName, lockContents := fakeLockHandler.UpdateLockArgsForCall(0)
						Ω(lockName).Should(Equal("some-lock"))
						Ω(string(lockContents)).Should(Equal("lock-contents"))
					})

					Context("when updating the lock fails", func() {
						BeforeEach(func() {
							called := false

							fakeLockHandler.UpdateLockStub = func(_ string, _ []byte) (string, error) {
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
							_, _, err := lockPool.UpdateLock(lockDir)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(fakeLockHandler.UpdateLockCallCount()).Should(Equal(2))
						})
					})

					Context("when updating the lock succeeds", func() {
						BeforeEach(func() {
							fakeLockHandler.UpdateLockReturns("some-ref", nil)
						})

						It("tries to broadcast to the lock pool", func() {
							_, _, err := lockPool.ReleaseLock(lockDir)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
						})

						ValidateSharedBehaviorDuringBroadcastFailures(
							func() error {
								_, _, err := lockPool.UpdateLock(lockDir)
								return err
							}, func(expectedNumberOfInteractions int) {
								Ω(fakeLockHandler.UpdateLockCallCount()).Should(Equal(expectedNumberOfInteractions))
							})

						Context("when broadcasting succeeds", func() {
							It("returns the lockname, and a version", func() {
								lockName, version, err := lockPool.UpdateLock(lockDir)

								Ω(err).ShouldNot(HaveOccurred())
								Ω(lockName).Should(Equal("some-lock"))
								Ω(version).Should(Equal(out.Version{
									Ref: "some-ref",
								}))
							})
						})
					})
				})
			})
		})
	})

	Context("Checking a lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = os.MkdirTemp("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())

		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when a name file doesn't exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.CheckLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name file does exist", func() {
			BeforeEach(func() {
				err := os.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.CheckLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				Context("when the lock is unclaimed", func() {
					BeforeEach(func() {
						fakeLockHandler.CheckLockReturns("some-ref", nil)
					})

					It("bypasses broadcasting to the lock pool", func() {
						_, _, err := lockPool.CheckLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.CheckLock(lockDir)

							Ω(err).ShouldNot(HaveOccurred())
							Ω(lockName).Should(Equal("some-lock"))
							Ω(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})

	Context("Checking an unclaimed lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = os.MkdirTemp("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())

		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when a name file doesn't exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.CheckUnclaimedLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name file does exist", func() {
			BeforeEach(func() {
				err := os.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.CheckUnclaimedLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				Context("when the lock is claimed", func() {
					BeforeEach(func() {
						fakeLockHandler.CheckUnclaimedLockReturns("some-ref", nil)
					})

					It("bypasses broadcasting to the lock pool", func() {
						_, _, err := lockPool.CheckUnclaimedLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(1))
					})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.CheckUnclaimedLock(lockDir)

							Ω(err).ShouldNot(HaveOccurred())
							Ω(lockName).Should(Equal("some-lock"))
							Ω(version).Should(Equal(out.Version{
								Ref: "some-ref",
							}))
						})
					})
				})
			})
		})
	})
})
