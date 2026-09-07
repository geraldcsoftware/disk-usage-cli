package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"dusk": func() { os.Exit(realMain()) },
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "script"),
		Setup: func(env *testscript.Env) error {
			home := filepath.Join(env.WorkDir, "home")
			for _, d := range []string{"config", "state", filepath.Join("home", ".m2", "repository")} {
				if err := os.MkdirAll(filepath.Join(env.WorkDir, d), 0o755); err != nil {
					return err
				}
			}
			env.Setenv("HOME", home)
			env.Setenv("XDG_CONFIG_HOME", filepath.Join(env.WorkDir, "config"))
			env.Setenv("XDG_STATE_HOME", filepath.Join(env.WorkDir, "state"))
			env.Setenv("NO_COLOR", "1")
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			// exit <code> <command> [args...] runs the command and asserts its
			// exit code exactly, which the built in exec cannot do.
			"exit": func(ts *testscript.TestScript, neg bool, args []string) {
				if len(args) < 2 {
					ts.Fatalf("usage: exit <code> <command> [args...]")
				}
				want, err := strconv.Atoi(args[0])
				if err != nil {
					ts.Fatalf("exit: bad code %q", args[0])
				}
				err = ts.Exec(args[1], args[2:]...)
				got := 0
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					got = ee.ExitCode()
				} else if err != nil {
					ts.Fatalf("exit: %v", err)
				}
				if (got == want) == neg {
					ts.Fatalf("exit code %d, want %d", got, want)
				}
			},
		},
	})
}
