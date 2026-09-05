//go:build darwin

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/scan"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

func TestGrowthSince(t *testing.T) {
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	samples := []state.Sample{
		{TS: now.Add(-40 * day), Rule: "m", Allocated: 1e9},
		{TS: now.Add(-20 * day), Rule: "m", Allocated: 5e9},
		{TS: now.Add(-6 * day), Rule: "m", Allocated: 8e9},
		{TS: now.Add(-1 * day), Rule: "other", Allocated: 1},
	}
	if g := growthSince(samples, "m", 9e9, now, 7*day); g == nil || *g != 1e9 {
		t.Errorf("7d growth = %v, want 1 GB against the 6 day old sample", deref(g))
	}
	if g := growthSince(samples, "m", 9e9, now, 30*day); g == nil || *g != 4e9 {
		t.Errorf("30d growth = %v, want 4 GB against the 20 day old sample", deref(g))
	}
	if g := growthSince(samples, "m", 9e9, now, 2*day); g != nil {
		t.Errorf("no sample inside 2d must give nil, got %v", *g)
	}
	if g := growthSince(samples, "new", 1, now, 7*day); g != nil {
		t.Error("unknown rule must give nil")
	}
}

func deref(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestLargestUnits(t *testing.T) {
	units := []scan.Unit{
		{RelPath: "a", Allocated: 1, Freeable: true},
		{RelPath: "b", Allocated: 30, Freeable: false},
		{RelPath: "c", Allocated: 20, Freeable: true},
		{RelPath: "d", Allocated: 10, Freeable: true},
	}
	got := largestUnits(units, 3)
	if len(got) != 3 || got[0].RelPath != "b" || got[1].RelPath != "c" || got[2].RelPath != "d" {
		t.Errorf("largest = %+v", got)
	}
	if got[0].Freeable != false {
		t.Errorf("largest[0].Freeable = %v, want false", got[0].Freeable)
	}
	if got[1].Freeable != true {
		t.Errorf("largest[1].Freeable = %v, want true", got[1].Freeable)
	}
	if len(largestUnits(units[:1], 3)) != 1 {
		t.Error("fewer units than requested returns them all")
	}
}

func TestReportCommand(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(t, lenientConfig+"\n[[dir_rules]]\nrule_name = \"gone\"\npath = \"~/gone\"\nmax_size = \"1GB\"\n")
	if err := os.WriteFile(filepath.Join(h.home, ".m2", "repository", "big.jar"), make([]byte, 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, _ := state.DefaultDir(func(string) string { return "" }, h.home)
	if err := dir.AppendSamples([]state.Sample{{TS: h.now.Add(-3 * 24 * time.Hour), Rule: "maven-repository", Allocated: 4096}}); err != nil {
		t.Fatal(err)
	}
	if code := h.run("report"); code != 0 {
		t.Fatalf("exit %d, stderr %s", code, h.stderr.String())
	}
	out := h.stdout.String()
	for _, want := range []string{"maven-repository", "over", "big.jar", "+4 KB", "gone", "not measured"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if _, err := dir.ReadStatus(); err == nil {
		t.Error("report must not write status.json")
	}
	if code := h.run("report", "--json"); code != 0 {
		t.Fatal(h.stderr.String())
	}
	var raw struct {
		Rules []struct {
			Name     string `json:"name"`
			Growth7d *int64 `json:"growth_7d_bytes"`
			Err      string `json:"error"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &raw); err != nil || len(raw.Rules) != 2 || raw.Rules[0].Growth7d == nil || raw.Rules[1].Err == "" {
		t.Errorf("report --json = %s, %v", h.stdout.String(), err)
	}
}
