//go:build darwin

package cli

import (
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
	"github.com/geraldcsoftware/disk-usage-cli/internal/policy"
	"github.com/geraldcsoftware/disk-usage-cli/internal/scan"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

// ruleMeasurement pairs a rule with its scan so report can show units.
type ruleMeasurement struct {
	Ref    config.RuleRef
	Result scan.Result
	Err    error
}

// measureRules scans every rule path. A path that cannot be scanned yields a
// status with no measurement time, so it reads as never measured and the next
// run tries again; the error travels in the measurement. It never aborts.
func measureRules(cfg *config.Config, home string, now time.Time, collectUnits bool) ([]state.RuleStatus, []state.Sample, []ruleMeasurement) {
	refs := cfg.Rules()
	statuses := make([]state.RuleStatus, 0, len(refs))
	samples := make([]state.Sample, 0, len(refs))
	measurements := make([]ruleMeasurement, 0, len(refs))
	for _, ref := range refs {
		unit := ref.Unit
		if unit == "" {
			unit = "top_level_dirs"
		}
		res, err := scan.Scan(ref.Path, scan.Options{Unit: unit, CollectUnits: collectUnits, Home: home})
		measurements = append(measurements, ruleMeasurement{Ref: ref, Result: res, Err: err})
		rs := state.RuleStatus{
			RuleName: ref.Name, Kind: ref.Kind, MaxBytes: int64(ref.MaxSize), AutoCleanup: ref.AutoCleanup,
		}
		if err == nil {
			measured := now
			rs.MeasuredAt = &measured
			rs.AllocatedBytes = res.Allocated
			rs.OverMax = policy.OverMax(res.Allocated, int64(ref.MaxSize))
			rs.Unmeasured = state.Unmeasured{PrivacyProtected: len(res.UnmeasuredRoots), CloudOnly: res.Skipped[scan.ClassCloudOnly]}
			samples = append(samples, state.Sample{TS: now, Rule: ref.Name, Allocated: res.Allocated, Apparent: res.Apparent, CloudOnly: res.CloudOnly, Units: res.FileCount})
		}
		statuses = append(statuses, rs)
	}
	return statuses, samples, measurements
}

// measurementDue reports whether rule directories need walking this run: the
// interval has elapsed since the oldest measurement, there is no previous
// measurement yet, or the configured rules have changed (added, removed or
// had their maximum size edited) since the previous run.
func measurementDue(prev *state.Status, refs []config.RuleRef, every time.Duration, now time.Time) bool {
	if prev == nil || len(prev.Rules) == 0 {
		return true
	}
	if len(prev.Rules) != len(refs) {
		return true
	}
	maxByName := make(map[string]int64, len(refs))
	for _, ref := range refs {
		maxByName[ref.Name] = int64(ref.MaxSize)
	}
	for _, r := range prev.Rules {
		if r.MeasuredAt == nil || !now.Before(r.MeasuredAt.Add(every)) {
			return true
		}
		max, ok := maxByName[r.RuleName]
		if !ok || max != r.MaxBytes {
			return true
		}
	}
	return false
}

// snapshotFrom lifts the previous disk state out of status.json.
func snapshotFrom(prev *state.Status) policy.Snapshot {
	if prev == nil {
		return policy.Snapshot{}
	}
	snap := policy.Snapshot{State: policy.State(prev.Disk.State), StateSince: prev.Disk.StateSince}
	if prev.Disk.LastNotified != nil {
		snap.LastNotified = *prev.Disk.LastNotified
	}
	return snap
}
