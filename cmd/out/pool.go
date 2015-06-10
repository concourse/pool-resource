package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/concourse/pool-resource/out"
)

var ErrNoLocksAvailable = errors.New("No locks to claim")

type Pools struct {
	Source out.Source

	dir string
}

func (p *Pools) AcquireLock(pool string) (string, out.Version, error) {
	err := p.setup()
	if err != nil {
		return "", out.Version{}, err
	}

	var (
		lock                string
		ref                 string
		broadcastSuccessful bool
	)

	broadcastSuccessful = false

	for broadcastSuccessful == false {
		// output something so that we know it's still churning

		lock, ref, err = p.grabAvailableLock(pool)

		if err == nil {
			err = p.broadcastLockPool()
		}

		if err != nil {
			err = p.resetLock()

			if err != nil {

				fatal("resetting lock", err)
			}

			time.Sleep(10 * time.Second)
		} else {
			broadcastSuccessful = true
		}
	}

	return lock, out.Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (p *Pools) ReleaseLock(inDir string) (string, out.Version, error) {
	nameFileContents, err := ioutil.ReadFile(filepath.Join(inDir, "name"))
	if err != nil {
		return "", out.Version{}, err
	}

	lockName := strings.TrimSpace(string(nameFileContents))
	err = p.setup()
	if err != nil {
		return "", out.Version{}, err
	}

	ref, err := p.unclaimLock(lockName)

	if err != nil {
		return "", out.Version{}, err
	}

	err = p.broadcastLockPool()
	if err != nil {
		return "", out.Version{}, err
	}

	return lockName, out.Version{
		Ref: strings.TrimSpace(ref),
	}, nil
}

func (p *Pools) unclaimLock(lockName string) (string, error) {
	pool := filepath.Join(p.dir, p.Source.Pool)

	_, err := p.git("mv", filepath.Join(pool, "claimed", lockName), filepath.Join(pool, "unclaimed", lockName))
	if err != nil {
		return "", err
	}

	_, err = p.git("commit", "-am", fmt.Sprintf("unclaiming: %s", lockName))
	if err != nil {
		return "", err
	}

	ref, err := p.git("rev-parse", "HEAD")

	return string(ref), nil
}

func (p *Pools) resetLock() error {
	_, err := p.git("fetch", "origin", p.Source.Branch)
	if err != nil {
		return err
	}

	_, err = p.git("reset", "--hard", "origin/"+p.Source.Branch)
	if err != nil {
		return err
	}
	return nil
}

func (p *Pools) setup() error {
	var err error

	p.dir, err = ioutil.TempDir("", "pool-resource")
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "clone", p.Source.URI, p.dir)
	err = cmd.Run()
	if err != nil {
		return err
	}

	_, err = p.git("config", "user.name", "CI Pool Resource")
	if err != nil {
		return err
	}

	_, err = p.git("config", "user.email", "ci-pool@localhost")
	if err != nil {
		return err
	}

	return nil
}

func (p *Pools) grabAvailableLock(pool string) (string, string, error) {
	var files []os.FileInfo

	allFiles, err := ioutil.ReadDir(filepath.Join(p.dir, pool, "unclaimed"))
	if err != nil {
		return "", "", err
	}

	for _, file := range allFiles {
		fileName := filepath.Base(file.Name())
		if !strings.HasPrefix(fileName, ".") {
			files = append(files, file)
		}
	}

	if len(files) == 0 {
		return "", "", ErrNoLocksAvailable
	}

	rand.Seed(time.Now().Unix())
	index := rand.Int() % len(files)
	name := filepath.Base(files[index].Name())

	_, err = p.git("mv", filepath.Join(pool, "unclaimed", name), filepath.Join(pool, "claimed", name))
	if err != nil {
		return "", "", err
	}

	_, err = p.git("commit", "-am", fmt.Sprintf("claiming: %s", name))
	if err != nil {
		return "", "", err
	}

	ref, err := p.git("rev-parse", "HEAD")

	return name, string(ref), nil
}

func (p *Pools) broadcastLockPool() error {
	_, err := p.git("push", "origin", p.Source.Branch)
	return err
}

func (p *Pools) git(args ...string) ([]byte, error) {
	arguments := append([]string{"-C", p.dir}, args...)
	cmd := exec.Command("git", arguments...)
	cmd.Stderr = os.Stderr

	return cmd.Output()
}
