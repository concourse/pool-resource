package main

import (
	"encoding/json"
	"os"
	"time"
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

	var versions []Version
	err = json.NewEncoder(os.Stdout).Encode(versions)
	if err != nil {
		panic(err)
	}
}
