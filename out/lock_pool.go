package out

import (
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
	AddLock(lock string, contents []byte) (version string, err error)
	RemoveLock(lock string) (version string, err error)

	Setup() error
	BroadcastLockPool() error
	ResetLock() error
}

func (lp *LockPool) AcquireLock() (string, Version, error) {
	err := lp.LockHandler.Setup()
	if err != nil {
		return "", Version{}, err
	}

	var (
		lock string
		ref  string
	)

	fmt.Fprintf(lp.Output, "acquiring lock on: %s\n", lp.Source.Pool)

	for {
		err = lp.LockHandler.ResetLock()
		if err != nil {
			return "", Version{}, err
		}

		lock, ref, err = lp.LockHandler.GrabAvailableLock()

		if err == ErrNoLocksAvailable {
			fmt.Fprint(lp.Output, ".")
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to acquire lock on pool: %s! (err: %s) retrying...\n", lp.Source.Pool, err)
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		err = lp.LockHandler.BroadcastLockPool()

		if err == ErrLockConflict {
			fmt.Fprint(lp.Output, ".")
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to broadcast the change to lock state! (err: %s) retrying...\n", err)
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		break
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

	err = lp.LockHandler.Setup()
	if err != nil {
		return "", Version{}, err
	}

	var ref string
	for {
		err = lp.LockHandler.ResetLock()
		if err != nil {
			return "", Version{}, err
		}

		ref, err = lp.LockHandler.UnclaimLock(lockName)
		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to unclaim the lock: %s! (err: %s)\n", lockName, err)
			return "", Version{}, err
		}

		err = lp.LockHandler.BroadcastLockPool()

		if err == ErrLockConflict {
			fmt.Fprint(lp.Output, ".")
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to broadcast the change to lock state! (err: %s) retrying...\n", err)
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		break
	}

	return lockName, Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (lp *LockPool) AddLock(inDir string) (string, Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", Version{}, fmt.Errorf("could not read the name file of your lock: %s", err)
	}

	lockName := strings.TrimSpace(string(nameFileContents))

	lockContents, err := ioutil.ReadFile(filepath.Join(inDir, "metadata"))
	if err != nil {
		return "", Version{}, fmt.Errorf("could not read the metadata file of your lock: %s", err)
	}

	fmt.Fprintf(lp.Output, "adding lock: %s to pool: %s\n", lockName, lp.Source.Pool)

	err = lp.LockHandler.Setup()
	if err != nil {
		return "", Version{}, err
	}

	var ref string
	for {
		err = lp.LockHandler.ResetLock()
		if err != nil {
			return "", Version{}, err
		}

		ref, err = lp.LockHandler.AddLock(lockName, lockContents)
		if err != nil {
			fmt.Fprintf(lp.Output, "failed to add the lock: %s! (err: %s) retrying...\n", lockName, err)
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		err = lp.LockHandler.BroadcastLockPool()

		if err == ErrLockConflict {
			fmt.Fprint(lp.Output, ".")
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to broadcast the change to lock state! (err: %s) retrying...\n", err)
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		break
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

	err = lp.LockHandler.Setup()
	if err != nil {
		return "", Version{}, err
	}

	var ref string

	for {
		err = lp.LockHandler.ResetLock()
		if err != nil {
			fmt.Fprintf(lp.Output, "failed to reset the lock: %s! (err: %s)\n", lockName, err)
			return "", Version{}, err
		}

		ref, err = lp.LockHandler.RemoveLock(lockName)
		if err != nil {
			fmt.Fprintf(lp.Output, "failed to remove the lock: %s! (err: %s)\n", lockName, err)
			return "", Version{}, err
		}

		err = lp.LockHandler.BroadcastLockPool()

		if err == ErrLockConflict {
			fmt.Fprintf(lp.Output, ".")
			time.Sleep(lp.Source.RetryDelay)
			continue
		}

		if err != nil {
			fmt.Fprintf(lp.Output, "\nfailed to broadcast the change to lock state! (err: %s) retrying...\n", err)
			time.Sleep(lp.Source.RetryDelay)
			continue
		}
		break
	}

	return lockName, Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}
