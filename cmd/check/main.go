package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/src-d/go-git.v4"
	"io"
)

type checkRequest struct {
	Source  Source  `json:"source"`
	Version Version `json:"version"`
}

type Source struct {
	URI        string        `json:"uri"`
	Branch     string        `json:"branch"`
	PrivateKey string        `json:"private_key" mapstructure:"private_key"`
	Pool       string        `json:"pool"`
	RetryDelay time.Duration `json:"retry_delay" mapstructure:"retry_delay"`
}

type Version struct {
	Ref string `json:"ref"`
}

func main() {
	var req checkRequest
	err := json.NewDecoder(os.Stdin).Decode(&req)
	if err != nil {
		panic(err)
	}

	defer os.Stdin.Close()

	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		panic(err)
	}

	repo, err := git.PlainClone(tmpDir, false, &git.CloneOptions{
		URL:      req.Source.URI,
		Progress: os.Stderr,
	})
	if err != nil {
		panic(err)
	}

	var versions []Version

	foundReference := false

	if req.Version.Ref == "" {
		ref, err := repo.Head()
		if err != nil {
			panic(err)
		}

		versions = append(versions, Version{Ref: ref.Hash().String()})

	} else {
		commits, err := repo.Log(&git.LogOptions{})
		if err != nil {
			panic(err)
		}


		for {
			commit, err := commits.Next()
			if err != nil {
				if err == io.EOF {
					break
				}

				panic(err)
			}

			versions = append([]Version{{Ref: commit.Hash.String()}}, versions...)

			if commit.Hash.String() == req.Version.Ref {
				foundReference = true
				break
			}
		}

		if !foundReference {
			versions = []Version{versions[len(versions) - 1]}
		}
	}

	err = json.NewEncoder(os.Stdout).Encode(versions)
	if err != nil {
		panic(err)
	}
}
