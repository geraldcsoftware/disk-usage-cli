//go:build darwin

package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

// statusCapacity is the pre-allocated size of status.json. The file is
// rewritten in place inside this allocation so a write never needs new
// blocks, which APFS refuses at its allocation floor.
const statusCapacity = 64 << 10

// ErrNoStatus means no check has run yet.
var ErrNoStatus = errors.New("no status yet: run dusk check")

// Status is the result of the last check, schema version 1.
type Status struct {
	Schema     int          `json:"schema"`
	TS         time.Time    `json:"ts"`
	StaleAfter time.Time    `json:"stale_after"`
	Disk       DiskStatus   `json:"disk"`
	Rules      []RuleStatus `json:"rules"`
	Summary    Summary      `json:"summary"`
	LastRun    LastRun      `json:"last_run"`
}

type DiskStatus struct {
	Volume          string     `json:"volume"`
	TotalBytes      int64      `json:"total_bytes"`
	UsableFreeBytes int64      `json:"usable_free_bytes"`
	FreePct         float64    `json:"free_pct"`
	State           string     `json:"state"`
	StateSince      time.Time  `json:"state_since"`
	LastNotified    *time.Time `json:"last_notified"`
	BallastReleased bool       `json:"ballast_released"`
}

type RuleStatus struct {
	RuleName       string     `json:"rule_name"`
	Kind           string     `json:"kind"`
	AllocatedBytes int64      `json:"allocated_bytes"`
	MaxBytes       int64      `json:"max_bytes"`
	OverMax        bool       `json:"over_max"`
	AutoCleanup    bool       `json:"auto_cleanup"`
	MeasuredAt     *time.Time `json:"measured_at"`
	Unmeasured     Unmeasured `json:"unmeasured"`
}

type Unmeasured struct {
	PrivacyProtected int `json:"privacy_protected"`
	CloudOnly        int `json:"cloud_only"`
}

type Summary struct {
	State           string `json:"state"`
	OverMaxCount    int    `json:"over_max_count"`
	UnmeasuredRoots int    `json:"unmeasured_roots"`
	JournalDegraded bool   `json:"journal_degraded"`
}

type LastRun struct {
	ID             string   `json:"id"`
	ReclaimedBytes int64    `json:"reclaimed_bytes"`
	RulesCleaned   []string `json:"rules_cleaned"`
}

// ReadStatus parses status.json, tolerating the space padding.
func (d Dir) ReadStatus() (*Status, error) {
	raw, err := os.ReadFile(d.Path("status.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoStatus
	}
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimRight(raw, " \n\x00")
	if len(raw) == 0 {
		return nil, ErrNoStatus
	}
	var s Status
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("status.json: %w", err)
	}
	return &s, nil
}

// WriteStatus rewrites status.json in place: the JSON followed by spaces up
// to the pre-allocated capacity, then fsync. Trailing whitespace is valid
// JSON, so readers need no framing.
func (d Dir) WriteStatus(s *Status) error {
	if s.Rules == nil {
		s.Rules = []RuleStatus{}
	}
	if s.LastRun.RulesCleaned == nil {
		s.LastRun.RulesCleaned = []string{}
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if len(data) > statusCapacity {
		return fmt.Errorf("status is %d bytes, over the %d byte allocation", len(data), statusCapacity)
	}
	f, err := os.OpenFile(d.Path("status.json"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() < statusCapacity {
		if err := sys.Preallocate(f, statusCapacity-fi.Size()); err != nil {
			return err
		}
	}
	buf := make([]byte, statusCapacity)
	copy(buf, data)
	for i := len(data); i < len(buf); i++ {
		buf[i] = ' '
	}
	if _, err := f.WriteAt(buf, 0); err != nil {
		return err
	}
	return f.Sync()
}
