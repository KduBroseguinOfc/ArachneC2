package main

import (
	"fmt"
	"os"

	"github.com/anomalyco/arachne-c2/server/core"
)

func main() {
	if err := core.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
