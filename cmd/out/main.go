package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/concourse/pool-resource/out"
)

func main() {
	if len(os.Args) < 2 {
		println("usage: " + os.Args[0] + " <source>")
		os.Exit(1)
	}

	sourceDir := os.Args[1]

	var request out.OutRequest
	err := json.NewDecoder(os.Stdin).Decode(&request)
	if err != nil {
		fatal("reading request", err)
	}

	validateRequest(request)

	pools := Pools{
		Source: request.Source,
	}

	var (
		lock    string
		version out.Version
	)

	if request.Params.Acquire {
		lock, version, err = pools.AcquireLock(request.Source.Pool)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if request.Params.Release != "" {
		lockPool := filepath.Join(sourceDir, request.Params.Release)
		lock, version, err = pools.ReleaseLock(lockPool)
		if err != nil {
			log.Fatalln(err)
		}
	}

	err = json.NewEncoder(os.Stdout).Encode(out.OutResponse{
		Version: version,
		Metadata: []out.MetadataPair{
			{Name: "lock_name", Value: lock},
			{Name: "pool_name", Value: request.Source.Pool},
		},
	})
	if err != nil {
		fatal("encoding output", err)
	}
}

func fatal(doing string, err error) {
	println("error " + doing + ": " + err.Error())
	os.Exit(1)
}

func validateRequest(request out.OutRequest) {
	var errorMessages []string

	if request.Source.URI == "" {
		errorMessages = append(errorMessages, "uri is required in the resource source config")
	}

	if request.Source.Pool == "" {
		errorMessages = append(errorMessages, "pool is required in the resource source config")
	}

	if request.Source.Branch == "" {
		errorMessages = append(errorMessages, "branch is required in the resource source config")
	}

	if request.Params.Acquire == false && request.Params.Release == "" {
		errorMessages = append(errorMessages, "acquire or release is required in the put params")
	}

	if len(errorMessages) > 0 {
		for _, errorMessage := range errorMessages {
			println(errorMessage)
		}
		os.Exit(1)
	}
}
