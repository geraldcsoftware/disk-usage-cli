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
// status with zero size and the error in the measurement; it never aborts.
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
		measured := now
		rs := state.RuleStatus{
			RuleName: ref.Name, Kind: ref.Kind, MaxBytes: int64(ref.MaxSize), AutoCleanup: ref.AutoCleanup, MeasuredAt: &measured,
		}
		if err == nil {
			rs.AllocatedBytes = res.Allocated
			rs.OverMax = policy.OverMax(res.Allocated, int64(ref.MaxSize))
			rs.Unmeasured = state.Unmeasured{PrivacyProtected: res.Skipped[scan.ClassPrivacyProtected], CloudOnly: res.Skipped[scan.ClassCloudOnly]}
			samples = append(samples, state.Sample{TS: now, Rule: ref.Name, Allocated: res.Allocated, Apparent: res.Apparent, CloudOnly: res.CloudOnly, Units: res.FileCount})
		}
		statuses = append(statuses, rs)
	}
	return statuses, samples, measurements
}

// measurementDue reports whether rule directories need walking this run.
func measurementDue(prev *state.Status, every time.Duration, now time.Time) bool {
	if prev == nil || len(prev.Rules) == 0 {
		return true
	}
	for _, r := range prev.Rules {
		if r.MeasuredAt == nil || !now.Before(r.MeasuredAt.Add(every)) {
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
