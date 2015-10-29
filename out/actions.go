package out

import (
	"fmt"
	"io"
)

type lockPoolAction interface {
	Run() (bool, error)
}

type grabAvailableLockAction struct {
	output      io.Writer
	lockHandler LockHandler
	pool        string
	lock        string
	ref         string
}

func (a *grabAvailableLockAction) Run() (bool, error) {
	var err error
	a.lock, a.ref, err = a.lockHandler.GrabAvailableLock()

	if err == ErrNoLocksAvailable {
		fmt.Fprint(a.output, ".")
		return true, nil
	}

	if err != nil {
		fmt.Fprintf(a.output, "\nfailed to acquire lock on pool: %s! (err: %s) retrying...\n", a.pool, err)
		return true, nil
	}

	return false, nil
}

type unclaimLockAction struct {
	output      io.Writer
	lockHandler LockHandler
	lockName    string
	ref         string
}

func (a *unclaimLockAction) Run() (bool, error) {
	var err error
	a.ref, err = a.lockHandler.UnclaimLock(a.lockName)

	if err != nil {
		fmt.Fprintf(a.output, "\nfailed to unclaim the lock: %s! (err: %s)\n", a.lockName, err)
		return false, err
	}

	return false, nil
}

type addLockAction struct {
	output       io.Writer
	lockHandler  LockHandler
	lockName     string
	lockContents []byte
	ref          string
}

func (a *addLockAction) Run() (bool, error) {
	var err error
	a.ref, err = a.lockHandler.AddLock(a.lockName, a.lockContents)

	if err != nil {
		fmt.Fprintf(a.output, "failed to add the lock: %s! (err: %s) retrying...\n", a.lockName, err)
		return true, nil
	}

	return false, nil
}

type removeLockAction struct {
	output      io.Writer
	lockHandler LockHandler
	lockName    string
	ref         string
}

func (a *removeLockAction) Run() (bool, error) {
	var err error
	a.ref, err = a.lockHandler.RemoveLock(a.lockName)

	if err != nil {
		fmt.Fprintf(a.output, "\nfailed to remove the lock: %s! (err: %s)\n", a.lockName, err)
		return false, err
	}

	return false, nil
}
