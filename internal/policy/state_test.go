//go:build darwin

package policy

import (
	"testing"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
)

func bands() Bands {
	cfg := config.Default()
	return Bands{Warn: cfg.Disk.Warn, Critical: cfg.Disk.Critical}
}

const total = 200_000_000_000 // 200 GB: warn under 30 GB, clear over 40 GB; critical under 16 GB, clear over 24 GB

func gb(n float64) int64 { return int64(n * 1e9) }

func TestNextTransitions(t *testing.T) {
	cases := []struct {
		name string
		prev State
		free int64
		want State
	}{
		{"ok stays ok", OK, gb(50), OK},
		{"ok enters warn on percent", OK, gb(29), Warn},
		{"ok enters warn on absolute", OK, gb(24), Warn},
		{"ok jumps to critical", OK, gb(15), Critical},
		{"warn holds in hysteresis band", Warn, gb(35), Warn},
		{"warn clears above every entry", Warn, gb(41), OK},
		{"warn does not clear above only one entry", Warn, gb(31), Warn},
		{"warn worsens", Warn, gb(15), Critical},
		{"critical holds in its band", Critical, gb(20), Critical},
		{"critical drops to warn", Critical, gb(25), Warn},
		{"critical clears straight to ok", Critical, gb(45), OK},
	}
	for _, c := range cases {
		if got := Next(c.prev, c.free, total, bands()); got != c.want {
			t.Errorf("%s: Next(%s, %d) = %s, want %s", c.name, c.prev, c.free, got, c.want)
		}
	}
}

func TestEvaluateTracksStateSince(t *testing.T) {
	t0 := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	first := Evaluate(Snapshot{}, gb(50), total, bands(), t0)
	if first.State != OK || !first.StateSince.Equal(t0) {
		t.Errorf("first = %+v", first)
	}
	same := Evaluate(first, gb(48), total, bands(), t0.Add(time.Hour))
	if !same.StateSince.Equal(t0) {
		t.Errorf("unchanged state must keep state_since: %+v", same)
	}
	worse := Evaluate(same, gb(20), total, bands(), t0.Add(2*time.Hour))
	if worse.State != Warn || !worse.StateSince.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("transition must reset state_since: %+v", worse)
	}
	if !worse.LastNotified.IsZero() {
		t.Error("Evaluate never sets LastNotified; the notifier does")
	}
}

func TestNoticeFor(t *testing.T) {
	t0 := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	renotify := 4 * time.Hour
	ok := Snapshot{State: OK, StateSince: t0}
	warn := Snapshot{State: Warn, StateSince: t0}
	crit := Snapshot{State: Critical, StateSince: t0}
	if NoticeFor(ok, warn, renotify, t0) != NoticeWorsened {
		t.Error("ok to warn is a worsening")
	}
	if NoticeFor(warn, crit, renotify, t0) != NoticeWorsened {
		t.Error("warn to critical is a worsening")
	}
	if NoticeFor(warn, ok, renotify, t0) != NoticeRecovered {
		t.Error("warn to ok is a recovery")
	}
	if NoticeFor(crit, warn, renotify, t0) != NoticeNone {
		t.Error("critical to warn is an improvement that is not ok; the standing banner stays")
	}
	if NoticeFor(ok, ok, renotify, t0) != NoticeNone {
		t.Error("ok to ok is silent")
	}
	notified := Snapshot{State: Warn, StateSince: t0, LastNotified: t0}
	if NoticeFor(notified, Snapshot{State: Warn, StateSince: t0}, renotify, t0.Add(3*time.Hour)) != NoticeNone {
		t.Error("three hours after the last banner is too early to repeat")
	}
	if NoticeFor(notified, Snapshot{State: Warn, StateSince: t0}, renotify, t0.Add(4*time.Hour)) != NoticeRepeat {
		t.Error("renotify_every elapsed in warn must repeat")
	}
	never := Snapshot{State: Warn, StateSince: t0}
	if NoticeFor(never, Snapshot{State: Warn, StateSince: t0}, renotify, t0.Add(9*time.Hour)) != NoticeRepeat {
		t.Error("a warn state that was never notified (quiet hours) must notify once the window ends")
	}
}

func TestQuietHours(t *testing.T) {
	ws, err := ParseQuietHours([]string{"23:00-07:00", "12:30-13:00"})
	if err != nil {
		t.Fatal(err)
	}
	at := func(h, m int) time.Time { return time.Date(2026, 9, 5, h, m, 0, 0, time.Local) }
	for _, c := range []struct {
		t    time.Time
		want bool
	}{{at(23, 30), true}, {at(2, 0), true}, {at(6, 59), true}, {at(7, 0), false}, {at(12, 45), true}, {at(13, 0), false}, {at(15, 0), false}} {
		if got := InQuietHours(ws, c.t); got != c.want {
			t.Errorf("InQuietHours(%v) = %v, want %v", c.t.Format("15:04"), got, c.want)
		}
	}
	if _, err := ParseQuietHours([]string{"night"}); err == nil {
		t.Error("malformed window must error")
	}
	if InQuietHours(nil, at(3, 0)) {
		t.Error("no windows means never quiet")
	}
}

func TestExitCodesAndOverMax(t *testing.T) {
	if OK.ExitCode() != 0 || Warn.ExitCode() != 1 || Critical.ExitCode() != 2 {
		t.Error("exit codes must follow the monitoring convention")
	}
	if !OverMax(6_000_000_001, 6_000_000_000) || OverMax(6_000_000_000, 6_000_000_000) {
		t.Error("over max is strictly greater than the maximum")
	}
}
