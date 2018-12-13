package out

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNoLocksAvailable = errors.New("no locks to claim")
var ErrLockConflict = errors.New("pool state out of date")

type GitLockHandler struct {
	Source Source

	dir string
}

const falsePushString = "Everything up-to-date"
const pushRejectedString = "[rejected]"
const pushRemoteRejectedString = "[remote rejected]"

func NewGitLockHandler(source Source) *GitLockHandler {
	return &GitLockHandler{
		Source: source,
	}
}

func (glh *GitLockHandler) ClaimLock(lockName string) (string, error) {
	_, err := ioutil.ReadFile(filepath.Join(glh.dir, glh.Source.Pool, "unclaimed", lockName))
	if err != nil {
		return "", ErrNoLocksAvailable
	}

	_, err = glh.git("mv", filepath.Join(glh.Source.Pool, "unclaimed", lockName), filepath.Join(glh.Source.Pool, "claimed", lockName))
	if err != nil {
		return "", err
	}

	commitMessage := fmt.Sprintf(glh.messagePrefix()+"claiming: %s", lockName)
	_, err = glh.git("commit", "-m", commitMessage)
	if err != nil {
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return string(ref), nil
}

func (glh *GitLockHandler) RemoveLock(lockName string) (string, error) {
	pool := filepath.Join(glh.dir, glh.Source.Pool)

	_, err := glh.git("rm", filepath.Join(pool, "claimed", lockName))
	if err != nil {
		return "", err
	}

	_, err = glh.git("commit", "-m", glh.messagePrefix()+fmt.Sprintf("removing: %s", lockName))
	if err != nil {
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return string(ref), nil
}

func (glh *GitLockHandler) UnclaimLock(lockName string) (string, error) {
	pool := filepath.Join(glh.dir, glh.Source.Pool)

	_, err := glh.git("mv", filepath.Join(pool, "claimed", lockName), filepath.Join(pool, "unclaimed", lockName))
	if err != nil {
		return "", err
	}

	_, err = glh.git("commit", "-m", glh.messagePrefix()+fmt.Sprintf("unclaiming: %s", lockName))
	if err != nil {
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

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

func (glh *GitLockHandler) AddLock(lock string, contents []byte, initiallyClaimed bool) (string, error) {
	var claimedness string
	if initiallyClaimed {
		claimedness = "claimed"
	} else {
		claimedness = "unclaimed"
	}

	pool := filepath.Join(glh.dir, glh.Source.Pool)
	lockPath := filepath.Join(pool, claimedness, lock)

	err := ioutil.WriteFile(lockPath, contents, 0555)
	if err != nil {
		return "", err
	}

	_, err = glh.git("add", lockPath)
	if err != nil {
		return "", err
	}

	commitMessage := glh.messagePrefix() + fmt.Sprintf("adding %s: %s", claimedness, lock)
	_, err = glh.git("commit", lockPath, "-m", commitMessage)
	if err != nil {
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return string(ref), nil
}

func (glh *GitLockHandler) UpdateLock(lockName string, contents []byte) (string, error) {
	// Wait if claimed
	_, err := ioutil.ReadFile(filepath.Join(glh.dir, glh.Source.Pool, "claimed", lockName))
	if err == nil {
		return "", ErrNoLocksAvailable
	}

	operation := "updating"

	// Remove if unclaimed
	err = os.Remove(filepath.Join(glh.dir, glh.Source.Pool, "unclaimed", lockName))
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		operation = "adding unclaimed"
	}

	// Add new lock
	lockPath := filepath.Join(glh.dir, glh.Source.Pool, "unclaimed", lockName)

	err = ioutil.WriteFile(lockPath, contents, 0555)
	if err != nil {
		return "", err
	}

	_, err = glh.git("add", lockPath)
	if err != nil {
		return "", err
	}

	commitMessage := glh.messagePrefix() + fmt.Sprintf("%s: %s", operation, lockName)
	_, err = glh.git("commit", lockPath, "-m", commitMessage)
	if err != nil {
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return string(ref), nil
}

func (glh *GitLockHandler) Setup() error {
	var err error

	glh.dir, err = ioutil.TempDir("", "pool-resource")
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "clone", "--branch", glh.Source.Branch, glh.Source.URI, glh.dir)
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

func (glh *GitLockHandler) GrabAvailableLock() (string, string, error) {
	var files []os.FileInfo

	allFiles, err := ioutil.ReadDir(filepath.Join(glh.dir, glh.Source.Pool, "unclaimed"))
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

	index := rand.Int() % len(files)
	name := filepath.Base(files[index].Name())

	_, err = glh.git("mv", filepath.Join(glh.Source.Pool, "unclaimed", name), filepath.Join(glh.Source.Pool, "claimed", name))
	if err != nil {
		return "", "", err
	}

	commitMessage := fmt.Sprintf(glh.messagePrefix()+"claiming: %s", name)
	_, err = glh.git("commit", "-m", commitMessage)
	if err != nil {
		return "", "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}

	return name, string(ref), nil
}

func (glh *GitLockHandler) BroadcastLockPool() ([]byte, error) {
	contents, err := glh.git("push", "origin", "HEAD:"+glh.Source.Branch)

	// if we push and everything is up to date then someone else has made
	// a commit in the same second acquiring the same lock
	//
	// we need to stop and try again
	if strings.Contains(string(contents), falsePushString) {
		return contents, ErrLockConflict
	}

	if strings.Contains(string(contents), pushRejectedString) {
		return contents, ErrLockConflict
	}

	if strings.Contains(string(contents), pushRemoteRejectedString) {
		return contents, ErrLockConflict
	}

	return contents, err
}

func (glh *GitLockHandler) git(args ...string) ([]byte, error) {
	arguments := append([]string{"-C", glh.dir}, args...)
	cmd := exec.Command("git", arguments...)
	return cmd.CombinedOutput()
}

func (glh *GitLockHandler) messagePrefix() string {
	buildID := os.Getenv("BUILD_ID")
	buildName := os.Getenv("BUILD_NAME")
	jobName := os.Getenv("BUILD_JOB_NAME")
	pipelineName := os.Getenv("BUILD_PIPELINE_NAME")
	teamName := os.Getenv("BUILD_TEAM_NAME")

	if buildName != "" && jobName != "" && pipelineName != "" && teamName != "" {
		return fmt.Sprintf("%s/%s/%s build %s ", teamName, pipelineName, jobName, buildName)
	} else if buildID != "" {
		return fmt.Sprintf("one-off build %s ", buildID)
	}

	return ""
}
