//go:build darwin

package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
)

func newConfigCmd(s *session) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Inspect the configuration"}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the resolved config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(s.app.Stdout, s.configPath)
			return err
		},
	}, &cobra.Command{
		Use:   "validate",
		Short: "Parse and check the config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, warnings, err := s.loadConfig()
			if err != nil {
				var ve *config.ValidationError
				if errors.As(err, &ve) {
					for _, line := range ve.Errors {
						fmt.Fprintln(s.app.Stderr, "error:", line)
					}
					return exitWith(ExitConfig, fmt.Errorf("%s: %d problem(s)", s.configPath, len(ve.Errors)))
				}
				return err
			}
			fmt.Fprintln(s.app.Stdout, "config valid:", s.configPath)
			for _, w := range warnings {
				fmt.Fprintln(s.app.Stdout, "warning:", w)
			}
			return nil
		},
	})
	return cmd
}
