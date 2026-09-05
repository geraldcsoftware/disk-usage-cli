// Command dusk is a disk usage keeper for macOS.
//
// This entry point currently exposes only the version subcommand so that the
// continuous integration and release pipelines have a binary to build and the
// Homebrew formula test has something to run. The full command set is added
// milestone by milestone.
package main

import (
	"fmt"
	"os"
)

// Set at build time by GoReleaser through -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = "usage: dusk version\n"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "dusk %s (commit %s, built %s)\n", version, commit, date)
		return 0
	}
	fmt.Fprint(stderr, usage)
	return 64
}
