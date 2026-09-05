//go:build darwin

package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
	"github.com/geraldcsoftware/disk-usage-cli/internal/report"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

func newStatusCmd(s *session) *cobra.Command {
	var prompt bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the result of the last check without scanning",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := s.loadConfig()
			if err != nil && !prompt {
				return err
			}
			promptTemplates := config.Default().Prompt
			if cfg != nil {
				promptTemplates = cfg.Prompt
			}
			dir, err := s.stateDir()
			if err != nil {
				return err
			}
			st, err := dir.ReadStatus()
			if errors.Is(err, state.ErrNoStatus) {
				if !prompt {
					fmt.Fprintln(s.app.Stderr, "dusk:", err)
				}
				return nil
			}
			if err != nil {
				return exitWith(ExitUnknown, err)
			}
			now := s.app.Now()
			switch {
			case prompt:
				_, err = fmt.Fprint(s.app.Stdout, report.RenderPrompt(st, promptTemplates, now))
				return err
			case s.json:
				enc := json.NewEncoder(s.app.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(st)
			default:
				report.WriteStatus(s.app.Stdout, st, s.format, s.palette, now)
				return nil
			}
		},
	}
	cmd.Flags().BoolVar(&prompt, "prompt", false, "print the Starship segment text and nothing else")
	return cmd
}
