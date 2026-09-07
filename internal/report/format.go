//go:build darwin

// Package report renders dusk's data for terminals and for the Starship
// prompt. JSON output is produced by the callers with encoding/json.
package report

import (
	"fmt"
	"time"
)

// Format holds the presentation switches shared by every renderer.
type Format struct {
	// IEC prints binary units (GiB) instead of the decimal units Finder and
	// Disk Utility use.
	IEC bool
}

// Bytes formats a byte count in the largest unit that keeps the value at or
// above one, with one decimal for gigabytes and above.
func (f Format) Bytes(n int64) string {
	base, suffixes := 1000.0, []string{"B", "KB", "MB", "GB", "TB"}
	if f.IEC {
		base, suffixes = 1024.0, []string{"B", "KiB", "MiB", "GiB", "TiB"}
	}
	v := float64(n)
	i := 0
	for v >= base && i < len(suffixes)-1 {
		v /= base
		i++
	}
	if i >= 3 {
		return fmt.Sprintf("%.1f %s", v, suffixes[i])
	}
	return fmt.Sprintf("%.0f %s", v, suffixes[i])
}

// Signed formats a growth figure with its sign.
func (f Format) Signed(n int64) string {
	if n < 0 {
		return "-" + f.Bytes(-n)
	}
	return "+" + f.Bytes(n)
}

// ColorEnabled applies the precedence flag, then NO_COLOR, then terminal
// detection.
func ColorEnabled(flag string, getenv func(string) string, isTerminal bool) (bool, error) {
	switch flag {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "auto", "":
		if getenv("NO_COLOR") != "" {
			return false, nil
		}
		return isTerminal, nil
	}
	return false, fmt.Errorf("--color must be always, never or auto, not %q", flag)
}

// Palette colours state words when enabled.
type Palette struct {
	Enabled bool
}

// State wraps warn in yellow and critical in red.
func (p Palette) State(s string) string {
	if !p.Enabled {
		return s
	}
	switch s {
	case "warn":
		return "\x1b[33m" + s + "\x1b[0m"
	case "critical":
		return "\x1b[31m" + s + "\x1b[0m"
	}
	return s
}

// Age renders a duration as days and hours, or hours and minutes under a day.
func Age(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
