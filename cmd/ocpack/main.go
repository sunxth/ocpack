package main

import (
	"fmt"
	"os"

	"ocpack/cmd/ocpack/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
} 