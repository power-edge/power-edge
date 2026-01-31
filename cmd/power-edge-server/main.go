package main

import (
	"fmt"
	"os"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	fmt.Fprintf(os.Stderr, "power-edge-server %s (commit: %s, built: %s)\n", Version, GitCommit, BuildTime)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Central controller not yet implemented.")
	fmt.Fprintln(os.Stderr, "Coming soon: Fleet management, config distribution, centralized monitoring")
	os.Exit(0)
}
