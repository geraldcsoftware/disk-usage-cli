//go:build darwin

package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func testDir(t *testing.T) Dir {
	t.Helper()
	home := t.TempDir()
	d, err := DefaultDir(func(string) string { return "" }, home)
	if err != nil {
		t.Fatal(err)
	}
	if string(d) != filepath.Join(home, ".local", "state", "dusk") {
		t.Fatalf("dir = %s", d)
	}
	fi, err := os.Stat(string(d))
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("state dir mode = %v, %v; want 0700", fi.Mode(), err)
	}
	return d
}

func TestDefaultDirHonoursXDG(t *testing.T) {
	base := t.TempDir()
	d, err := DefaultDir(func(k string) string {
		if k == "XDG_STATE_HOME" {
			return base
		}
		return ""
	}, "/Users/ignored")
	if err != nil || string(d) != filepath.Join(base, "dusk") {
		t.Errorf("dir = %s, %v", d, err)
	}
}

func TestLockIsExclusive(t *testing.T) {
	d := testDir(t)
	release, err := d.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Lock(); !errors.Is(err, ErrLocked) {
		t.Errorf("second lock error = %v, want ErrLocked", err)
	}
	release()
	release2, err := d.Lock()
	if err != nil {
		t.Errorf("lock after release: %v", err)
	}
	release2()
}

func TestULID(t *testing.T) {
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	a, b := NewULID(now), NewULID(now)
	pattern := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	if !pattern.MatchString(a) || !pattern.MatchString(b) {
		t.Errorf("ULIDs %q %q are not Crockford base32 of length 26", a, b)
	}
	if a == b {
		t.Error("two ULIDs from the same millisecond must differ in their random part")
	}
	if a[:10] != b[:10] {
		t.Error("same timestamp must share the first ten characters")
	}
	later := NewULID(now.Add(time.Second))
	if later[:10] <= a[:10] {
		t.Error("ULIDs must sort by time")
	}
}

func sampleStatus(now time.Time) *Status {
	measured := now.Add(-time.Hour)
	return &Status{
		Schema: 1, TS: now, StaleAfter: now.Add(time.Hour),
		Disk:    DiskStatus{Volume: "/System/Volumes/Data", TotalBytes: 245_107_195_904, UsableFreeBytes: 40_976_000_000, FreePct: 16.7, State: "warn", StateSince: now.Add(-24 * time.Hour)},
		Rules:   []RuleStatus{{RuleName: "maven-repository", Kind: "dir", AllocatedBytes: 9e9, MaxBytes: 6e9, OverMax: true, MeasuredAt: &measured}},
		Summary: Summary{State: "warn", OverMaxCount: 1},
		LastRun: LastRun{ID: "01J0000000000000000000000X", RulesCleaned: []string{}},
	}
}

func TestStatusRoundTripThroughPreallocatedFile(t *testing.T) {
	d := testDir(t)
	if _, err := d.ReadStatus(); !errors.Is(err, ErrNoStatus) {
		t.Fatalf("missing status error = %v, want ErrNoStatus", err)
	}
	now := time.Date(2026, 9, 5, 10, 30, 5, 0, time.UTC)
	if err := d.WriteStatus(sampleStatus(now)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(d.Path("status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64<<10 {
		t.Errorf("status.json is %d bytes, want the full 64 KiB", len(raw))
	}
	if !bytes.HasSuffix(raw, []byte("   ")) || !bytes.HasSuffix(bytes.TrimRight(raw, " "), []byte("}")) {
		t.Error("status.json must be JSON followed by space padding")
	}
	var st unix.Stat_t
	if err := unix.Stat(d.Path("status.json"), &st); err != nil {
		t.Fatal(err)
	}
	if st.Blocks*512 < 64<<10 {
		t.Errorf("allocated %d, want at least 64 KiB of real blocks", st.Blocks*512)
	}
	got, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if got.Disk.State != "warn" || got.Rules[0].RuleName != "maven-repository" || !got.TS.Equal(now) || got.Summary.OverMaxCount != 1 {
		t.Errorf("round trip = %+v", got)
	}
	// A second, shorter write must not leave stale bytes behind.
	short := sampleStatus(now.Add(time.Minute))
	short.Rules = nil
	if err := d.WriteStatus(short); err != nil {
		t.Fatal(err)
	}
	again, err := d.ReadStatus()
	if err != nil || len(again.Rules) != 0 {
		t.Errorf("second read = %+v, %v", again, err)
	}
}

func TestStatusWriteRepairsSparseFile(t *testing.T) {
	d := testDir(t)
	f, err := os.Create(d.Path("status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 10); err != nil {
		t.Fatal(err)
	}
	f.Close()
	var before unix.Stat_t
	if err := unix.Stat(d.Path("status.json"), &before); err != nil {
		t.Fatal(err)
	}
	if before.Blocks != 0 {
		t.Fatalf("sparse fixture already has %d blocks allocated", before.Blocks)
	}
	now := time.Date(2026, 9, 5, 10, 30, 5, 0, time.UTC)
	if err := d.WriteStatus(sampleStatus(now)); err != nil {
		t.Fatal(err)
	}
	var after unix.Stat_t
	if err := unix.Stat(d.Path("status.json"), &after); err != nil {
		t.Fatal(err)
	}
	if after.Blocks*512 < 64<<10 {
		t.Errorf("allocated %d after write, want at least 64 KiB of real blocks", after.Blocks*512)
	}
	raw, err := os.ReadFile(d.Path("status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64<<10 {
		t.Errorf("status.json is %d bytes, want the full 64 KiB", len(raw))
	}
	got, err := d.ReadStatus()
	if err != nil || got.Disk.State != "warn" || !got.TS.Equal(now) {
		t.Errorf("round trip after sparse repair = %+v, %v", got, err)
	}
}

func TestStatusTooLargeIsRefused(t *testing.T) {
	d := testDir(t)
	s := sampleStatus(time.Now())
	for i := 0; i < 2000; i++ {
		s.Rules = append(s.Rules, RuleStatus{RuleName: strings.Repeat("x", 40), Kind: "dir"})
	}
	if err := d.WriteStatus(s); err == nil {
		t.Error("a status over 64 KiB must be refused, not truncated")
	}
}

func TestPromptWriteAndRead(t *testing.T) {
	d := testDir(t)
	if got, err := d.ReadPrompt(); err != nil || got != "" {
		t.Errorf("missing prompt = %q, %v; want empty and nil", got, err)
	}
	if err := d.WritePrompt("󰋊 17% free"); err != nil {
		t.Fatal(err)
	}
	got, err := d.ReadPrompt()
	if err != nil || got != "󰋊 17% free" {
		t.Errorf("prompt = %q, %v", got, err)
	}
	entries, _ := os.ReadDir(string(d))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "prompt-") {
			t.Errorf("temporary file %s left behind", e.Name())
		}
	}
}

func TestSamplesAppendReadCompact(t *testing.T) {
	d := testDir(t)
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	old := Sample{TS: now.Add(-200 * 24 * time.Hour), Rule: "maven-repository", Allocated: 1}
	recent := Sample{TS: now.Add(-time.Hour), Rule: "maven-repository", Allocated: 2, Units: 10}
	disk := Sample{TS: now, Rule: "_disk", UsableFree: 40e9, Total: 245e9}
	if err := d.AppendSamples([]Sample{old, recent}); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendSamples([]Sample{disk}); err != nil {
		t.Fatal(err)
	}
	all, err := d.ReadSamples()
	if err != nil || len(all) != 3 || all[2].Rule != "_disk" || all[1].Units != 10 {
		t.Fatalf("samples = %+v, %v", all, err)
	}
	raw, _ := os.ReadFile(d.Path("samples.jsonl"))
	if strings.Count(string(raw), "\n") != 3 || strings.Contains(string(raw), `"units":0`) {
		t.Errorf("samples.jsonl = %q; want three lines with omitted zero fields", raw)
	}
	if err := d.CompactSamples(now.Add(-180 * 24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	kept, err := d.ReadSamples()
	if err != nil || len(kept) != 2 || kept[0].Allocated != 2 {
		t.Errorf("after compaction = %+v, %v", kept, err)
	}
}
