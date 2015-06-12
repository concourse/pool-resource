package out_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/concourse/pool-resource/out"
	fakes "github.com/concourse/pool-resource/out/fakes"
)

var _ = Describe("Lock Pool", func() {
	var lockPool out.LockPool
	var fakeLockHandler *fakes.FakeLockHandler

	BeforeEach(func() {
		fakeLockHandler = new(fakes.FakeLockHandler)

		lockPool = out.LockPool{
			Source: out.Source{
				URI:        "some-uri",
				Pool:       "my-pool",
				Branch:     "some-branch",
				RetryDelay: 100 * time.Millisecond,
			},
			Output:      GinkgoWriter,
			LockHandler: fakeLockHandler,
		}
	})

	Context("Releasing a lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = ioutil.TempDir("", "lock-dir")
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
				err := ioutil.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
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
						called := false

						fakeLockHandler.UnclaimLockStub = func(lockName string) (string, error) {
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
						_, _, err := lockPool.ReleaseLock(lockDir)
						Ω(err).ShouldNot(HaveOccurred())

						Ω(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(2))
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

					Context("when broadcasting fails", func() {
						BeforeEach(func() {
							called := false

							fakeLockHandler.BroadcastLockPoolStub = func() error {
								// succeed on second call
								if !called {
									called = true
									return errors.New("disaster")
								} else {
									return nil
								}
							}
						})

						It("does not return an error as it retries", func() {
							_, _, err := lockPool.ReleaseLock(lockDir)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(fakeLockHandler.UnclaimLockCallCount()).Should(Equal(2))
							Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(2))
						})
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

	Context("adding a lock", func() {
		var lockDir string

		BeforeEach(func() {
			var err error
			lockDir, err = ioutil.TempDir("", "lock-dir")
			Ω(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			err := os.RemoveAll(lockDir)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when a no files file doesn't exist", func() {
			It("returns an error", func() {
				_, _, err := lockPool.AddLock(lockDir)
				Ω(err).Should(HaveOccurred())
			})
		})

		Context("when a name and metadata file does exist", func() {
			BeforeEach(func() {
				err := ioutil.WriteFile(filepath.Join(lockDir, "name"), []byte("some-lock"), 0755)
				Ω(err).ShouldNot(HaveOccurred())

				err = ioutil.WriteFile(filepath.Join(lockDir, "metadata"), []byte("lock-contents"), 0755)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("when setup fails", func() {
				BeforeEach(func() {
					fakeLockHandler.SetupReturns(errors.New("some-error"))
				})

				It("returns an error", func() {
					_, _, err := lockPool.AddLock(lockDir)
					Ω(err).Should(HaveOccurred())
				})
			})

			Context("when setup succeeds", func() {
				It("tries to add the lock it found in the name file", func() {
					_, _, err := lockPool.AddLock(lockDir)
					Ω(err).ShouldNot(HaveOccurred())

					Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(1))
					lockName, lockContents := fakeLockHandler.AddLockArgsForCall(0)
					Ω(lockName).Should(Equal("some-lock"))
					Ω(string(lockContents)).Should(Equal("lock-contents"))
				})

				Context("when adding the lock fails", func() {
					BeforeEach(func() {
						called := false

						fakeLockHandler.AddLockStub = func(lockName string, lockContents []byte) (string, error) {
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
						_, _, err := lockPool.AddLock(lockDir)
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

					Context("when broadcasting fails", func() {
						BeforeEach(func() {
							called := false

							fakeLockHandler.BroadcastLockPoolStub = func() error {
								// succeed on second call
								if !called {
									called = true
									return errors.New("disaster")
								} else {
									return nil
								}
							}
						})

						It("does not return an error as it retries", func() {
							_, _, err := lockPool.AddLock(lockDir)
							Ω(err).ShouldNot(HaveOccurred())

							Ω(fakeLockHandler.AddLockCallCount()).Should(Equal(2))
							Ω(fakeLockHandler.BroadcastLockPoolCallCount()).Should(Equal(2))
						})
					})

					Context("when broadcasting succeeds", func() {
						It("returns the lockname, and a version", func() {
							lockName, version, err := lockPool.AddLock(lockDir)

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
