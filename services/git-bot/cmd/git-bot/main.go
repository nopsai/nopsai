package main

import (
	"os"

	"nopsai/pkg/buildinfo"
	"nopsai/services/git-bot/internal/app"
)

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-git-bot")
		return
	}
	app.Run()
}
