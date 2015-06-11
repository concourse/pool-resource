package out

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrNoLocksAvailable = errors.New("No locks to claim")

type GitLockHandler struct {
	Source Source

	dir string
}

func NewGitLockHandler(source Source) *GitLockHandler {
	return &GitLockHandler{
		Source: source,
	}
}

func (glh *GitLockHandler) UnclaimLock(lockName string) (string, error) {
	pool := filepath.Join(glh.dir, glh.Source.Pool)

	_, err := glh.git("mv", filepath.Join(pool, "claimed", lockName), filepath.Join(pool, "unclaimed", lockName))
	if err != nil {
		return "", err
	}

	_, err = glh.git("commit", "-am", fmt.Sprintf("unclaiming: %s", lockName))
	if err != nil {
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")

	return string(ref), nil
}

func (glh *GitLockHandler) ResetLock() error {
	_, err := glh.git("fetch", "origin", glh.Source.Branch)
	if err != nil {
		return err
	}

	_, err = glh.git("reset", "--hard", "origin/"+glh.Source.Branch)
	if err != nil {
		return err
	}
	return nil
}

func (glh *GitLockHandler) Setup() error {
	var err error

	glh.dir, err = ioutil.TempDir("", "pool-resource")
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "clone", glh.Source.URI, glh.dir)
	err = cmd.Run()
	if err != nil {
		return err
	}

	_, err = glh.git("config", "user.name", "CI Pool Resource")
	if err != nil {
		return err
	}

	_, err = glh.git("config", "user.email", "ci-pool@localhost")
	if err != nil {
		return err
	}

	return nil
}

func (glh *GitLockHandler) GrabAvailableLock(pool string) (string, string, error) {
	var files []os.FileInfo

	allFiles, err := ioutil.ReadDir(filepath.Join(glh.dir, pool, "unclaimed"))
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

	_, err = glh.git("mv", filepath.Join(pool, "unclaimed", name), filepath.Join(pool, "claimed", name))
	if err != nil {
		return "", "", err
	}

	_, err = glh.git("commit", "-am", fmt.Sprintf("claiming: %s", name))
	if err != nil {
		return "", "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")

	return name, string(ref), nil
}

func (glh *GitLockHandler) BroadcastLockPool() error {
	_, err := glh.git("push", "origin", glh.Source.Branch)
	return err
}

func (glh *GitLockHandler) git(args ...string) ([]byte, error) {
	arguments := append([]string{"-C", glh.dir}, args...)
	cmd := exec.Command("git", arguments...)

	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	defer func() {
		if !cmd.ProcessState.Success() {
			fmt.Fprintln(os.Stderr, stderr.String())
		}
	}()

	return cmd.Output()
}
