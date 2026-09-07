//go:build darwin

package config

import (
	"strings"
	"testing"
	"time"
)

const specExample = `
[disk]
volume_path      = "~"
renotify_every   = "4h"
quiet_hours      = []

[disk.warn]
when_free_below  = ["15%", "25GB"]
clear_when_above = ["20%", "30GB"]

[disk.critical]
when_free_below  = ["8%", "10GB"]
clear_when_above = ["12%", "15GB"]

[schedule]
check_every             = "30m"
measure_rule_dirs_every = "6h"

[notifications]
on_disk_state_change  = true
on_rule_over_max_size = true
after_auto_cleanup    = true
sound                 = false
click_activates       = "com.apple.Terminal"

[prompt]
text_when_ok       = ""
text_when_warn     = "󰋊 {free_pct}% free"
text_when_critical = "󰋊 {free_gb}GB free!"
text_when_stale    = "󰋊 dusk stale"
over_max_suffix    = " · {over_max_count} over max"
stale_after        = "1h"

[safety]
allow_paths_outside_home = false
ballast_size             = "128MB"

[[dir_rules]]
rule_name            = "maven-repository"
path                 = "~/.m2/repository"
max_size             = "6GB"
auto_cleanup         = false
cleanup_mode         = "oldest_first"
cleanup_target_size  = "5GB"
cleanup_unit         = "files"
keep_newer_than      = "7d"
exclude_patterns     = ["**/*.lock"]
skip_if_running      = ["java"]
max_delete_per_run   = "0"
contains_user_data   = false

[[command_rules]]
rule_name          = "homebrew-cache"
path               = "~/Library/Caches/Homebrew"
max_size           = "1GB"
cleanup_command    = ["brew", "cleanup", "--prune=30"]
preview_command    = ["brew", "cleanup", "-n", "--prune=30"]
env                = { HOMEBREW_NO_AUTO_UPDATE = "1", HOMEBREW_NO_ANALYTICS = "1" }
success_exit_codes = [0]
timeout            = "10m"
auto_cleanup       = false
`

func TestParseSpecExample(t *testing.T) {
	cfg, err := Parse([]byte(specExample), "/Users/test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Disk.VolumePath != "/Users/test" {
		t.Errorf("volume_path = %q, want home expanded", cfg.Disk.VolumePath)
	}
	if cfg.Disk.RenotifyEvery.Std() != 4*time.Hour {
		t.Errorf("renotify_every = %v", cfg.Disk.RenotifyEvery.Std())
	}
	if len(cfg.Disk.Warn.WhenFreeBelow) != 2 || !cfg.Disk.Warn.WhenFreeBelow[0].IsPercent || cfg.Disk.Warn.WhenFreeBelow[1].Bytes != 25_000_000_000 {
		t.Errorf("warn.when_free_below = %+v", cfg.Disk.Warn.WhenFreeBelow)
	}
	if cfg.Schedule.CheckEvery.Std() != 30*time.Minute || cfg.Schedule.MeasureRuleDirsEvery.Std() != 6*time.Hour {
		t.Errorf("schedule = %+v", cfg.Schedule)
	}
	if cfg.Prompt.TextWhenWarn != "󰋊 {free_pct}% free" || cfg.Prompt.StaleAfter.Std() != time.Hour {
		t.Errorf("prompt = %+v", cfg.Prompt)
	}
	if cfg.Safety.BallastSize != 128_000_000 {
		t.Errorf("ballast_size = %d", cfg.Safety.BallastSize)
	}
	if len(cfg.DirRules) != 1 {
		t.Fatalf("dir_rules = %d", len(cfg.DirRules))
	}
	d := cfg.DirRules[0]
	if d.RuleName != "maven-repository" || d.Path != "/Users/test/.m2/repository" || d.MaxSize != 6_000_000_000 ||
		d.CleanupMode != "oldest_first" || d.CleanupTargetSize != 5_000_000_000 || d.CleanupUnit != "files" ||
		d.KeepNewerThan.Std() != 7*24*time.Hour || d.ExcludePatterns[0] != "**/*.lock" || d.SkipIfRunning[0] != "java" ||
		d.MaxDeletePerRun != 0 || d.ContainsUserData || d.AutoCleanup {
		t.Errorf("dir rule = %+v", d)
	}
	if len(cfg.CommandRules) != 1 {
		t.Fatalf("command_rules = %d", len(cfg.CommandRules))
	}
	c := cfg.CommandRules[0]
	if c.RuleName != "homebrew-cache" || c.Path != "/Users/test/Library/Caches/Homebrew" || c.MaxSize != 1_000_000_000 ||
		strings.Join(c.CleanupCommand, " ") != "brew cleanup --prune=30" || strings.Join(c.PreviewCommand, " ") != "brew cleanup -n --prune=30" ||
		c.Env["HOMEBREW_NO_AUTO_UPDATE"] != "1" || len(c.SuccessExitCodes) != 1 || c.SuccessExitCodes[0] != 0 ||
		c.Timeout.Std() != 10*time.Minute || c.AutoCleanup {
		t.Errorf("command rule = %+v", c)
	}
}

func TestParseAppliesDefaults(t *testing.T) {
	src := `
[[dir_rules]]
rule_name = "x"
path      = "~/cache"
max_size  = "10GB"

[[command_rules]]
rule_name       = "y"
path            = "~/other"
max_size        = "1GB"
cleanup_command = ["true"]
`
	cfg, err := Parse([]byte(src), "/Users/test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Schedule.CheckEvery.Std() != 30*time.Minute {
		t.Errorf("default check_every = %v", cfg.Schedule.CheckEvery.Std())
	}
	if cfg.Prompt.StaleAfter.Std() != time.Hour {
		t.Errorf("default stale_after = %v, want twice check_every", cfg.Prompt.StaleAfter.Std())
	}
	if len(cfg.Disk.Warn.WhenFreeBelow) != 2 || len(cfg.Disk.Critical.ClearWhenAbove) != 2 {
		t.Errorf("default thresholds missing: %+v", cfg.Disk)
	}
	if !cfg.Notifications.OnDiskStateChange || cfg.Notifications.ClickActivates != "com.apple.Terminal" {
		t.Errorf("default notifications = %+v", cfg.Notifications)
	}
	d := cfg.DirRules[0]
	if d.CleanupMode != "oldest_first" || d.CleanupUnit != "files" || d.KeepNewerThan.Std() != time.Hour || d.CleanupTargetSize != 9_000_000_000 {
		t.Errorf("dir rule defaults = %+v", d)
	}
	c := cfg.CommandRules[0]
	if len(c.SuccessExitCodes) != 1 || c.SuccessExitCodes[0] != 0 || c.Timeout.Std() != 10*time.Minute {
		t.Errorf("command rule defaults = %+v", c)
	}
}

func TestParseRejectsUnknownKeys(t *testing.T) {
	src := "[disk]\nvolume_path = \"~\"\nfree_below = [\"10%\"]\n"
	_, err := Parse([]byte(src), "/Users/test")
	var ve *ValidationError
	if err == nil || !asValidationError(err, &ve) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if len(ve.Errors) != 1 || !strings.Contains(ve.Errors[0], "line 3") || !strings.Contains(ve.Errors[0], "disk.free_below") {
		t.Errorf("errors = %v", ve.Errors)
	}
}

func TestParseRejectsBadValues(t *testing.T) {
	src := "[[dir_rules]]\nrule_name = \"x\"\npath = \"~/c\"\nmax_size = \"10G\"\n"
	if _, err := Parse([]byte(src), "/Users/test"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguous size", err)
	}
}

func TestDefaultPath(t *testing.T) {
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return "/tmp/xdg"
		}
		return ""
	}
	if got := DefaultPath(env, "/Users/test"); got != "/tmp/xdg/dusk/config.toml" {
		t.Errorf("with XDG = %q", got)
	}
	if got := DefaultPath(func(string) string { return "" }, "/Users/test"); got != "/Users/test/.config/dusk/config.toml" {
		t.Errorf("without XDG = %q", got)
	}
}

func TestRules(t *testing.T) {
	cfg, err := Parse([]byte(specExample), "/Users/test")
	if err != nil {
		t.Fatal(err)
	}
	refs := cfg.Rules()
	if len(refs) != 2 || refs[0].Kind != "dir" || refs[0].Mode != "oldest_first" || refs[1].Kind != "command" || refs[1].Name != "homebrew-cache" {
		t.Errorf("Rules() = %+v", refs)
	}
}

func asValidationError(err error, target **ValidationError) bool {
	ve, ok := err.(*ValidationError)
	if ok {
		*target = ve
	}
	return ok
}
