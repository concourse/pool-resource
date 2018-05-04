package out

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"
)

type LockPool struct {
	Source Source
	Output io.Writer

	LockHandler LockHandler
	dir         string
}

func NewLockPool(source Source, output io.Writer) LockPool {
	lockPool := LockPool{
		Source: source,
		Output: output,
	}
	lockPool.LockHandler = NewGitLockHandler(source)

	return lockPool
}

//go:generate counterfeiter . LockHandler

type LockHandler interface {
	GrabAvailableLock() (lock string, version string, err error)
	UnclaimLock(lock string) (version string, err error)
	AddLock(lock string, contents []byte, initiallyClaimed bool) (version string, err error)
	RemoveLock(lock string) (version string, err error)
	ClaimLock(lock string) (version string, err error)
	UpdateLock(lock string, contents []byte) (version string, err error)

	Setup() error
	BroadcastLockPool() ([]byte, error)
	ResetLock() error
}

func (lp *LockPool) ClaimLock(lock string) (Version, error) {
	var ref string

	err := lp.performRobustAction(func() (bool, error) {
		var err error

		ref, err = lp.LockHandler.ClaimLock(lock)

		if err == ErrNoLocksAvailable {
			fmt.Fprint(lp.Output, ".")
			return true, nil
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to acquire lock on pool: %s! (err: %s) retrying...\n", lp.Source.Pool, err)
			return true, nil
		}

		return false, nil
	})

	return Version{
		Ref: strings.TrimSpace(ref),
	}, err
}

func (lp *LockPool) AcquireLock() (string, Version, error) {
	var (
		lock string
		ref  string
	)

	fmt.Fprintf(lp.Output, "acquiring lock on: %s\n", lp.Source.Pool)

	err := lp.performRobustAction(func() (bool, error) {
		var err error
		lock, ref, err = lp.LockHandler.GrabAvailableLock()

		if err == ErrNoLocksAvailable {
			fmt.Fprint(lp.Output, ".")
			return true, nil
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to acquire lock on pool: %s! (err: %s) retrying...\n", lp.Source.Pool, err)
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return "", Version{}, err
	}

	return lock, Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (lp *LockPool) ReleaseLock(inDir string) (string, Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", Version{}, err
	}
	lockName := strings.TrimSpace(string(nameFileContents))

	fmt.Fprintf(lp.Output, "releasing lock: %s on pool: %s\n", lockName, lp.Source.Pool)

	var ref string
	err = lp.performRobustAction(func() (bool, error) {
		var err error
		ref, err = lp.LockHandler.UnclaimLock(lockName)

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to unclaim the lock: %s! (err: %s)\n", lockName, err)
			return false, err
		}

		return false, nil
	})

	if err != nil {
		return "", Version{}, err
	}

	return lockName, Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (lp *LockPool) AddClaimedLock(inDir string) (string, Version, error) {
	return lp.addLock(inDir, true)
}

func (lp *LockPool) AddUnclaimedLock(inDir string) (string, Version, error) {
	return lp.addLock(inDir, false)
}

func (lp *LockPool) addLock(inDir string, initiallyClaimed bool) (string, Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", Version{}, fmt.Errorf("could not read the name file of your lock: %s", err)
	}
	lockName := strings.TrimSpace(string(nameFileContents))

	lockContents, err := ioutil.ReadFile(filepath.Join(inDir, "metadata"))
	if err != nil {
		return "", Version{}, fmt.Errorf("could not read the metadata file of your lock: %s", err)
	}

	if initiallyClaimed {
		fmt.Fprintf(lp.Output, "adding claimed lock: %s to pool: %s\n", lockName, lp.Source.Pool)
	} else {
		fmt.Fprintf(lp.Output, "adding unclaimed lock: %s to pool: %s\n", lockName, lp.Source.Pool)
	}

	var ref string

	err = lp.performRobustAction(func() (bool, error) {
		var err error
		ref, err = lp.LockHandler.AddLock(lockName, lockContents, initiallyClaimed)

		if err != nil {
			fmt.Fprintf(lp.Output, "failed to add the lock: %s! (err: %s) retrying...\n", lockName, err)
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return "", Version{}, err
	}

	return lockName, Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (lp *LockPool) RemoveLock(inDir string) (string, Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", Version{}, err
	}

	lockName := strings.TrimSpace(string(nameFileContents))

	fmt.Fprintf(lp.Output, "removing lock: %s on pool: %s\n", lockName, lp.Source.Pool)

	var ref string

	err = lp.performRobustAction(func() (bool, error) {
		var err error
		ref, err = lp.LockHandler.RemoveLock(lockName)

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to remove the lock: %s! (err: %s)\n", lockName, err)
			return false, err
		}

		return false, nil
	})

	if err != nil {
		return "", Version{}, err
	}

	return lockName, Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (lp *LockPool) UpdateLock(inDir string) (string, Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", Version{}, fmt.Errorf("could not read the name file of your lock: %s", err)
	}
	lockName := strings.TrimSpace(string(nameFileContents))

	lockContents, err := ioutil.ReadFile(filepath.Join(inDir, "metadata"))
	if err != nil {
		return "", Version{}, fmt.Errorf("could not read the metadata file of your lock: %s", err)
	}

	fmt.Fprintf(lp.Output, "updating lock: %s in pool: %s\n", lockName, lp.Source.Pool)

	var ref string

	err = lp.performRobustAction(func() (bool, error) {
		var err error
		ref, err = lp.LockHandler.UpdateLock(lockName, lockContents)

		if err == ErrNoLocksAvailable {
			fmt.Fprint(lp.Output, ".")
			return true, nil
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "failed to update the lock: %s! (err: %s) retrying...\n", lockName, err)
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return "", Version{}, err
	}

	return lockName, Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (lp *LockPool) performRobustAction(action func() (bool, error)) error {
	err := lp.LockHandler.Setup()
	if err != nil {
		return err
	}

	var gitOutput []byte
	unexpectedErrorRetry := 0
	for unexpectedErrorRetry < 5 {
		err = lp.LockHandler.ResetLock()
		if err != nil {
			return err
		}

		retry, err := action()

		if err != nil {
			return err
		}

		if retry {
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		gitOutput, err = lp.LockHandler.BroadcastLockPool()

		if err == ErrLockConflict {
			fmt.Fprint(lp.Output, ".")
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		if err != nil {
			unexpectedErrorRetry++
			fmt.Fprintf(lp.Output, "\nfailed to broadcast the change to lock state!\nerr: %s\ngit-err: %s\nretrying...\n", err, gitOutput)
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		break
	}

	if unexpectedErrorRetry == 5 {
		return errors.New("too-many-unexpected-errors")
	}

	return nil
}
