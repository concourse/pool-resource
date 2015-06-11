package integration_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/concourse/pool-resource/out"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var outPath string
var inPath string

var _ = BeforeSuite(func() {
	var err error

	outPath, err = gexec.Build("github.com/concourse/pool-resource/cmd/out")
	Ω(err).ShouldNot(HaveOccurred())

	pwd, err := os.Getwd()
	Ω(err).ShouldNot(HaveOccurred())
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
	Ω(err).ShouldNot(HaveOccurred())

	session, err := gexec.Start(inCmd, GinkgoWriter, GinkgoWriter)
	Ω(err).ShouldNot(HaveOccurred())

	stdin.Write([]byte(inJson))
	stdin.Close()

	Ω(err).ShouldNot(HaveOccurred())

	Eventually(session).Should(gexec.Exit(expectedExitCode))

	return session
}

func runOut(request out.OutRequest, sourceDir string) *gexec.Session {
	outCmd := exec.Command(outPath, sourceDir)

	stdin, err := outCmd.StdinPipe()
	Ω(err).ShouldNot(HaveOccurred())

	session, err := gexec.Start(outCmd, GinkgoWriter, GinkgoWriter)
	Ω(err).ShouldNot(HaveOccurred())

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
	`)
	gitSetup.Dir = dir

	gitSetup.Stderr = GinkgoWriter
	gitSetup.Stdout = GinkgoWriter

	err := gitSetup.Run()
	Ω(err).ShouldNot(HaveOccurred())
}

func getVersion(gitURI string) out.Version {
	gitVersionRepo, err := ioutil.TempDir("", "git-version-repo")
	Ω(err).ShouldNot(HaveOccurred())

	defer os.RemoveAll(gitVersionRepo)

	gitSetup := exec.Command("git", "clone", gitURI, ".")
	gitSetup.Dir = gitVersionRepo
	err = gitSetup.Run()
	Ω(err).ShouldNot(HaveOccurred())

	gitVersion := exec.Command("git", "rev-parse", "HEAD")
	gitVersion.Dir = gitVersionRepo
	sha, err := gitVersion.Output()
	Ω(err).ShouldNot(HaveOccurred())

	return out.Version{
		Ref: strings.TrimSpace(string(sha)),
	}
}
