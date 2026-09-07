// Command dusk is a disk usage keeper for macOS.
package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/geraldcsoftware/disk-usage-cli/internal/cli"
	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

// Set at build time by GoReleaser through -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(realMain())
}

// realMain disables dataless materialisation before any subcommand runs, so
// no code path can download a cloud placeholder, then dispatches to the
// command line.
func realMain() int {
	if err := sys.DisableDatalessMaterialisation(); err != nil {
		fmt.Fprintln(os.Stderr, "dusk: cannot disable dataless materialisation:", err)
		return cli.ExitUnknown
	}
	return cli.Main(os.Args[1:], cli.App{
		Info:             cli.BuildInfo{Version: version, Commit: commit, Date: date},
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		Getenv:           os.Getenv,
		Now:              time.Now,
		StdoutIsTerminal: term.IsTerminal(int(os.Stdout.Fd())),
	})
}
