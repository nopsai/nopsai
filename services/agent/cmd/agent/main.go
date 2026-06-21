package main

import (
	"os"

	"nopsai/pkg/buildinfo"
	"nopsai/services/agent"
)

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-agent")
		return
	}
	os.Exit(agent.Run())
}
