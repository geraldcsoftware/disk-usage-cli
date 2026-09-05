//go:build darwin

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

// lenientConfig keeps the disk in ok on any machine with more than one
// percent free, so the tests exercise the pipeline rather than the state.
const lenientConfig = `
[disk.warn]
when_free_below  = ["1%"]
clear_when_above = ["2%"]
[disk.critical]
when_free_below  = ["0.5%"]
clear_when_above = ["1%"]
[schedule]
measure_rule_dirs_every = "6h"
[[dir_rules]]
rule_name = "maven-repository"
path      = "~/.m2/repository"
max_size  = "1KB"
`

func (h *harness) statusFile(t *testing.T) *state.Status {
	t.Helper()
	dir, err := state.DefaultDir(func(string) string { return "" }, h.home)
	if err != nil {
		t.Fatal(err)
	}
	st, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestCheckWritesStatusPromptAndSamples(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(t, lenientConfig)
	if err := os.WriteFile(filepath.Join(h.home, ".m2", "repository", "big.jar"), make([]byte, 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := h.run("check"); code != 0 {
		t.Fatalf("check exit %d, stderr %s", code, h.stderr.String())
	}
	st := h.statusFile(t)
	if st.Schema != 1 || st.Disk.State != "ok" || !st.TS.Equal(h.now) || !st.StaleAfter.Equal(h.now.Add(time.Hour)) {
		t.Errorf("status = %+v", st)
	}
	if len(st.Rules) != 1 || st.Rules[0].RuleName != "maven-repository" || !st.Rules[0].OverMax || st.Rules[0].AllocatedBytes < 8192 || st.Rules[0].MeasuredAt == nil {
		t.Errorf("rules = %+v", st.Rules)
	}
	if st.Summary.OverMaxCount != 1 || st.Summary.State != "ok" || st.LastRun.ID == "" {
		t.Errorf("summary = %+v, last run = %+v", st.Summary, st.LastRun)
	}
	dir, _ := state.DefaultDir(func(string) string { return "" }, h.home)
	prompt, _ := dir.ReadPrompt()
	if prompt != "· 1 over max" {
		t.Errorf("prompt = %q", prompt)
	}
	samples, _ := dir.ReadSamples()
	if len(samples) != 2 || samples[0].Rule != "maven-repository" || samples[1].Rule != state.DiskSampleRule {
		t.Errorf("samples = %+v", samples)
	}
}

func TestCheckCarriesRulesForwardUntilDue(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(t, lenientConfig)
	if code := h.run("check"); code != 0 {
		t.Fatal(h.stderr.String())
	}
	first := h.statusFile(t)
	h.now = h.now.Add(30 * time.Minute)
	if code := h.run("check"); code != 0 {
		t.Fatal(h.stderr.String())
	}
	second := h.statusFile(t)
	if !second.Rules[0].MeasuredAt.Equal(*first.Rules[0].MeasuredAt) {
		t.Error("rules must be carried forward before measure_rule_dirs_every elapses")
	}
	if !second.TS.Equal(h.now) {
		t.Error("status.json must still be rewritten every check")
	}
	h.now = h.now.Add(6 * time.Hour)
	if code := h.run("check"); code != 0 {
		t.Fatal(h.stderr.String())
	}
	third := h.statusFile(t)
	if !third.Rules[0].MeasuredAt.Equal(h.now) {
		t.Error("rules must be re-measured once the interval has elapsed")
	}
	h.now = h.now.Add(time.Minute)
	if code := h.run("check", "--full"); code != 0 {
		t.Fatal(h.stderr.String())
	}
	if !h.statusFile(t).Rules[0].MeasuredAt.Equal(h.now) {
		t.Error("--full forces measurement")
	}
	dir, _ := state.DefaultDir(func(string) string { return "" }, h.home)
	samples, _ := dir.ReadSamples()
	if len(samples) != 4+3 {
		t.Errorf("samples = %d, want one disk sample per check and one rule sample per measurement", len(samples))
	}
}

func TestCheckExitCodesAndLock(t *testing.T) {
	h := newHarness(t)
	if code := h.run("check"); code != ExitUnknown {
		t.Errorf("missing config exit = %d, want 3", code)
	}
	h.writeConfig(t, lenientConfig)
	dir, _ := state.DefaultDir(func(string) string { return "" }, h.home)
	release, err := dir.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if code := h.run("check"); code != ExitLocked {
		t.Errorf("locked exit = %d, want 75", code)
	}
	release()
	if code := h.run("check"); code != 0 {
		t.Errorf("unlocked exit = %d, stderr %s", code, h.stderr.String())
	}
}

func TestStatusCommand(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(t, lenientConfig)
	if err := os.WriteFile(filepath.Join(h.home, ".m2", "repository", "big.jar"), make([]byte, 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := h.run("status"); code != 0 || !strings.Contains(h.stderr.String(), "no status yet") {
		t.Errorf("status before check: exit %d, err %q", code, h.stderr.String())
	}
	if code := h.run("status", "--prompt"); code != 0 || h.stdout.String() != "" {
		t.Errorf("prompt before check must be empty: exit %d, out %q", code, h.stdout.String())
	}
	if code := h.run("check"); code != 0 {
		t.Fatal(h.stderr.String())
	}
	if code := h.run("status"); code != 0 || !strings.Contains(h.stdout.String(), "maven-repository") || !strings.Contains(h.stdout.String(), "state ok") {
		t.Errorf("status: exit %d, out %q", code, h.stdout.String())
	}
	if code := h.run("status", "--prompt"); code != 0 || h.stdout.String() != "· 1 over max" {
		t.Errorf("status --prompt = %q", h.stdout.String())
	}
	if code := h.run("status", "--json"); code != 0 {
		t.Fatal(h.stderr.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(h.stdout.Bytes(), &raw); err != nil || raw["schema"].(float64) != 1 {
		t.Errorf("status --json = %s, %v", h.stdout.String(), err)
	}
	h.now = h.now.Add(3 * time.Hour)
	if code := h.run("status", "--prompt"); code != 0 || h.stdout.String() != "󰋊 dusk stale" {
		t.Errorf("stale prompt = %q", h.stdout.String())
	}
}
