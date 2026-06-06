package main

import (
	"os"

	"nopsai/services/agent"
)

func main() {
	os.Exit(agent.Run())
}
