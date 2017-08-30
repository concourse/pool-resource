package main

import (
	"fmt"
	"time"
)

type inRequest struct {
	Source  Source  `json:"source"`
	Version Version `json:"version"`
}

type inResponse struct {
	Version  Version    `json:"version"`
	Metadata []Metadata `json:"metadata"`
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

type Metadata struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func main() {
	fmt.Println("vim-go")
}
