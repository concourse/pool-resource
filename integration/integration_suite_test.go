package integration_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/concourse/pool-resource/out"

	"github.com/onsi/gomega/gexec"

	"testing"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var (
	outPath    string
	inPath     string
	builtCheck string
)

var _ = BeforeSuite(func() {
	var err error

	outPath, err = gexec.Build("github.com/concourse/pool-resource/cmd/out")
	Expect(err).NotTo(HaveOccurred())

	builtCheck, err = gexec.Build("github.com/concourse/pool-resource/cmd/check")
	Expect(err).NotTo(HaveOccurred())

	pwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	inPath = filepath.Join(pwd, "../assets/in")
})

type version struct {
	Ref string `json:"ref"`
}

type metadataPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type inResponse struct {
	Version  version        `json:"version"`
	Metadata []metadataPair `json:"metadata"`
}

func runIn(inJson string, destination string, expectedExitCode int) *gexec.Session {
	inCmd := exec.Command(inPath, destination)

	stdin, err := inCmd.StdinPipe()
	Expect(err).NotTo(HaveOccurred())

	session, err := gexec.Start(inCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())

	stdin.Write([]byte(inJson))
	stdin.Close()

	Expect(err).NotTo(HaveOccurred())

	<-session.Exited
	Expect(session.ExitCode()).To(Equal(expectedExitCode))

	return session
}

func runOut(request out.OutRequest, sourceDir string) *gexec.Session {
	outCmd := exec.Command(outPath, sourceDir)

	outCmd.Env = append(
		os.Environ(),
		"BUILD_ID=1234",
		"BUILD_NAME=42",
		"BUILD_JOB_NAME=job-name",
		"BUILD_PIPELINE_NAME=pipeline-name",
	)

	stdin, err := outCmd.StdinPipe()
	Expect(err).NotTo(HaveOccurred())

	session, err := gexec.Start(outCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())

	json.NewEncoder(stdin).Encode(request)
	stdin.Close()

	return session
}

func setupGitRepo(dir string) {
	setupGitRepoWithPool(dir, "lock-pool")
}

func setupGitRepoWithPool(dir, pool string) {
	_, err := git.PlainInit(dir, false)
	Expect(err).NotTo(HaveOccurred())

	setupPool(dir, pool)
}

func setupPool(repoDir, pool string) {
	r, err := git.PlainOpen(repoDir)
	Expect(err).NotTo(HaveOccurred())

	w, err := r.Worktree()
	Expect(err).NotTo(HaveOccurred())

	err = os.Mkdir(filepath.Join(repoDir, pool), 0777)
	Expect(err).NotTo(HaveOccurred())

	directories := []string{"unclaimed", "claimed"}

	for _, directory := range directories {
		path := filepath.Join(repoDir, pool, directory)
		err = os.Mkdir(path, 0777)
		Expect(err).NotTo(HaveOccurred())

		gitKeepPath := filepath.Join(path, ".gitkeep")

		err = ioutil.WriteFile(gitKeepPath, []byte{}, 0777)
		Expect(err).NotTo(HaveOccurred())

		_, err = w.Add(filepath.Join(pool, directory, ".gitkeep"))
		Expect(err).NotTo(HaveOccurred())
	}

	_, err = w.Commit("test-git-setup", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Ginkgo Local",
			Email: "ginkgo@localhost",
			When:  time.Now(),
		},
	})
	Expect(err).NotTo(HaveOccurred())
}

func addLockCommit(repoDir string) string {
	return addLockToPool(repoDir, "lock-pool", fmt.Sprintf("%s-lock", randString()))
}

func addLockToPool(repoDir, pool, lockName string) string {
	r, err := git.PlainOpen(repoDir)
	Expect(err).NotTo(HaveOccurred())

	w, err := r.Worktree()
	Expect(err).NotTo(HaveOccurred())

	lockFileName := lockName

	lockFilePath := filepath.Join(pool, "unclaimed", lockFileName)

	err = ioutil.WriteFile(filepath.Join(repoDir, lockFilePath), []byte(`{"some":"json"}`), 0777)
	Expect(err).NotTo(HaveOccurred())

	_, err = w.Add(lockFilePath)
	Expect(err).NotTo(HaveOccurred())

	commit, err := w.Commit("another lock added", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Ginkgo Local",
			Email: "ginkgo@localhost",
			When:  time.Now(),
		},
	})
	Expect(err).NotTo(HaveOccurred())

	obj, err := r.CommitObject(commit)
	Expect(err).NotTo(HaveOccurred())

	return fmt.Sprint(obj.Hash)
}

func getVersion(gitURI string, ref string) out.Version {
	gitVersionRepo, err := ioutil.TempDir("", "git-version-repo")
	Expect(err).NotTo(HaveOccurred())

	defer os.RemoveAll(gitVersionRepo)

	gitSetup := exec.Command("git", "clone", gitURI, ".")
	gitSetup.Dir = gitVersionRepo
	err = gitSetup.Run()
	Expect(err).NotTo(HaveOccurred())

	gitVersion := exec.Command("git", "rev-parse", ref)
	gitVersion.Dir = gitVersionRepo
	sha, err := gitVersion.Output()
	Expect(err).NotTo(HaveOccurred())

	return out.Version{
		Ref: strings.TrimSpace(string(sha)),
	}
}

func randString() string {
	rand.Seed(time.Now().UnixNano())

	alpha := []rune("abcdefghijklmnopqrstuvwxyz")

	final := make([]rune, 10)

	for i := range final {
		final[i] = alpha[rand.Intn(len(alpha))]
	}

	return string(final)
}
