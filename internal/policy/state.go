//go:build darwin

// Package policy holds the decisions dusk makes from measurements: the disk
// state machine, when to notify, and whether a rule is over its maximum.
package policy

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
)

// State is the disk state with hysteresis.
type State string

const (
	OK       State = "ok"
	Warn     State = "warn"
	Critical State = "critical"
)

// ExitCode follows the monitoring convention used by dusk check.
func (s State) ExitCode() int {
	switch s {
	case Warn:
		return 1
	case Critical:
		return 2
	}
	return 0
}

// Rank orders states from healthy to severe.
func (s State) Rank() int { return s.ExitCode() }

// Bands are the warn and critical thresholds from the config.
type Bands struct {
	Warn     config.Band
	Critical config.Band
}

// Next applies the hysteresis rules. Entry happens when usable free space is
// below any when_free_below entry; leaving needs every clear_when_above entry
// to be satisfied. Critical is evaluated before warn.
func Next(prev State, free, total int64, b Bands) State {
	switch prev {
	case Critical:
		if !b.Critical.ClearWhenAbove.AllAbove(free, total) {
			return Critical
		}
		if b.Warn.ClearWhenAbove.AllAbove(free, total) {
			return OK
		}
		return Warn
	case Warn:
		if b.Critical.WhenFreeBelow.AnyBelow(free, total) {
			return Critical
		}
		if b.Warn.ClearWhenAbove.AllAbove(free, total) {
			return OK
		}
		return Warn
	default:
		if b.Critical.WhenFreeBelow.AnyBelow(free, total) {
			return Critical
		}
		if b.Warn.WhenFreeBelow.AnyBelow(free, total) {
			return Warn
		}
		return OK
	}
}

// Snapshot is the disk part of the previous status a check compares against.
type Snapshot struct {
	State        State
	StateSince   time.Time
	LastNotified time.Time
}

// Evaluate produces the next snapshot. StateSince is kept while the state is
// unchanged and reset on a transition. LastNotified is carried over untouched;
// the notifier updates it.
func Evaluate(prev Snapshot, free, total int64, b Bands, now time.Time) Snapshot {
	if prev.State == "" {
		prev.State = OK
	}
	next := Snapshot{State: Next(prev.State, free, total, b), StateSince: prev.StateSince, LastNotified: prev.LastNotified}
	if next.State != prev.State || prev.StateSince.IsZero() {
		next.StateSince = now
	}
	return next
}

// Notice says whether a check should raise a banner.
type Notice int

const (
	NoticeNone      Notice = iota
	NoticeWorsened         // entered warn or critical from a better state
	NoticeRecovered        // returned to ok
	NoticeRepeat           // still warn or critical and renotify_every has elapsed
)

// NoticeFor decides the notification for a transition from prev to next.
// Repeats are measured from the last banner, or from state_since when no
// banner has been shown yet.
func NoticeFor(prev, next Snapshot, renotify time.Duration, now time.Time) Notice {
	if prev.State == "" {
		prev.State = OK
	}
	switch {
	case next.State.Rank() > prev.State.Rank():
		return NoticeWorsened
	case next.State == OK && prev.State != OK:
		return NoticeRecovered
	case next.State == OK:
		return NoticeNone
	case next.State.Rank() < prev.State.Rank():
		return NoticeNone
	}
	since := prev.LastNotified
	if since.IsZero() {
		since = next.StateSince
	}
	if renotify > 0 && !now.Before(since.Add(renotify)) {
		return NoticeRepeat
	}
	return NoticeNone
}

// Window is a daily quiet period in minutes since midnight. Start greater
// than End means the window wraps past midnight.
type Window struct {
	Start int
	End   int
}

var windowPattern = regexp.MustCompile(`^(\d{2}):(\d{2})-(\d{2}):(\d{2})$`)

// ParseQuietHours parses entries such as "23:00-07:00".
func ParseQuietHours(entries []string) ([]Window, error) {
	out := make([]Window, 0, len(entries))
	for _, e := range entries {
		m := windowPattern.FindStringSubmatch(e)
		if m == nil {
			return nil, fmt.Errorf("quiet hours entry %q must look like 23:00-07:00", e)
		}
		sh, _ := strconv.Atoi(m[1])
		sm, _ := strconv.Atoi(m[2])
		eh, _ := strconv.Atoi(m[3])
		em, _ := strconv.Atoi(m[4])
		if sh > 23 || eh > 23 || sm > 59 || em > 59 {
			return nil, fmt.Errorf("quiet hours entry %q has an out of range time", e)
		}
		out = append(out, Window{Start: sh*60 + sm, End: eh*60 + em})
	}
	return out, nil
}

// InQuietHours reports whether t falls inside any window. The end minute is
// exclusive.
func InQuietHours(ws []Window, t time.Time) bool {
	m := t.Hour()*60 + t.Minute()
	for _, w := range ws {
		if w.Start <= w.End {
			if m >= w.Start && m < w.End {
				return true
			}
		} else if m >= w.Start || m < w.End {
			return true
		}
	}
	return false
}

// OverMax reports whether a rule's allocated size exceeds its maximum.
func OverMax(allocated, max int64) bool { return allocated > max }
