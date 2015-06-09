package integration_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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
	inPath = filepath.Join(pwd, "../in/in")
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

func runIn(inJson string, destination string) inResponse {
	var response inResponse

	inCmd := exec.Command(inPath, destination)
	stdin, err := inCmd.StdinPipe()
	Ω(err).ShouldNot(HaveOccurred())

	session, err := gexec.Start(inCmd, GinkgoWriter, GinkgoWriter)
	Ω(err).ShouldNot(HaveOccurred())

	stdin.Write([]byte(inJson))
	stdin.Close()

	Ω(err).ShouldNot(HaveOccurred())

	Eventually(session).Should(gexec.Exit(0))

	err = json.Unmarshal(session.Out.Contents(), &response)
	Ω(err).ShouldNot(HaveOccurred())
	return response
}

func runOut(request out.OutRequest, sourceDir string) out.OutResponse {
	var response out.OutResponse

	outCmd := exec.Command(outPath, sourceDir)
	stdin, err := outCmd.StdinPipe()
	Ω(err).ShouldNot(HaveOccurred())

	session, err := gexec.Start(outCmd, GinkgoWriter, GinkgoWriter)
	Ω(err).ShouldNot(HaveOccurred())

	json.NewEncoder(stdin).Encode(request)
	stdin.Close()

	Eventually(session, 5*time.Second).Should(gexec.Exit(0))

	err = json.Unmarshal(session.Out.Contents(), &response)
	Ω(err).ShouldNot(HaveOccurred())

	return response
}

func setupGitRepo(dir string) {
	gitSetup := exec.Command("bash", "-e", "-c", `
		git init

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

	err := gitSetup.Run()
	Ω(err).ShouldNot(HaveOccurred())
}
