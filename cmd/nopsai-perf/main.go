package main

import (
	"fmt"
	"os"

	"nopsai/internal/perf"
	"nopsai/pkg/buildinfo"
)

func main() {
	if err := perf.Execute(buildinfo.Current().Version); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
