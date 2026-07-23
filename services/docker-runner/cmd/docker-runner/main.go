package main

import (
	"os"

	"nopsai/pkg/buildinfo"
	"nopsai/services/docker-runner/internal/app"
)

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-docker-runner")
		return
	}
	app.Run()
}
