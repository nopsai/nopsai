package main

import (
	"os"

	"nopsai/pkg/buildinfo"
	"nopsai/services/k8s-runner/internal/app"
)

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-k8s-runner")
		return
	}
	app.Run()
}
