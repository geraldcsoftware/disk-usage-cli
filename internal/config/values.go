//go:build darwin

package config

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ByteSize is a size in bytes parsed from the config grammar: decimal units
// (KB, MB, GB, TB) or binary units (KiB, MiB, GiB, TiB), case insensitive.
// A bare single letter unit such as 10G is rejected as ambiguous.
type ByteSize int64

var unitMultipliers = map[string]int64{
	"": 1, "b": 1,
	"kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12,
	"kib": 1 << 10, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40,
}

// ParseByteSize parses "6GB", "5GiB", "128MB" or a bare number of bytes.
func ParseByteSize(s string) (ByteSize, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, errors.New("size is empty")
	}
	i := 0
	for i < len(t) && (t[i] >= '0' && t[i] <= '9' || t[i] == '.') {
		i++
	}
	num, unit := t[:i], strings.ToLower(strings.TrimSpace(t[i:]))
	if num == "" {
		return 0, fmt.Errorf("size %q has no number", s)
	}
	if len(unit) == 1 && unit != "b" {
		u := strings.ToUpper(unit)
		return 0, fmt.Errorf("size %q is ambiguous: use %sB for decimal or %siB for binary", s, u, u)
	}
	mult, ok := unitMultipliers[unit]
	if !ok {
		return 0, fmt.Errorf("size %q has unknown unit %q", s, unit)
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q: %w", s, err)
	}
	v := f * float64(mult)
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("size %q is out of range", s)
	}
	return ByteSize(math.Round(v)), nil
}

func (b *ByteSize) UnmarshalText(text []byte) error {
	v, err := ParseByteSize(string(text))
	if err != nil {
		return err
	}
	*b = v
	return nil
}

func (b ByteSize) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(b), 10) + "B"), nil
}

// Duration is a time.Duration parsed from Go syntax extended with a d suffix
// for days ("30m", "6h", "7d").
type Duration time.Duration

// ParseDuration accepts Go durations and a whole or fractional day count.
func ParseDuration(s string) (Duration, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, errors.New("duration is empty")
	}
	if strings.HasSuffix(t, "d") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(t, "d"), 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("duration %q: expected a day count such as 7d", s)
		}
		return Duration(time.Duration(n * float64(24*time.Hour))), nil
	}
	d, err := time.ParseDuration(t)
	if err != nil {
		return 0, fmt.Errorf("duration %q: %w", s, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("duration %q is negative", s)
	}
	return Duration(d), nil
}

// Std returns the value as a time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// Threshold is either a percentage of the container size or an absolute
// usable free size.
type Threshold struct {
	Percent   float64
	Bytes     ByteSize
	IsPercent bool
}

// ParseThreshold parses "15%" or an absolute size such as "25GB".
func ParseThreshold(s string) (Threshold, error) {
	t := strings.TrimSpace(s)
	if strings.HasSuffix(t, "%") {
		p, err := strconv.ParseFloat(strings.TrimSuffix(t, "%"), 64)
		if err != nil || p <= 0 || p >= 100 {
			return Threshold{}, fmt.Errorf("threshold %q: percentage must be between 0 and 100 exclusive", s)
		}
		return Threshold{Percent: p, IsPercent: true}, nil
	}
	b, err := ParseByteSize(t)
	if err != nil {
		return Threshold{}, fmt.Errorf("threshold %q: %w", s, err)
	}
	if b <= 0 {
		return Threshold{}, fmt.Errorf("threshold %q must be positive", s)
	}
	return Threshold{Bytes: b}, nil
}

func (t *Threshold) UnmarshalText(text []byte) error {
	v, err := ParseThreshold(string(text))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

func (t Threshold) MarshalText() ([]byte, error) {
	if t.IsPercent {
		return []byte(strconv.FormatFloat(t.Percent, 'f', -1, 64) + "%"), nil
	}
	return t.Bytes.MarshalText()
}

func (t Threshold) limit(total int64) int64 {
	if t.IsPercent {
		return int64(float64(total) * t.Percent / 100)
	}
	return int64(t.Bytes)
}

// Below reports whether usable free space is under the threshold.
func (t Threshold) Below(free, total int64) bool { return free < t.limit(total) }

// Above reports whether usable free space is over the threshold.
func (t Threshold) Above(free, total int64) bool { return free > t.limit(total) }

// Thresholds is a list where any entry triggers entry into a state and every
// entry must be satisfied to leave it.
type Thresholds []Threshold

// AnyBelow reports whether free space is below at least one entry.
func (ts Thresholds) AnyBelow(free, total int64) bool {
	for _, t := range ts {
		if t.Below(free, total) {
			return true
		}
	}
	return false
}

// AllAbove reports whether free space is above every entry.
func (ts Thresholds) AllAbove(free, total int64) bool {
	for _, t := range ts {
		if !t.Above(free, total) {
			return false
		}
	}
	return true
}
