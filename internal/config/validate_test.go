//go:build darwin

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHome builds a home directory with the paths validation refers to.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, d := range []string{".m2/repository", "Library/Caches/x", "Library/Mobile Documents/com~apple~CloudDocs/a", "Library/CloudStorage/OneDrive-Personal/b", "Desktop/scratch", "Pictures/Photos Library.photoslibrary/inner", "bin"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "brew"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "notes.txt"), []byte("a file, not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".m2", "repository"), filepath.Join(home, "repository-link")); err != nil {
		t.Fatal(err)
	}
	return home
}

func testValidator(home string) Validator {
	return Validator{
		Home:                home,
		PATH:                filepath.Join(home, "bin"),
		Lstat:               os.Lstat,
		TimeMachineIncluded: func(string) (bool, error) { return false, nil },
		CheckExternal:       true,
	}
}

func parseFor(t *testing.T, home, src string) *Config {
	t.Helper()
	cfg, err := Parse([]byte(src), home)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestValidateAcceptsSpecExample(t *testing.T) {
	home := fakeHome(t)
	if err := os.MkdirAll(filepath.Join(home, "Library/Caches/Homebrew"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := parseFor(t, home, specExample)
	warnings, err := cfg.Validate(testValidator(home))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestValidateErrors(t *testing.T) {
	home := fakeHome(t)
	dir := func(name, path, extra string) string {
		return "[[dir_rules]]\nrule_name = \"" + name + "\"\npath = \"" + path + "\"\nmax_size = \"1GB\"\n" + extra + "\n"
	}
	cases := []struct{ name, src, want string }{
		{"outside home", dir("a", "/tmp", ""), "outside"},
		{"home itself", dir("a", "~", ""), "never be"},
		{"library itself", dir("a", "~/Library", ""), "never be"},
		{"volume root", "[safety]\nallow_paths_outside_home = true\n" + dir("a", "/", ""), "never be"},
		{"data volume root", "[safety]\nallow_paths_outside_home = true\n" + dir("a", "/System/Volumes/Data", ""), "never be"},
		{"icloud", dir("a", "~/Library/Mobile Documents/com~apple~CloudDocs/a", ""), "denied"},
		{"cloudstorage child", dir("a", "~/Library/CloudStorage/OneDrive-Personal/b", ""), "denied"},
		{"photos library", dir("a", "~/Pictures/Photos Library.photoslibrary/inner", ""), "denied"},
		{"desktop without flag", dir("a", "~/Desktop/scratch", ""), "contains_user_data"},
		{"regular file", dir("a", "~/notes.txt", ""), "not a directory"},
		{"symlink to a directory", dir("a", "~/repository-link", ""), "not a directory"},
		{"target not below max", dir("a", "~/.m2/repository", "cleanup_target_size = \"1GB\""), "cleanup_target_size"},
		{"bad mode", dir("a", "~/.m2/repository", "cleanup_mode = \"prune\""), "cleanup_mode"},
		{"bad unit", dir("a", "~/.m2/repository", "cleanup_unit = \"dirs\""), "cleanup_unit"},
		{"bad glob", dir("a", "~/.m2/repository", "exclude_patterns = [\"[\"]"), "exclude_patterns"},
		{"bad name", dir("Bad_Name", "~/.m2/repository", ""), "rule_name"},
		{"duplicate names", dir("a", "~/.m2/repository", "") + "[[command_rules]]\nrule_name = \"a\"\npath = \"~/Library/Caches/x\"\nmax_size = \"1GB\"\ncleanup_command = [\"brew\"]\n", "unique"},
		{"shell command", "[[command_rules]]\nrule_name = \"c\"\npath = \"~/Library/Caches/x\"\nmax_size = \"1GB\"\ncleanup_command = [\"sh\", \"-c\", \"brew cleanup\"]\n", "cleanup_command"},
		{"rm preview", "[[command_rules]]\nrule_name = \"c\"\npath = \"~/Library/Caches/x\"\nmax_size = \"1GB\"\ncleanup_command = [\"brew\"]\npreview_command = [\"rm\", \"-rf\", \"x\"]\n", "preview_command"},
		{"empty command", "[[command_rules]]\nrule_name = \"c\"\npath = \"~/Library/Caches/x\"\nmax_size = \"1GB\"\n", "cleanup_command"},
		{"bad check_every", "[schedule]\ncheck_every = \"45m\"\n", "check_every"},
		{"empty thresholds", "[disk.warn]\nwhen_free_below = []\n", "when_free_below"},
		{"bad quiet hours", "[disk]\nquiet_hours = [\"night\"]\n", "quiet_hours"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := parseFor(t, home, c.src)
			_, err := cfg.Validate(testValidator(home))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want mention of %q", err, c.want)
			}
		})
	}
}

func TestValidateUserDataForcesManual(t *testing.T) {
	home := fakeHome(t)
	cfg := parseFor(t, home, "[[dir_rules]]\nrule_name = \"d\"\npath = \"~/Desktop/scratch\"\nmax_size = \"1GB\"\nauto_cleanup = true\ncontains_user_data = true\n")
	if _, err := cfg.Validate(testValidator(home)); err != nil {
		t.Fatal(err)
	}
	if cfg.DirRules[0].AutoCleanup {
		t.Error("contains_user_data must force auto_cleanup to false")
	}
}

func TestValidateOutsideHomeWhenAllowed(t *testing.T) {
	home := fakeHome(t)
	other := t.TempDir()
	cfg := parseFor(t, home, "[safety]\nallow_paths_outside_home = true\n[[dir_rules]]\nrule_name = \"o\"\npath = \""+other+"\"\nmax_size = \"1GB\"\n")
	if _, err := cfg.Validate(testValidator(home)); err != nil {
		t.Errorf("outside home should pass when allowed: %v", err)
	}
}

func TestValidateWarnings(t *testing.T) {
	home := fakeHome(t)
	src := "[[dir_rules]]\nrule_name = \"missing\"\npath = \"~/not-yet\"\nmax_size = \"1GB\"\n" +
		"[[command_rules]]\nrule_name = \"c\"\npath = \"~/Library/Caches/x\"\nmax_size = \"1GB\"\ncleanup_command = [\"nosuchtool\", \"prune\"]\n"
	cfg := parseFor(t, home, src)
	v := testValidator(home)
	v.TimeMachineIncluded = func(p string) (bool, error) { return strings.HasSuffix(p, "Caches/x"), nil }
	warnings, err := cfg.Validate(v)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"does not exist yet", "nosuchtool", "Time Machine"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings %q lack %q", joined, want)
		}
	}
}

func TestValidateSkipsExternalChecksByDefault(t *testing.T) {
	home := fakeHome(t)
	src := "[[dir_rules]]\nrule_name = \"missing\"\npath = \"~/not-yet\"\nmax_size = \"1GB\"\n" +
		"[[command_rules]]\nrule_name = \"c\"\npath = \"~/Library/Caches/x\"\nmax_size = \"1GB\"\ncleanup_command = [\"nosuchtool\", \"prune\"]\n"
	cfg := parseFor(t, home, src)
	v := testValidator(home)
	v.CheckExternal = false
	v.TimeMachineIncluded = func(string) (bool, error) {
		t.Error("Time Machine must not be consulted when CheckExternal is false")
		return true, nil
	}
	warnings, err := cfg.Validate(v)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "does not exist yet") {
		t.Errorf("warnings %q lack the missing path warning", joined)
	}
	for _, unwanted := range []string{"nosuchtool", "Time Machine"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("warnings %q mention %q, which needs an external check", joined, unwanted)
		}
	}
}

func TestLookPath(t *testing.T) {
	home := fakeHome(t)
	p, err := LookPath("brew", filepath.Join(home, "bin")+":/nonexistent")
	if err != nil || p != filepath.Join(home, "bin", "brew") {
		t.Errorf("LookPath = %q, %v", p, err)
	}
	if _, err := LookPath("nothing", filepath.Join(home, "bin")); err == nil {
		t.Error("missing binary should error")
	}
	if got := FixedPATH("/Users/x"); got != "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/Users/x/.local/bin:/Users/x/go/bin" {
		t.Errorf("FixedPATH = %q", got)
	}
}

func TestCalendarEntries(t *testing.T) {
	half, err := CalendarEntries(Duration(30 * 60 * 1e9))
	if err != nil || len(half) != 2 || half[0] != (CalendarEntry{Hour: -1, Minute: 5}) || half[1] != (CalendarEntry{Hour: -1, Minute: 35}) {
		t.Errorf("30m = %+v, %v", half, err)
	}
	six, err := CalendarEntries(Duration(6 * 3600 * 1e9))
	if err != nil || len(six) != 4 || six[0] != (CalendarEntry{Hour: 0, Minute: 5}) || six[3] != (CalendarEntry{Hour: 18, Minute: 5}) {
		t.Errorf("6h = %+v, %v", six, err)
	}
	day, err := CalendarEntries(Duration(24 * 3600 * 1e9))
	if err != nil || len(day) != 1 || day[0] != (CalendarEntry{Hour: 0, Minute: 5}) {
		t.Errorf("24h = %+v, %v", day, err)
	}
	if _, err := CalendarEntries(Duration(45 * 60 * 1e9)); err == nil {
		t.Error("45m must be rejected")
	}
}
