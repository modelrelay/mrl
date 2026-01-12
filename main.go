package main

import (
	"fmt"
	"os"
)

// version is set by -ldflags at build time
var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
