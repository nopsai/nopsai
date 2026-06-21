package main

import (
	"fmt"
	"os"

	"nopsai/internal/cli/command"
	"nopsai/pkg/buildinfo"
)

func main() {
	if err := command.Execute(buildinfo.Current().Version); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
