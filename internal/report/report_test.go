//go:build darwin

package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

func TestBytes(t *testing.T) {
	dec := Format{}
	for _, c := range []struct {
		n    int64
		want string
	}{{41_200_000_000, "41.2 GB"}, {999_999_999, "1000 MB"}, {128_000_000, "128 MB"}, {4096, "4 KB"}, {512, "512 B"}, {0, "0 B"}} {
		if got := dec.Bytes(c.n); got != c.want {
			t.Errorf("decimal %d = %q, want %q", c.n, got, c.want)
		}
	}
	iec := Format{IEC: true}
	for _, c := range []struct {
		n    int64
		want string
	}{{5 << 30, "5.0 GiB"}, {128 << 20, "128 MiB"}, {4096, "4 KiB"}} {
		if got := iec.Bytes(c.n); got != c.want {
			t.Errorf("iec %d = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestColorEnabled(t *testing.T) {
	none := func(string) string { return "" }
	noColor := func(k string) string {
		if k == "NO_COLOR" {
			return "1"
		}
		return ""
	}
	for _, c := range []struct {
		flag string
		env  func(string) string
		tty  bool
		want bool
	}{
		{"always", noColor, false, true},
		{"never", none, true, false},
		{"auto", noColor, true, false},
		{"auto", none, true, true},
		{"auto", none, false, false},
	} {
		got, err := ColorEnabled(c.flag, c.env, c.tty)
		if err != nil || got != c.want {
			t.Errorf("ColorEnabled(%s, tty=%v) = %v, %v; want %v", c.flag, c.tty, got, err, c.want)
		}
	}
	if _, err := ColorEnabled("sometimes", none, true); err == nil {
		t.Error("unknown colour mode must error")
	}
	if (Palette{Enabled: true}).State("warn") == "warn" || (Palette{}).State("warn") != "warn" {
		t.Error("palette must colour only when enabled")
	}
}

func promptConfig() config.Prompt { return config.Default().Prompt }

func status(stateName string, freePct float64, free int64, overMax int, ts time.Time) *state.Status {
	return &state.Status{
		Schema: 1, TS: ts, StaleAfter: ts.Add(time.Hour),
		Disk:    state.DiskStatus{State: stateName, FreePct: freePct, UsableFreeBytes: free},
		Summary: state.Summary{State: stateName, OverMaxCount: overMax},
	}
}

func TestRenderPrompt(t *testing.T) {
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	p := promptConfig()
	if got := RenderPrompt(nil, p, now); got != "" {
		t.Errorf("no status = %q, want empty", got)
	}
	if got := RenderPrompt(status("ok", 40, 100e9, 0, now), p, now); got != "" {
		t.Errorf("ok = %q, want empty", got)
	}
	if got := RenderPrompt(status("warn", 16.7, 41e9, 0, now), p, now); got != "󰋊 17% free" {
		t.Errorf("warn = %q", got)
	}
	if got := RenderPrompt(status("critical", 7.2, 9.4e9, 3, now), p, now); got != "󰋊 9GB free! · 3 over max" {
		t.Errorf("critical = %q", got)
	}
	if got := RenderPrompt(status("ok", 40, 100e9, 2, now), p, now); got != "· 2 over max" {
		t.Errorf("ok with over max = %q", got)
	}
	if got := RenderPrompt(status("warn", 16.7, 41e9, 0, now), p, now.Add(2*time.Hour)); got != "󰋊 dusk stale" {
		t.Errorf("stale = %q", got)
	}
}

func TestWriteRules(t *testing.T) {
	var buf bytes.Buffer
	size := int64(9e9)
	WriteRules(&buf, []RuleRow{
		{Name: "maven-repository", Kind: "dir", Path: "/Users/x/.m2/repository", Max: 6e9, Mode: "oldest_first", Unit: "files", LastSize: &size},
		{Name: "homebrew-cache", Kind: "command", Path: "/Users/x/Library/Caches/Homebrew", Max: 1e9},
	}, Format{})
	out := buf.String()
	for _, want := range []string{"NAME", "maven-repository", "dir", "6.0 GB", "oldest_first", "9.0 GB", "homebrew-cache", "command", "unmeasured"} {
		if !strings.Contains(out, want) {
			t.Errorf("rules table lacks %q:\n%s", want, out)
		}
	}
}

func TestWriteStatusAndReport(t *testing.T) {
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	st := status("warn", 16.7, 41e9, 1, now)
	st.Disk.Volume = "/System/Volumes/Data"
	st.Disk.TotalBytes = 245e9
	st.Disk.StateSince = now.Add(-26 * time.Hour)
	measured := now.Add(-time.Hour)
	st.Rules = []state.RuleStatus{{RuleName: "maven-repository", Kind: "dir", AllocatedBytes: 9e9, MaxBytes: 6e9, OverMax: true, MeasuredAt: &measured}}
	var buf bytes.Buffer
	WriteStatus(&buf, st, Format{}, Palette{}, now)
	out := buf.String()
	for _, want := range []string{"warn", "41.0 GB", "16.7%", "/System/Volumes/Data", "maven-repository", "over", "1d 2h"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output lacks %q:\n%s", want, out)
		}
	}

	g7 := int64(1.5e9)
	rep := Report{TS: now, Disk: st.Disk, Rules: []ReportRule{{
		Name: "maven-repository", Kind: "dir", Path: "/Users/x/.m2/repository", Allocated: 9e9, Apparent: 9.5e9, Max: 6e9, OverMax: true, Growth7d: &g7,
		Largest: []ReportUnit{
			{RelPath: "org/big.jar", Allocated: 500e6, ModTime: now.Add(-72 * time.Hour), Freeable: true},
			{RelPath: "org/linked.jar", Allocated: 100e6, ModTime: now.Add(-24 * time.Hour), Freeable: false},
		},
		Skipped: map[string]int{"permission": 2},
	}, {Name: "gone", Kind: "dir", Path: "/nope", Err: "lstat /nope: no such file or directory"}}}
	buf.Reset()
	WriteReport(&buf, rep, Format{}, Palette{})
	out = buf.String()
	for _, want := range []string{"maven-repository", "9.0 GB", "6.0 GB", "+1.5 GB", "org/big.jar", "500 MB", "org/linked.jar", "(not freeable)", "permission: 2", "gone", "no such file"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output lacks %q:\n%s", want, out)
		}
	}
}
