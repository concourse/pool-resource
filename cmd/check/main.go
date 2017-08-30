package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
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

type memo map[plumbing.Hash]plumbing.Hash

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
		Depth:    100,
	})
	if err != nil {
		panic(err)
	}

	var versions []Version

	if req.Version.Ref != "" {
		_, err = repo.CommitObject(plumbing.NewHash(req.Version.Ref))
		if err == plumbing.ErrObjectNotFound {
			head, err := repo.Head()
			if err != nil {
				panic(err)
			}

			versions = []Version{{Ref: head.Hash().String()}}
		}
	}

	if req.Version.Ref == "" && req.Source.Pool == "" {
		head, err := repo.Head()
		if err != nil {
			panic(err)
		}

		versions = []Version{{Ref: head.Hash().String()}}
	}

	if len(versions) == 0 {
		cIter, err := repo.Log(&git.LogOptions{})
		if err != nil {
			panic(err)
		}

		m := make(memo)

		err = cIter.ForEach(func(c *object.Commit) error {
			if err := ensure(m, c, req.Source.Pool); err != nil {
				return err
			}

			if c.NumParents() == 0 && !m[c.Hash].IsZero() {
				versions = append(versions, Version{Ref: c.Hash.String()})
				return nil
			}

			for _, p := range c.ParentHashes {
				if _, ok := m[p]; !ok {
					pc, err := repo.CommitObject(p)
					if err != nil {
						return err
					}
					if err := ensure(m, pc, req.Source.Pool); err != nil {
						return err
					}
				}
				if m[p] != m[c.Hash] {
					versions = append(versions, Version{Ref: c.Hash.String()})
					return nil
				}
			}

			return nil
		})

		for i, version := range versions {
			if version.Ref == req.Version.Ref {
				versions = []Version{versions[i], versions[i-1]}
			}
		}

		if req.Version.Ref == "" {
			versions = []Version{versions[0]}
		}
	}

	err = json.NewEncoder(os.Stdout).Encode(versions)
	if err != nil {
		panic(err)
	}
}

func ensure(m memo, c *object.Commit, path string) error {
	if _, ok := m[c.Hash]; !ok {
		t, err := c.Tree()
		if err != nil {
			return err
		}
		te, err := t.FindEntry(path)
		if err == object.ErrDirectoryNotFound {
			m[c.Hash] = plumbing.ZeroHash
			return nil
		} else if err != nil {
			if !strings.ContainsRune(path, '/') {
				m[c.Hash] = plumbing.ZeroHash
				return nil
			}
			return err
		}
		m[c.Hash] = te.Hash
	}

	return nil
}
