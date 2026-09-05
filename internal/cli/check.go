//go:build darwin

package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
	"github.com/geraldcsoftware/disk-usage-cli/internal/disk"
	"github.com/geraldcsoftware/disk-usage-cli/internal/policy"
	"github.com/geraldcsoftware/disk-usage-cli/internal/report"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

// sampleRetention is how long samples.jsonl keeps history.
const sampleRetention = 180 * 24 * time.Hour

func newCheckCmd(s *session) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Measure free space, update the disk state and write status.json",
		Long:  "check is the scheduled entry point. Exit code 0 means ok, 1 warn, 2 critical and 3 unknown (statfs failed or the config is invalid).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := s.loadConfig(false)
			if err != nil {
				return exitWith(ExitUnknown, err)
			}
			dir, err := s.stateDir()
			if err != nil {
				return err
			}
			st, err := runCheck(s, cfg, dir, full)
			if err != nil {
				return err
			}
			if code := st.ExitCode(); code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "measure every rule directory now")
	return cmd
}

// runCheck performs one scheduled run: lock, read free space, evaluate the
// state machine, measure rules when due, write status and prompt, compact
// samples when healthy. The returned state decides the exit code.
func runCheck(s *session, cfg *config.Config, dir state.Dir, full bool) (policy.State, error) {
	release, err := dir.Lock()
	if errors.Is(err, state.ErrLocked) {
		return "", exitWith(ExitLocked, err)
	}
	if err != nil {
		return "", exitWith(ExitUnknown, err)
	}
	defer release()

	now := s.app.Now().UTC().Truncate(time.Second)
	prev, err := dir.ReadStatus()
	if err != nil && !errors.Is(err, state.ErrNoStatus) {
		fmt.Fprintln(s.app.Stderr, "warning: previous status unreadable, starting fresh:", err)
		prev = nil
	}

	usage, err := disk.Read(cfg.Disk.VolumePath)
	if err != nil {
		return "", exitWith(ExitUnknown, err)
	}
	bands := policy.Bands{Warn: cfg.Disk.Warn, Critical: cfg.Disk.Critical}
	snap := policy.Evaluate(snapshotFrom(prev), usage.UsableFreeBytes, usage.TotalBytes, bands, now)

	var rules []state.RuleStatus
	samples := []state.Sample{}
	if full || snap.State == policy.Critical || measurementDue(prev, cfg.Rules(), cfg.Schedule.MeasureRuleDirsEvery.Std(), now) {
		rules, samples, _ = measureRules(cfg, s.home, now, false)
	} else {
		rules = prev.Rules
	}
	samples = append(samples, state.Sample{TS: now, Rule: state.DiskSampleRule, UsableFree: usage.UsableFreeBytes, Total: usage.TotalBytes})
	if err := dir.AppendSamples(samples); err != nil {
		fmt.Fprintln(s.app.Stderr, "warning: samples not recorded:", err)
	}

	overMax, unmeasuredRoots := 0, 0
	for _, r := range rules {
		if r.OverMax {
			overMax++
		}
		if r.Unmeasured.PrivacyProtected > 0 {
			unmeasuredRoots++
		}
	}
	status := &state.Status{
		Schema:     1,
		TS:         now,
		StaleAfter: now.Add(cfg.Prompt.StaleAfter.Std()),
		Disk: state.DiskStatus{
			Volume: usage.Volume, TotalBytes: usage.TotalBytes, UsableFreeBytes: usage.UsableFreeBytes, FreePct: usage.FreePct(),
			State: string(snap.State), StateSince: snap.StateSince,
		},
		Rules:   rules,
		Summary: state.Summary{State: string(snap.State), OverMaxCount: overMax, UnmeasuredRoots: unmeasuredRoots},
		LastRun: state.LastRun{ID: state.NewULID(now), RulesCleaned: []string{}},
	}
	if !snap.LastNotified.IsZero() {
		ln := snap.LastNotified
		status.Disk.LastNotified = &ln
	}
	if err := dir.WriteStatus(status); err != nil {
		fmt.Fprintln(s.app.Stderr, "warning: status.json not written:", err)
	}
	if err := dir.WritePrompt(report.RenderPrompt(status, cfg.Prompt, now)); err != nil {
		fmt.Fprintln(s.app.Stderr, "warning: prompt not written, previous prompt kept:", err)
	}
	if snap.State == policy.OK {
		if err := dir.CompactSamples(now.Add(-sampleRetention)); err != nil {
			fmt.Fprintln(s.app.Stderr, "warning: samples not compacted:", err)
		}
	}
	return snap.State, nil
}
