//go:build darwin

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the parsed and defaulted content of config.toml.
type Config struct {
	Disk          Disk          `toml:"disk"`
	Schedule      Schedule      `toml:"schedule"`
	Notifications Notifications `toml:"notifications"`
	Prompt        Prompt        `toml:"prompt"`
	Safety        Safety        `toml:"safety"`
	DirRules      []DirRule     `toml:"dir_rules"`
	CommandRules  []CommandRule `toml:"command_rules"`
}

type Disk struct {
	VolumePath    string   `toml:"volume_path"`
	RenotifyEvery Duration `toml:"renotify_every"`
	QuietHours    []string `toml:"quiet_hours"`
	Warn          Band     `toml:"warn"`
	Critical      Band     `toml:"critical"`
}

// Band holds the entry and exit thresholds of one disk state.
type Band struct {
	WhenFreeBelow  Thresholds `toml:"when_free_below"`
	ClearWhenAbove Thresholds `toml:"clear_when_above"`
}

type Schedule struct {
	CheckEvery           Duration `toml:"check_every"`
	MeasureRuleDirsEvery Duration `toml:"measure_rule_dirs_every"`
}

type Notifications struct {
	OnDiskStateChange bool   `toml:"on_disk_state_change"`
	OnRuleOverMaxSize bool   `toml:"on_rule_over_max_size"`
	AfterAutoCleanup  bool   `toml:"after_auto_cleanup"`
	Sound             bool   `toml:"sound"`
	ClickActivates    string `toml:"click_activates"`
}

type Prompt struct {
	TextWhenOK       string   `toml:"text_when_ok"`
	TextWhenWarn     string   `toml:"text_when_warn"`
	TextWhenCritical string   `toml:"text_when_critical"`
	TextWhenStale    string   `toml:"text_when_stale"`
	OverMaxSuffix    string   `toml:"over_max_suffix"`
	StaleAfter       Duration `toml:"stale_after"`
}

type Safety struct {
	AllowPathsOutsideHome bool     `toml:"allow_paths_outside_home"`
	BallastSize           ByteSize `toml:"ballast_size"`
}

// DirRule is a directory dusk measures and, in later milestones, cleans itself.
type DirRule struct {
	RuleName          string   `toml:"rule_name"`
	Path              string   `toml:"path"`
	MaxSize           ByteSize `toml:"max_size"`
	AutoCleanup       bool     `toml:"auto_cleanup"`
	CleanupMode       string   `toml:"cleanup_mode"`
	CleanupTargetSize ByteSize `toml:"cleanup_target_size"`
	CleanupUnit       string   `toml:"cleanup_unit"`
	KeepNewerThan     Duration `toml:"keep_newer_than"`
	ExcludePatterns   []string `toml:"exclude_patterns"`
	SkipIfRunning     []string `toml:"skip_if_running"`
	MaxDeletePerRun   ByteSize `toml:"max_delete_per_run"`
	ContainsUserData  bool     `toml:"contains_user_data"`
}

// CommandRule is a directory dusk measures while a configured command does
// the cleaning.
type CommandRule struct {
	RuleName         string            `toml:"rule_name"`
	Path             string            `toml:"path"`
	MaxSize          ByteSize          `toml:"max_size"`
	CleanupCommand   []string          `toml:"cleanup_command"`
	PreviewCommand   []string          `toml:"preview_command"`
	Env              map[string]string `toml:"env"`
	SuccessExitCodes []int             `toml:"success_exit_codes"`
	Timeout          Duration          `toml:"timeout"`
	AutoCleanup      bool              `toml:"auto_cleanup"`
}

// RuleRef is the kind independent view of a rule used by measurement and
// reporting.
type RuleRef struct {
	Name        string
	Kind        string // "dir" or "command"
	Path        string
	MaxSize     ByteSize
	AutoCleanup bool
	Mode        string // dir rules only
	Unit        string // dir rules only
}

// ValidationError lists every problem found in a config, one line each.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return "config: " + strings.Join(e.Errors, "; ")
}

// Default returns the configuration used when a key is absent from the file.
func Default() *Config {
	return &Config{
		Disk: Disk{
			VolumePath:    "~",
			RenotifyEvery: Duration(4 * time.Hour),
			Warn:          Band{WhenFreeBelow: mustThresholds("15%", "25GB"), ClearWhenAbove: mustThresholds("20%", "30GB")},
			Critical:      Band{WhenFreeBelow: mustThresholds("8%", "10GB"), ClearWhenAbove: mustThresholds("12%", "15GB")},
		},
		Schedule: Schedule{CheckEvery: Duration(30 * time.Minute), MeasureRuleDirsEvery: Duration(6 * time.Hour)},
		Notifications: Notifications{
			OnDiskStateChange: true, OnRuleOverMaxSize: true, AfterAutoCleanup: true,
			ClickActivates: "com.apple.Terminal",
		},
		Prompt: Prompt{
			TextWhenWarn:     "󰋊 {free_pct}% free",
			TextWhenCritical: "󰋊 {free_gb}GB free!",
			TextWhenStale:    "󰋊 dusk stale",
			OverMaxSuffix:    " · {over_max_count} over max",
		},
		Safety: Safety{BallastSize: 128_000_000},
	}
}

func mustThresholds(values ...string) Thresholds {
	out := make(Thresholds, 0, len(values))
	for _, v := range values {
		t, err := ParseThreshold(v)
		if err != nil {
			panic(err)
		}
		out = append(out, t)
	}
	return out
}

// DefaultPath resolves $XDG_CONFIG_HOME/dusk/config.toml, falling back to
// ~/.config/dusk/config.toml.
func DefaultPath(getenv func(string) string, home string) string {
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "dusk", "config.toml")
	}
	return filepath.Join(home, ".config", "dusk", "config.toml")
}

// ExpandHome replaces a leading ~ with the home directory. Only path values
// are expanded, so callers pass exactly the fields that hold paths.
func ExpandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Load reads and parses the file at path.
func Load(path, home string) (*Config, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(src, home)
}

// Parse decodes TOML over the defaults, rejects unknown keys with their line
// numbers, expands ~ in path values and fills per rule defaults. It does not
// validate; see Validate.
func Parse(src []byte, home string) (*Config, error) {
	cfg := Default()
	md, err := toml.Decode(string(src), cfg)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		ve := &ValidationError{}
		for _, key := range undecoded {
			if line := lineOf(src, key); line > 0 {
				ve.Errors = append(ve.Errors, fmt.Sprintf("line %d: unknown key %q", line, key.String()))
			} else {
				ve.Errors = append(ve.Errors, fmt.Sprintf("unknown key %q", key.String()))
			}
		}
		return nil, ve
	}
	cfg.Disk.VolumePath = ExpandHome(cfg.Disk.VolumePath, home)
	for i := range cfg.DirRules {
		cfg.DirRules[i].Path = ExpandHome(cfg.DirRules[i].Path, home)
	}
	for i := range cfg.CommandRules {
		cfg.CommandRules[i].Path = ExpandHome(cfg.CommandRules[i].Path, home)
	}
	cfg.applyRuleDefaults()
	return cfg, nil
}

// lineOf finds the first line that assigns the last segment of key. The TOML
// decoder does not report positions for unknown keys, so this is a best
// effort lookup; 0 means not found.
func lineOf(src []byte, key toml.Key) int {
	last := key[len(key)-1]
	for i, line := range strings.Split(string(src), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, last) && strings.HasPrefix(strings.TrimSpace(t[len(last):]), "=") {
			return i + 1
		}
	}
	return 0
}

func (c *Config) applyRuleDefaults() {
	if c.Prompt.StaleAfter == 0 {
		c.Prompt.StaleAfter = Duration(2 * c.Schedule.CheckEvery.Std())
	}
	for i := range c.DirRules {
		r := &c.DirRules[i]
		if r.CleanupMode == "" {
			r.CleanupMode = "oldest_first"
		}
		if r.CleanupUnit == "" {
			r.CleanupUnit = "files"
		}
		if r.KeepNewerThan == 0 {
			r.KeepNewerThan = Duration(time.Hour)
		}
		if r.CleanupTargetSize == 0 {
			r.CleanupTargetSize = ByteSize(float64(r.MaxSize) * 0.9)
		}
	}
	for i := range c.CommandRules {
		r := &c.CommandRules[i]
		if r.SuccessExitCodes == nil {
			r.SuccessExitCodes = []int{0}
		}
		if r.Timeout == 0 {
			r.Timeout = Duration(10 * time.Minute)
		}
	}
}

// Rules returns every rule in config order, dir rules first.
func (c *Config) Rules() []RuleRef {
	out := make([]RuleRef, 0, len(c.DirRules)+len(c.CommandRules))
	for _, r := range c.DirRules {
		out = append(out, RuleRef{Name: r.RuleName, Kind: "dir", Path: r.Path, MaxSize: r.MaxSize, AutoCleanup: r.AutoCleanup, Mode: r.CleanupMode, Unit: r.CleanupUnit})
	}
	for _, r := range c.CommandRules {
		out = append(out, RuleRef{Name: r.RuleName, Kind: "command", Path: r.Path, MaxSize: r.MaxSize, AutoCleanup: r.AutoCleanup})
	}
	return out
}
