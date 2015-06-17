package main

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/concourse/pool-resource/out"
)

func main() {

	rand.Seed(time.Now().UnixNano())

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

	if request.Source.RetryDelay == 0 {
		request.Source.RetryDelay = 10 * time.Second
	}

	lockPool := out.NewLockPool(request.Source, os.Stderr)

	var (
		lock    string
		version out.Version
	)

	if request.Params.Acquire {
		lock, version, err = lockPool.AcquireLock()
		if err != nil {
			fatal("acquiring lock", err)
		}
	}

	if request.Params.Release != "" {
		poolName := filepath.Join(sourceDir, request.Params.Release)
		lock, version, err = lockPool.ReleaseLock(poolName)
		if err != nil {
			fatal("releasing lock", err)
		}
	}

	if request.Params.Add != "" {
		lockPath := filepath.Join(sourceDir, request.Params.Add)
		lock, version, err = lockPool.AddLock(lockPath)
		if err != nil {
			fatal("adding lock", err)
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
		errorMessages = append(errorMessages, "invalid payload (missing uri)")
	}

	if request.Source.Pool == "" {
		errorMessages = append(errorMessages, "invalid payload (missing pool)")
	}

	if request.Source.Branch == "" {
		errorMessages = append(errorMessages, "invalid payload (missing branch)")
	}

	if request.Params.Acquire == false && request.Params.Release == "" && request.Params.Add == "" {
		errorMessages = append(errorMessages, "invalid payload (missing acquire, release, or add)")
	}

	if len(errorMessages) > 0 {
		for _, errorMessage := range errorMessages {
			println(errorMessage)
		}
		os.Exit(1)
	}
}
