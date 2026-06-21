package main

import (
	"os"

	"nopsai/pkg/buildinfo"
	"nopsai/services/dispatcher/internal/app"
)

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-dispatcher")
		return
	}
	app.Run()
}
