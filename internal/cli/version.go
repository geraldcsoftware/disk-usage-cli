//go:build darwin

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(s.app.Stdout, "dusk %s (commit %s, built %s)\n", s.app.Info.Version, s.app.Info.Commit, s.app.Info.Date)
			return err
		},
	}
}
