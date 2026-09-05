//go:build darwin

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

// Validator supplies the environment validation consults, so tests can point
// it at a fabricated home directory.
type Validator struct {
	Home                string
	PATH                string
	Lstat               func(string) (os.FileInfo, error)
	TimeMachineIncluded func(string) (bool, error)
}

// DefaultValidator uses the real filesystem, dusk's fixed PATH and tmutil.
func DefaultValidator(home string) Validator {
	return Validator{Home: home, PATH: FixedPATH(home), Lstat: os.Lstat, TimeMachineIncluded: timeMachineIncluded}
}

// FixedPATH is the search path used for command rules, independent of the
// caller's environment.
func FixedPATH(home string) string {
	return "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + home + "/.local/bin:" + home + "/go/bin"
}

// LookPath resolves name on pathList the way exec.LookPath resolves on $PATH.
func LookPath(name, pathList string) (string, error) {
	if strings.Contains(name, "/") {
		if fi, err := os.Stat(name); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return name, nil
		}
		return "", fmt.Errorf("%s: not an executable file", name)
	}
	for _, dir := range strings.Split(pathList, ":") {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: not found on %s", name, pathList)
}

// timeMachineIncluded asks tmutil whether a path is part of backups.
func timeMachineIncluded(path string) (bool, error) {
	out, err := exec.Command("/usr/bin/tmutil", "isexcluded", path).Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), "[Included]"), nil
}

// AllowedCheckEvery lists the schedule.check_every values that expand to a
// clean launchd calendar.
var AllowedCheckEvery = []string{"5m", "10m", "15m", "20m", "30m", "1h", "2h", "3h", "4h", "6h", "8h", "12h", "24h"}

var (
	ruleNamePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	quietHoursPattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d-([01]\d|2[0-3]):[0-5]\d$`)
	commandBlockList  = map[string]bool{"sh": true, "bash": true, "zsh": true, "fish": true, "env": true, "sudo": true, "rm": true, "rmdir": true, "unlink": true}
	cleanupModes      = map[string]bool{"oldest_first": true, "largest_first": true, "delete_all": true}
	cleanupUnits      = map[string]bool{"files": true, "top_level_dirs": true}
	userDataRoots     = []string{"Desktop", "Documents", "Downloads"}
	deniedRelative    = []string{"Library/Mobile Documents", "Library/Keychains", "Library/Mail", "Library/Messages", "Library/Application Support/MobileSync"}
	neverRuleRelative = []string{"", "Library", "Library/Application Support", "Library/Containers"}
)

// Validate applies every rule from the specification. Errors are collected
// rather than returned one at a time. Warnings never fail validation.
func (c *Config) Validate(v Validator) ([]string, error) {
	var errs, warnings []string
	fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	if !checkEveryAllowed(c.Schedule.CheckEvery) {
		fail("schedule.check_every must be one of %s", strings.Join(AllowedCheckEvery, " "))
	}
	for name, list := range map[string]Thresholds{
		"disk.warn.when_free_below": c.Disk.Warn.WhenFreeBelow, "disk.warn.clear_when_above": c.Disk.Warn.ClearWhenAbove,
		"disk.critical.when_free_below": c.Disk.Critical.WhenFreeBelow, "disk.critical.clear_when_above": c.Disk.Critical.ClearWhenAbove,
	} {
		if len(list) == 0 {
			fail("%s needs at least one entry", name)
		}
	}
	for _, q := range c.Disk.QuietHours {
		if !quietHoursPattern.MatchString(q) {
			fail("disk.quiet_hours entry %q must look like 23:00-07:00", q)
		}
	}

	seen := map[string]bool{}
	checkName := func(name string) {
		if !ruleNamePattern.MatchString(name) {
			fail("rule_name %q must match [a-z0-9][a-z0-9-]*", name)
		}
		if seen[name] {
			fail("rule_name %q is not unique", name)
		}
		seen[name] = true
	}

	for i := range c.DirRules {
		r := &c.DirRules[i]
		checkName(r.RuleName)
		if r.MaxSize <= 0 {
			fail("dir rule %q: max_size must be positive", r.RuleName)
		}
		if r.CleanupTargetSize >= r.MaxSize {
			fail("dir rule %q: cleanup_target_size must be below max_size", r.RuleName)
		}
		if !cleanupModes[r.CleanupMode] {
			fail("dir rule %q: cleanup_mode must be oldest_first, largest_first or delete_all", r.RuleName)
		}
		if !cleanupUnits[r.CleanupUnit] {
			fail("dir rule %q: cleanup_unit must be files or top_level_dirs", r.RuleName)
		}
		for _, p := range r.ExcludePatterns {
			if !doublestar.ValidatePattern(p) {
				fail("dir rule %q: exclude_patterns entry %q is not a valid pattern", r.RuleName, p)
			}
		}
		c.validateDirPath(r, v, fail, warn)
	}

	for i := range c.CommandRules {
		r := &c.CommandRules[i]
		checkName(r.RuleName)
		if r.MaxSize <= 0 {
			fail("command rule %q: max_size must be positive", r.RuleName)
		}
		if len(r.CleanupCommand) == 0 {
			fail("command rule %q: cleanup_command is required", r.RuleName)
		}
		for field, argv := range map[string][]string{"cleanup_command": r.CleanupCommand, "preview_command": r.PreviewCommand} {
			if len(argv) == 0 {
				continue
			}
			if commandBlockList[filepath.Base(argv[0])] {
				fail("command rule %q: %s may not start with %q; file deletion belongs in dir_rules", r.RuleName, field, argv[0])
				continue
			}
			if _, err := LookPath(argv[0], v.PATH); err != nil {
				warn("command rule %q: %s binary %q not found on dusk's PATH", r.RuleName, field, argv[0])
			}
		}
		if _, err := v.Lstat(r.Path); errors.Is(err, os.ErrNotExist) {
			warn("command rule %q: path %s does not exist yet", r.RuleName, r.Path)
		} else if err == nil {
			if included, terr := v.TimeMachineIncluded(filepath.Clean(r.Path)); terr == nil && included {
				warn("command rule %q: path %s is included in Time Machine backups", r.RuleName, r.Path)
			}
		}
	}

	if len(errs) > 0 {
		return warnings, &ValidationError{Errors: errs}
	}
	return warnings, nil
}

func checkEveryAllowed(d Duration) bool {
	for _, a := range AllowedCheckEvery {
		if allowed, _ := ParseDuration(a); allowed == d {
			return true
		}
	}
	return false
}

// validateDirPath applies rules 1 and 2 and the existence and Time Machine
// warnings to one dir rule.
func (c *Config) validateDirPath(r *DirRule, v Validator, fail, warn func(string, ...any)) {
	clean := filepath.Clean(r.Path)
	resolved := clean
	if parent, err := filepath.EvalSymlinks(filepath.Dir(clean)); err == nil {
		resolved = filepath.Join(parent, filepath.Base(clean))
	}
	if _, err := v.Lstat(resolved); errors.Is(err, os.ErrNotExist) {
		warn("dir rule %q: path %s does not exist yet", r.RuleName, r.Path)
	} else if err == nil {
		if included, terr := v.TimeMachineIncluded(resolved); terr == nil && included {
			warn("dir rule %q: path %s is included in Time Machine backups", r.RuleName, r.Path)
		}
	}

	home := filepath.Clean(v.Home)
	if resolvedHome, err := filepath.EvalSymlinks(home); err == nil {
		home = resolvedHome
	}
	for _, rel := range neverRuleRelative {
		if resolved == filepath.Join(home, rel) {
			fail("dir rule %q: path %s can never be a rule path", r.RuleName, r.Path)
			return
		}
	}
	if resolved == "/" {
		fail("dir rule %q: path / can never be a rule path", r.RuleName)
		return
	}
	if fs, err := sys.Statfs(resolved); err == nil && fs.MountPoint == resolved {
		fail("dir rule %q: path %s is a volume root and can never be a rule path", r.RuleName, r.Path)
		return
	}
	inside := strings.HasPrefix(resolved, home+string(filepath.Separator))
	if !inside && !c.Safety.AllowPathsOutsideHome {
		fail("dir rule %q: path %s is outside the home directory; set safety.allow_paths_outside_home to permit it", r.RuleName, r.Path)
		return
	}
	if !inside {
		return
	}
	rel, _ := filepath.Rel(home, resolved)
	for _, d := range deniedRelative {
		if rel == d || strings.HasPrefix(rel, d+string(filepath.Separator)) {
			fail("dir rule %q: path %s is inside the denied location ~/%s", r.RuleName, r.Path, d)
			return
		}
	}
	if strings.HasPrefix(rel, "Library/CloudStorage"+string(filepath.Separator)) {
		fail("dir rule %q: path %s is inside a File Provider root under ~/Library/CloudStorage and is denied", r.RuleName, r.Path)
		return
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasSuffix(seg, ".photoslibrary") {
			fail("dir rule %q: path %s is inside a Photos library and is denied", r.RuleName, r.Path)
			return
		}
	}
	for _, u := range userDataRoots {
		if rel == u || strings.HasPrefix(rel, u+string(filepath.Separator)) {
			if !r.ContainsUserData {
				fail("dir rule %q: path %s is under ~/%s and requires contains_user_data = true", r.RuleName, r.Path, u)
				return
			}
			r.AutoCleanup = false
		}
	}
}

// CalendarEntry is one launchd StartCalendarInterval dictionary. Hour -1
// means every hour.
type CalendarEntry struct {
	Hour   int
	Minute int
}

// CalendarEntries expands check_every into calendar entries offset five
// minutes past the hour, so dusk never coincides with top of the hour jobs.
func CalendarEntries(d Duration) ([]CalendarEntry, error) {
	if !checkEveryAllowed(d) {
		return nil, fmt.Errorf("check_every %s is not one of %s", d.Std(), strings.Join(AllowedCheckEvery, " "))
	}
	std := d.Std()
	var out []CalendarEntry
	if std < time.Hour {
		for m := 5; m < 60; m += int(std / time.Minute) {
			out = append(out, CalendarEntry{Hour: -1, Minute: m})
		}
		return out, nil
	}
	for h := 0; h < 24; h += int(std / time.Hour) {
		out = append(out, CalendarEntry{Hour: h, Minute: 5})
	}
	return out, nil
}
