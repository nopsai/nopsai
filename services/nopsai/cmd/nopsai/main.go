package main

import (
	"os"

	"nopsai/pkg/buildinfo"
	"nopsai/services/nopsai/internal/app"
)

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-api")
		return
	}
	app.Run()
}
