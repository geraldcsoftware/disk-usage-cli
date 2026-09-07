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
			if prompt {
				return s.runPrompt()
			}
			dir, err := s.stateDir()
			if err != nil {
				return err
			}
			st, err := dir.ReadStatus()
			if errors.Is(err, state.ErrNoStatus) {
				fmt.Fprintln(s.app.Stderr, "dusk:", err)
				return nil
			}
			if err != nil {
				return exitWith(ExitUnknown, err)
			}
			if s.json {
				enc := json.NewEncoder(s.app.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(st)
			}
			report.WriteStatus(s.app.Stdout, st, s.format, s.palette, s.app.Now())
			return nil
		},
	}
	cmd.Flags().BoolVar(&prompt, "prompt", false, "print the Starship segment text and nothing else")
	return cmd
}

// runPrompt writes the Starship segment. It runs on every shell prompt, so it
// stays silent and successful whatever the state of the state directory: a
// missing, torn or unreadable status.json produces no output and exit 0.
func (s *session) runPrompt() error {
	dir, err := s.stateDir()
	if err != nil {
		return nil
	}
	st, err := dir.ReadStatus()
	if err != nil {
		return nil
	}
	_, err = fmt.Fprint(s.app.Stdout, report.RenderPrompt(st, s.promptTemplates(), s.app.Now()))
	return err
}

// promptTemplates reads the prompt templates from the config file. The config
// is parsed but never validated, since validation consults the filesystem and
// tmutil and the prompt must stay within a few milliseconds; an unusable
// config falls back to the built in templates.
func (s *session) promptTemplates() config.Prompt {
	if s.configPath == "" {
		return config.Default().Prompt
	}
	cfg, err := config.Load(s.configPath, s.home)
	if err != nil {
		return config.Default().Prompt
	}
	return cfg.Prompt
}
