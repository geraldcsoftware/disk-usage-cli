//go:build darwin

package cli

import (
	"encoding/json"
	"errors"

	"github.com/spf13/cobra"

	"github.com/geraldcsoftware/disk-usage-cli/internal/report"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

// ruleJSON is the --json shape of dusk rules.
type ruleJSON struct {
	RuleName    string `json:"rule_name"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	MaxBytes    int64  `json:"max_bytes"`
	Mode        string `json:"cleanup_mode,omitempty"`
	Unit        string `json:"cleanup_unit,omitempty"`
	AutoCleanup bool   `json:"auto_cleanup"`
	LastBytes   *int64 `json:"last_allocated_bytes"`
}

func newRulesCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "rules",
		Short: "Print the resolved rules with their last measured size",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := s.loadConfig(false)
			if err != nil {
				return err
			}
			last := map[string]int64{}
			if dir, err := s.stateDir(); err == nil {
				if st, err := dir.ReadStatus(); err == nil {
					for _, r := range st.Rules {
						if r.MeasuredAt != nil {
							last[r.RuleName] = r.AllocatedBytes
						}
					}
				} else if !errors.Is(err, state.ErrNoStatus) {
					return exitWith(ExitUnknown, err)
				}
			}
			refs := cfg.Rules()
			rows := make([]report.RuleRow, 0, len(refs))
			out := make([]ruleJSON, 0, len(refs))
			for _, r := range refs {
				var size *int64
				if v, ok := last[r.Name]; ok {
					size = &v
				}
				rows = append(rows, report.RuleRow{Name: r.Name, Kind: r.Kind, Path: r.Path, Max: int64(r.MaxSize), Mode: r.Mode, Unit: r.Unit, Auto: r.AutoCleanup, LastSize: size})
				out = append(out, ruleJSON{RuleName: r.Name, Kind: r.Kind, Path: r.Path, MaxBytes: int64(r.MaxSize), Mode: r.Mode, Unit: r.Unit, AutoCleanup: r.AutoCleanup, LastBytes: size})
			}
			if s.json {
				enc := json.NewEncoder(s.app.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			report.WriteRules(s.app.Stdout, rows, s.format)
			return nil
		},
	}
}
