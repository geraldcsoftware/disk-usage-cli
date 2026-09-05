//go:build darwin

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type harness struct {
	home   string
	stdout bytes.Buffer
	stderr bytes.Buffer
	now    time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{home: t.TempDir(), now: time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)}
	for _, d := range []string{".config/dusk", ".m2/repository", "Library/Caches/Homebrew"} {
		if err := os.MkdirAll(filepath.Join(h.home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func (h *harness) writeConfig(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.home, ".config", "dusk", "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) run(args ...string) int {
	h.stdout.Reset()
	h.stderr.Reset()
	return Main(args, App{
		Info:   BuildInfo{Version: "1.2.3", Commit: "abc", Date: "2026-09-05"},
		Stdout: &h.stdout, Stderr: &h.stderr,
		Getenv: func(k string) string {
			if k == "HOME" {
				return h.home
			}
			return ""
		},
		Now: func() time.Time { return h.now },
	})
}

const minimalConfig = `
[[dir_rules]]
rule_name = "maven-repository"
path      = "~/.m2/repository"
max_size  = "6GB"

[[command_rules]]
rule_name       = "homebrew-cache"
path            = "~/Library/Caches/Homebrew"
max_size        = "1GB"
cleanup_command = ["/usr/bin/true"]
`

func TestVersion(t *testing.T) {
	h := newHarness(t)
	if code := h.run("version"); code != 0 {
		t.Fatalf("exit %d, stderr %s", code, h.stderr.String())
	}
	if got := h.stdout.String(); got != "dusk 1.2.3 (commit abc, built 2026-09-05)\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t)
	if code := h.run("frobnicate"); code != ExitUsage {
		t.Errorf("unknown command exit = %d, want 64", code)
	}
	if code := h.run("version", "--bogus"); code != ExitUsage {
		t.Errorf("unknown flag exit = %d, want 64", code)
	}
	if code := h.run("--color=sometimes", "version"); code != ExitUsage {
		t.Errorf("bad colour mode exit = %d, want 64", code)
	}
}

func TestConfigPathAndValidate(t *testing.T) {
	h := newHarness(t)
	if code := h.run("config", "path"); code != 0 || strings.TrimSpace(h.stdout.String()) != filepath.Join(h.home, ".config", "dusk", "config.toml") {
		t.Errorf("config path: exit %d, out %q", code, h.stdout.String())
	}
	if code := h.run("config", "validate"); code != ExitConfig {
		t.Errorf("missing config exit = %d, want 78", code)
	}
	h.writeConfig(t, minimalConfig)
	if code := h.run("config", "validate"); code != 0 || !strings.Contains(h.stdout.String(), "config valid") {
		t.Errorf("valid config: exit %d, out %q, err %q", code, h.stdout.String(), h.stderr.String())
	}
	h.writeConfig(t, minimalConfig+"\n[[dir_rules]]\nrule_name = \"m\"\npath = \"~/missing\"\nmax_size = \"1GB\"\ncleanup_command = [\"x\"]\n")
	if code := h.run("config", "validate"); code != ExitConfig || !strings.Contains(h.stderr.String(), "unknown key") {
		t.Errorf("unknown key: exit %d, err %q", code, h.stderr.String())
	}
	h.writeConfig(t, minimalConfig+"\n[[dir_rules]]\nrule_name = \"m\"\npath = \"~/missing\"\nmax_size = \"1GB\"\n")
	if code := h.run("config", "validate"); code != 0 || !strings.Contains(h.stdout.String(), "warning: ") || !strings.Contains(h.stdout.String(), "does not exist yet") {
		t.Errorf("warning: exit %d, out %q", code, h.stdout.String())
	}
	if code := h.run("--config", filepath.Join(h.home, "nope.toml"), "config", "validate"); code != ExitConfig {
		t.Errorf("--config override exit = %d, want 78", code)
	}
}

func TestRules(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(t, minimalConfig)
	if code := h.run("rules"); code != 0 {
		t.Fatalf("exit %d, stderr %s", code, h.stderr.String())
	}
	out := h.stdout.String()
	for _, want := range []string{"maven-repository", "6.0 GB", "oldest_first", "homebrew-cache", "command", "unmeasured"} {
		if !strings.Contains(out, want) {
			t.Errorf("rules lacks %q:\n%s", want, out)
		}
	}
	if code := h.run("rules", "--json"); code != 0 {
		t.Fatalf("json exit %d", code)
	}
	var rows []map[string]any
	if err := json.Unmarshal(h.stdout.Bytes(), &rows); err != nil || len(rows) != 2 || rows[0]["rule_name"] != "maven-repository" || rows[0]["max_bytes"].(float64) != 6e9 {
		t.Errorf("rules --json = %s, %v", h.stdout.String(), err)
	}
	if code := h.run("rules", "--iec"); code != 0 || !strings.Contains(h.stdout.String(), "5.6 GiB") {
		t.Errorf("--iec: exit %d, out %q", code, h.stdout.String())
	}
}
