package out

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNoLocksAvailable = errors.New("no locks to claim")
var ErrLockConflict = errors.New("pool state out of date")
var ErrLockActive = errors.New("lock found")

var _ LockHandler = (*GitLockHandler)(nil)

type GitLockHandler struct {
	Source Source

	dir       string
	checkOnly bool
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
	_, err := os.ReadFile(filepath.Join(glh.dir, glh.Source.Pool, "unclaimed", lockName))
	if err != nil {
		return "", ErrNoLocksAvailable
	}

	output, err := glh.git("mv", filepath.Join(glh.Source.Pool, "unclaimed", lockName), filepath.Join(glh.Source.Pool, "claimed", lockName))
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	commitMessage := fmt.Sprintf("claiming: %s\n%s", lockName, glh.buildUrl())
	output, err = glh.git("commit", "-m", commitMessage)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", err
	}

	return ref, nil
}

func (glh *GitLockHandler) RemoveLock(lockName string) (string, error) {
	pool := filepath.Join(glh.dir, glh.Source.Pool)

	output, err := glh.git("rm", filepath.Join(pool, "claimed", lockName))
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	output, err = glh.git("commit", "-m", fmt.Sprintf("removing: %s\n%s", lockName, glh.buildUrl()))
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", err
	}

	return ref, nil
}

func (glh *GitLockHandler) UnclaimLock(lockName string) (string, error) {
	pool := filepath.Join(glh.dir, glh.Source.Pool)

	output, err := glh.git("mv", filepath.Join(pool, "claimed", lockName), filepath.Join(pool, "unclaimed", lockName))
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	output, err = glh.git("commit", "-m", fmt.Sprintf("unclaiming: %s\n%s", lockName, glh.buildUrl()))
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", err
	}

	return ref, nil
}

func (glh *GitLockHandler) ResetLock() error {
	output, err := glh.git("fetch", "origin", glh.Source.Branch)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return err
	}

	output, err = glh.git("reset", "--hard", "origin/"+glh.Source.Branch)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
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

	err := os.WriteFile(lockPath, contents, 0555)
	if err != nil {
		return "", err
	}

	output, err := glh.git("add", lockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	commitMessage := fmt.Sprintf("adding %s: %s\n%s", claimedness, lock, glh.buildUrl())
	output, err = glh.git("commit", lockPath, "-m", commitMessage)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", err
	}

	return string(ref), nil
}

func (glh *GitLockHandler) UpdateLock(lockName string, contents []byte) (string, error) {
	// Wait if claimed
	_, err := os.ReadFile(filepath.Join(glh.dir, glh.Source.Pool, "claimed", lockName))
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

	err = os.WriteFile(lockPath, contents, 0555)
	if err != nil {
		return "", err
	}

	output, err := glh.git("add", lockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	commitMessage := fmt.Sprintf("%s: %s\n%s", operation, lockName, glh.buildUrl())
	output, err = glh.git("commit", lockPath, "-m", commitMessage)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", err
	}

	return ref, nil
}

func (glh *GitLockHandler) CheckLock(lockName string) (string, error) {
	glh.checkOnly = true

	// Wait if claimed
	_, err := os.ReadFile(filepath.Join(glh.dir, glh.Source.Pool, "claimed", lockName))
	if err == nil {
		return "", ErrLockActive
	}

	output, err := glh.git("pull", "origin", glh.Source.Branch)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", err
	}

	return string(ref), nil
}

func (glh *GitLockHandler) CheckUnclaimedLock(lockName string) (string, error) {
	glh.checkOnly = true

	// Wait if unclaimed
	_, err := os.ReadFile(filepath.Join(glh.dir, glh.Source.Pool, "unclaimed", lockName))
	if err == nil {
		return "", ErrLockActive
	}

	output, err := glh.git("pull", "origin", glh.Source.Branch)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", err
	}

	return string(ref), nil
}

func (glh *GitLockHandler) Setup() error {
	var err error

	glh.dir, err = os.MkdirTemp("", "pool-resource")
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "clone", "--single-branch", "--branch", glh.Source.Branch, glh.Source.URI, glh.dir)
	err = cmd.Run()
	if err != nil {
		return err
	}

	_, err = glh.git("config", "user.name")
	if err != nil {
		// hardcode git user.name if not already set in git_config
		output, err := glh.git("config", "user.name", "CI Pool Resource")
		if err != nil {
			fmt.Fprintln(os.Stderr, output)
			return err
		}
	}

	_, err = glh.git("config", "user.email")
	if err != nil {
		// hardcode git user.email if not already set in git_config
		output, err := glh.git("config", "user.email", "ci-pool@localhost")
		if err != nil {
			fmt.Fprintln(os.Stderr, output)
			return err
		}
	}

	return nil
}

func (glh *GitLockHandler) GrabAvailableLock() (string, string, error) {
	var files []os.DirEntry

	allFiles, err := os.ReadDir(filepath.Join(glh.dir, glh.Source.Pool, "unclaimed"))
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

	output, err := glh.git("mv", filepath.Join(glh.Source.Pool, "unclaimed", name), filepath.Join(glh.Source.Pool, "claimed", name))
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", "", err
	}

	commitMessage := fmt.Sprintf("claiming: %s\n%s", name, glh.buildUrl())
	output, err = glh.git("commit", "-m", commitMessage)
	if err != nil {
		fmt.Fprintln(os.Stderr, output)
		return "", "", err
	}

	ref, err := glh.git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, ref)
		return "", "", err
	}

	return name, string(ref), nil
}

func (glh *GitLockHandler) BroadcastLockPool() (string, error) {
	// validate if we're doing check only
	if glh.checkOnly {
		return "", nil
	}

	contents, err := glh.git("push", "origin", "HEAD:"+glh.Source.Branch)

	// if we push and everything is up to date then someone else has made
	// a commit in the same second acquiring the same lock
	//
	// we need to stop and try again
	if strings.Contains(contents, falsePushString) {
		return contents, ErrLockConflict
	}

	if strings.Contains(contents, pushRejectedString) {
		return contents, ErrLockConflict
	}

	if strings.Contains(contents, pushRemoteRejectedString) {
		return contents, ErrLockConflict
	}

	return contents, err
}

func (glh *GitLockHandler) git(args ...string) (string, error) {
	arguments := append([]string{"-C", glh.dir}, args...)
	cmd := exec.Command("git", arguments...)
	s, err := cmd.CombinedOutput()
	return string(s), err
}

func (glh *GitLockHandler) buildUrl() string {
	buildUrl := os.Getenv("BUILD_URL")
	return fmt.Sprintf("Build URL: %s ", buildUrl)
}
