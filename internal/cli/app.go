//go:build darwin

// Package cli defines the dusk commands, their flags and exit codes.
package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
	"github.com/geraldcsoftware/disk-usage-cli/internal/report"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

// Exit codes. check uses the monitoring convention (0 ok, 1 warn, 2 critical,
// 3 unknown); every other command uses the sysexits style codes.
const (
	ExitOK           = 0
	ExitUnknown      = 3
	ExitUsage        = 64
	ExitDeleteFailed = 73
	ExitLocked       = 75
	ExitConfig       = 78
)

// BuildInfo is stamped by the linker at release time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// App carries everything the commands take from the process, so tests can
// substitute all of it.
type App struct {
	Info             BuildInfo
	Stdout           io.Writer
	Stderr           io.Writer
	Getenv           func(string) string
	Now              func() time.Time
	StdoutIsTerminal bool
}

// exitError carries an exit code out of a command. A nil err means the code
// is the whole message, as with check's state codes.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit %d", e.code)
	}
	return e.err.Error()
}

// Unwrap lets callers inspect the wrapped cause with errors.As, for example
// to print each line of a config.ValidationError.
func (e *exitError) Unwrap() error { return e.err }

func exitWith(code int, err error) error { return &exitError{code: code, err: err} }

// session is the per invocation state built from persistent flags.
type session struct {
	app        *App
	configPath string
	json       bool
	iec        bool
	color      string
	home       string
	format     report.Format
	palette    report.Palette
}

// Main runs the command line and returns the process exit code.
func Main(args []string, app App) int {
	root := newRoot(&app)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.err != nil {
			fmt.Fprintln(app.Stderr, "dusk:", ee.err)
		}
		return ee.code
	}
	fmt.Fprintln(app.Stderr, "dusk:", err)
	return ExitUsage
}

func newRoot(app *App) *cobra.Command {
	s := &session{app: app}
	root := &cobra.Command{
		Use:           "dusk",
		Short:         "Disk usage keeper for macOS",
		Long:          "dusk measures free space, tracks warn and critical states with hysteresis, and reports directories that grow past a configured maximum.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return s.prepare()
		},
	}
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return exitWith(ExitUsage, err)
	})
	pf := root.PersistentFlags()
	pf.StringVar(&s.configPath, "config", "", "config file (default $XDG_CONFIG_HOME/dusk/config.toml or ~/.config/dusk/config.toml)")
	pf.BoolVar(&s.json, "json", false, "print JSON instead of a table")
	pf.BoolVar(&s.iec, "iec", false, "print sizes in GiB and MiB instead of GB and MB")
	pf.StringVar(&s.color, "color", "auto", "colour output: always, never or auto")

	root.AddCommand(newVersionCmd(s), newConfigCmd(s), newRulesCmd(s))
	return root
}

// prepare resolves the colour mode for every command, and the home directory
// and config path for those that have one available. It never fails on a
// missing home directory, since cobra's own help and completion commands need
// neither; commands that do need one call requireHome first.
func (s *session) prepare() error {
	s.home = s.app.Getenv("HOME")
	enabled, err := report.ColorEnabled(s.color, s.app.Getenv, s.app.StdoutIsTerminal)
	if err != nil {
		return exitWith(ExitUsage, err)
	}
	s.palette = report.Palette{Enabled: enabled}
	s.format = report.Format{IEC: s.iec}
	if s.configPath == "" && s.home != "" {
		s.configPath = config.DefaultPath(s.app.Getenv, s.home)
	}
	return nil
}

// requireHome fails with ExitConfig when no home directory is available. Call
// it first in any command that needs the home directory or resolved config
// path, since prepare leaves both empty rather than failing outright.
func (s *session) requireHome() error {
	if s.home == "" {
		return exitWith(ExitConfig, errors.New("HOME is not set"))
	}
	return nil
}

// loadConfig parses and validates the config. Warnings are returned for the
// caller to print; errors carry ExitConfig.
func (s *session) loadConfig() (*config.Config, []string, error) {
	if err := s.requireHome(); err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load(s.configPath, s.home)
	if err != nil {
		return nil, nil, exitWith(ExitConfig, fmt.Errorf("%s: %w", s.configPath, err))
	}
	warnings, err := cfg.Validate(config.DefaultValidator(s.home))
	if err != nil {
		return nil, warnings, exitWith(ExitConfig, fmt.Errorf("%s: %w", s.configPath, err))
	}
	return cfg, warnings, nil
}

func (s *session) stateDir() (state.Dir, error) {
	if err := s.requireHome(); err != nil {
		return "", err
	}
	d, err := state.DefaultDir(s.app.Getenv, s.home)
	if err != nil {
		return "", exitWith(ExitUnknown, fmt.Errorf("state directory: %w", err))
	}
	return d, nil
}
