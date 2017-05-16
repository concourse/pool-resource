package integration_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/concourse/pool-resource/out"

	"github.com/onsi/gomega/gexec"

	"testing"
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
	gitSetup := exec.Command("bash", "-e", "-c", `
	  git init

		git config user.email "ginkgo@localhost"
		git config user.name "Ginkgo Local"


		mkdir -p lock-pool/unclaimed
		mkdir -p lock-pool/claimed

		touch lock-pool/unclaimed/.gitkeep
		touch lock-pool/claimed/.gitkeep

		touch lock-pool/unclaimed/some-lock
		touch lock-pool/unclaimed/some-other-lock

		echo '{"some":"json"}' > lock-pool/unclaimed/some-lock
		echo '{"some":"wrong-json"}' > lock-pool/unclaimed/some-other-lock

		git add .
		git commit -m 'test-git-setup'
		git checkout -b another-branch
		git checkout master
	`)
	gitSetup.Dir = dir

	gitSetup.Stderr = GinkgoWriter
	gitSetup.Stdout = GinkgoWriter

	err := gitSetup.Run()
	Expect(err).NotTo(HaveOccurred())
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
