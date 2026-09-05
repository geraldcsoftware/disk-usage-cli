package main

import (
	"os"
	"strings"
	"testing"
)

func capture(t *testing.T, args []string) (code int, stdout, stderr string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	errf, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	code = run(args, out, errf)
	o, _ := os.ReadFile(out.Name())
	e, _ := os.ReadFile(errf.Name())
	return code, string(o), string(e)
}

func TestVersionPrintsBuildInfo(t *testing.T) {
	code, out, errOut := capture(t, []string{"version"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(out, "dusk dev (commit none, built unknown)") {
		t.Fatalf("stdout = %q", out)
	}
	if errOut != "" {
		t.Fatalf("stderr = %q, want empty", errOut)
	}
}

func TestUnknownArgumentsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"version", "extra"}} {
		code, out, errOut := capture(t, args)
		if code != 64 {
			t.Errorf("args %v: exit code = %d, want 64", args, code)
		}
		if out != "" {
			t.Errorf("args %v: stdout = %q, want empty", args, out)
		}
		if errOut != usage {
			t.Errorf("args %v: stderr = %q, want usage", args, errOut)
		}
	}
}
