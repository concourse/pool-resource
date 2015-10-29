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
	AddLock(lock string, contents []byte) (version string, err error)
	RemoveLock(lock string) (version string, err error)

	Setup() error
	BroadcastLockPool() ([]byte, error)
	ResetLock() error
}

func (lp *LockPool) AcquireLock() (string, Version, error) {
	fmt.Fprintf(lp.Output, "acquiring lock on: %s\n", lp.Source.Pool)
	grabAvailableLockAction := &grabAvailableLockAction{
		output:      lp.Output,
		lockHandler: lp.LockHandler,
		pool:        lp.Source.Pool,
	}

	err := lp.performRobustAction(grabAvailableLockAction)
	if err != nil {
		return "", Version{}, err
	}

	return grabAvailableLockAction.lock, Version{
		Ref: strings.TrimSpace(grabAvailableLockAction.ref),
	}, nil
}

func (lp *LockPool) ReleaseLock(inDir string) (string, Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", Version{}, err
	}
	lockName := strings.TrimSpace(string(nameFileContents))

	fmt.Fprintf(lp.Output, "releasing lock: %s on pool: %s\n", lockName, lp.Source.Pool)

	unclaimLockAction := &unclaimLockAction{
		output:      lp.Output,
		lockHandler: lp.LockHandler,
		lockName:    lockName,
	}

	err = lp.performRobustAction(unclaimLockAction)
	if err != nil {
		return "", Version{}, err
	}

	return lockName, Version{
		Ref: strings.TrimSpace(unclaimLockAction.ref),
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

	addLockAction := &addLockAction{
		output:       lp.Output,
		lockHandler:  lp.LockHandler,
		lockName:     lockName,
		lockContents: lockContents,
	}

	err = lp.performRobustAction(addLockAction)
	if err != nil {
		return "", Version{}, err
	}

	return lockName, Version{
		Ref: strings.TrimSpace(addLockAction.ref),
	}, nil
}

func (lp *LockPool) RemoveLock(inDir string) (string, Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", Version{}, err
	}

	lockName := strings.TrimSpace(string(nameFileContents))

	fmt.Fprintf(lp.Output, "removing lock: %s on pool: %s\n", lockName, lp.Source.Pool)

	removeLockAction := &removeLockAction{
		output:      lp.Output,
		lockHandler: lp.LockHandler,
		lockName:    lockName,
	}

	err = lp.performRobustAction(removeLockAction)
	if err != nil {
		return "", Version{}, err
	}

	return lockName, Version{
		Ref: strings.TrimSpace(removeLockAction.ref),
	}, nil
}

func (lp *LockPool) performRobustAction(action lockPoolAction) error {
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

		retry, err := action.Run()

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
