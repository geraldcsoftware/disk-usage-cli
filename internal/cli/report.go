//go:build darwin

package cli

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/geraldcsoftware/disk-usage-cli/internal/disk"
	"github.com/geraldcsoftware/disk-usage-cli/internal/policy"
	"github.com/geraldcsoftware/disk-usage-cli/internal/report"
	"github.com/geraldcsoftware/disk-usage-cli/internal/scan"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

const largestPerRule = 5

// reportJSON is the --json shape of dusk report.
type reportJSON struct {
	TS    time.Time        `json:"ts"`
	Disk  state.DiskStatus `json:"disk"`
	Rules []reportRuleJSON `json:"rules"`
}

type reportRuleJSON struct {
	Name            string              `json:"name"`
	Kind            string              `json:"kind"`
	Path            string              `json:"path"`
	AllocatedBytes  int64               `json:"allocated_bytes"`
	ApparentBytes   int64               `json:"apparent_bytes"`
	CloudOnlyBytes  int64               `json:"cloud_only_bytes"`
	MaxBytes        int64               `json:"max_bytes"`
	OverMax         bool                `json:"over_max"`
	Growth7d        *int64              `json:"growth_7d_bytes"`
	Growth30d       *int64              `json:"growth_30d_bytes"`
	Largest         []report.ReportUnit `json:"largest"`
	Skipped         map[string]int      `json:"skipped"`
	UnmeasuredRoots []string            `json:"unmeasured_roots"`
	Err             string              `json:"error,omitempty"`
}

func newReportCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Measure now and print every rule against its maximum with growth",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := s.loadConfig()
			if err != nil {
				return err
			}
			dir, err := s.stateDir()
			if err != nil {
				return err
			}
			now := s.app.Now().UTC().Truncate(time.Second)
			usage, err := disk.Read(cfg.Disk.VolumePath)
			if err != nil {
				return exitWith(ExitUnknown, err)
			}
			prev, _ := dir.ReadStatus()
			bands := policy.Bands{Warn: cfg.Disk.Warn, Critical: cfg.Disk.Critical}
			snap := policy.Evaluate(snapshotFrom(prev), usage.UsableFreeBytes, usage.TotalBytes, bands, now)
			samples, _ := dir.ReadSamples()

			_, _, measurements := measureRules(cfg, s.home, now, true)
			rep := report.Report{TS: now, Disk: state.DiskStatus{
				Volume: usage.Volume, TotalBytes: usage.TotalBytes, UsableFreeBytes: usage.UsableFreeBytes, FreePct: usage.FreePct(),
				State: string(snap.State), StateSince: snap.StateSince,
			}}
			out := reportJSON{TS: now, Disk: rep.Disk}
			for _, m := range measurements {
				rr := report.ReportRule{Name: m.Ref.Name, Kind: m.Ref.Kind, Path: m.Ref.Path, Max: int64(m.Ref.MaxSize)}
				if m.Err != nil {
					rr.Err = m.Err.Error()
				} else {
					rr.Allocated, rr.Apparent, rr.CloudOnly = m.Result.Allocated, m.Result.Apparent, m.Result.CloudOnly
					rr.OverMax = policy.OverMax(m.Result.Allocated, int64(m.Ref.MaxSize))
					rr.Growth7d = growthSince(samples, m.Ref.Name, m.Result.Allocated, now, 7*24*time.Hour)
					rr.Growth30d = growthSince(samples, m.Ref.Name, m.Result.Allocated, now, 30*24*time.Hour)
					rr.Largest = largestUnits(m.Result.Units, largestPerRule)
					rr.Skipped = map[string]int{}
					for class, n := range m.Result.Skipped {
						rr.Skipped[string(class)] = n
					}
					rr.UnmeasuredRoots = m.Result.UnmeasuredRoots
				}
				rep.Rules = append(rep.Rules, rr)
				out.Rules = append(out.Rules, reportRuleJSON{
					Name: rr.Name, Kind: rr.Kind, Path: rr.Path, AllocatedBytes: rr.Allocated, ApparentBytes: rr.Apparent, CloudOnlyBytes: rr.CloudOnly,
					MaxBytes: rr.Max, OverMax: rr.OverMax, Growth7d: rr.Growth7d, Growth30d: rr.Growth30d, Largest: rr.Largest,
					Skipped: rr.Skipped, UnmeasuredRoots: rr.UnmeasuredRoots, Err: rr.Err,
				})
			}
			if s.json {
				enc := json.NewEncoder(s.app.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			report.WriteReport(s.app.Stdout, rep, s.format, s.palette)
			return nil
		},
	}
}

// growthSince compares the current size with the earliest sample of the rule
// inside the window. nil means no history reaches that far back.
func growthSince(samples []state.Sample, rule string, current int64, now time.Time, window time.Duration) *int64 {
	cutoff := now.Add(-window)
	var base *state.Sample
	for i := range samples {
		s := &samples[i]
		if s.Rule != rule || s.TS.Before(cutoff) || s.TS.After(now) {
			continue
		}
		if base == nil || s.TS.Before(base.TS) {
			base = s
		}
	}
	if base == nil {
		return nil
	}
	delta := current - base.Allocated
	return &delta
}

// largestUnits returns the n largest units by allocated size.
func largestUnits(units []scan.Unit, n int) []report.ReportUnit {
	sorted := make([]scan.Unit, len(units))
	copy(sorted, units)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Allocated > sorted[j].Allocated })
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	out := make([]report.ReportUnit, len(sorted))
	for i, u := range sorted {
		out[i] = report.ReportUnit{RelPath: u.RelPath, Allocated: u.Allocated, ModTime: u.ModTime, Freeable: u.Freeable}
	}
	return out
}
