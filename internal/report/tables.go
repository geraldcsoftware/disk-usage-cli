//go:build darwin

package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

// RuleRow is one line of dusk rules.
type RuleRow struct {
	Name     string
	Kind     string
	Path     string
	Max      int64
	Mode     string
	Unit     string
	Auto     bool
	LastSize *int64
}

// WriteRules prints the resolved rules with their last measured size.
func WriteRules(w io.Writer, rows []RuleRow, f Format) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tKIND\tMAX\tMODE\tUNIT\tAUTO\tLAST SIZE\tPATH")
	for _, r := range rows {
		last := "unmeasured"
		if r.LastSize != nil {
			last = f.Bytes(*r.LastSize)
		}
		mode, unit := r.Mode, r.Unit
		if r.Kind == "command" {
			mode, unit = "command", "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%v\t%s\t%s\n", r.Name, r.Kind, f.Bytes(r.Max), mode, unit, r.Auto, last, r.Path)
	}
	tw.Flush()
}

// WriteStatus prints the last check result.
func WriteStatus(w io.Writer, st *state.Status, f Format, pal Palette, now time.Time) {
	fmt.Fprintf(w, "disk    %s  %s free of %s (%.1f%%)  state %s for %s\n",
		st.Disk.Volume, f.Bytes(st.Disk.UsableFreeBytes), f.Bytes(st.Disk.TotalBytes), st.Disk.FreePct,
		pal.State(st.Disk.State), Age(now.Sub(st.Disk.StateSince)))
	fmt.Fprintf(w, "checked %s (%s ago)", st.TS.Local().Format("2006-01-02 15:04"), Age(now.Sub(st.TS)))
	if now.After(st.StaleAfter) {
		fmt.Fprint(w, "  stale")
	}
	fmt.Fprintln(w)
	if len(st.Rules) == 0 {
		fmt.Fprintln(w, "rules   none measured yet")
		return
	}
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "RULE\tKIND\tSIZE\tMAX\tSTATUS\tMEASURED")
	for _, r := range st.Rules {
		status := "ok"
		if r.OverMax {
			status = "over"
		}
		measured := "never"
		if r.MeasuredAt != nil {
			measured = Age(now.Sub(*r.MeasuredAt)) + " ago"
		}
		if r.Unmeasured.PrivacyProtected > 0 || r.Unmeasured.CloudOnly > 0 {
			status += fmt.Sprintf(" (partial: %d privacy, %d cloud)", r.Unmeasured.PrivacyProtected, r.Unmeasured.CloudOnly)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.RuleName, r.Kind, f.Bytes(r.AllocatedBytes), f.Bytes(r.MaxBytes), status, measured)
	}
	tw.Flush()
}

// ReportUnit is one of the largest units of a rule.
type ReportUnit struct {
	RelPath   string    `json:"rel_path"`
	Allocated int64     `json:"allocated_bytes"`
	ModTime   time.Time `json:"mod_time"`
	Freeable  bool      `json:"freeable"`
}

// ReportRule is one rule measured now.
type ReportRule struct {
	Name            string
	Kind            string
	Path            string
	Allocated       int64
	Apparent        int64
	CloudOnly       int64
	Max             int64
	OverMax         bool
	Growth7d        *int64
	Growth30d       *int64
	Largest         []ReportUnit
	Skipped         map[string]int
	UnmeasuredRoots []string
	Err             string
}

// Report is the output of dusk report.
type Report struct {
	TS    time.Time
	Disk  state.DiskStatus
	Rules []ReportRule
}

// WriteReport prints the disk line, the rule table with growth, then the
// largest units and skipped counts per rule.
func WriteReport(w io.Writer, r Report, f Format, pal Palette) {
	fmt.Fprintf(w, "disk %s  %s free of %s (%.1f%%)  state %s\n\n",
		r.Disk.Volume, f.Bytes(r.Disk.UsableFreeBytes), f.Bytes(r.Disk.TotalBytes), r.Disk.FreePct, pal.State(r.Disk.State))
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "RULE\tKIND\tSIZE\tMAX\tSTATUS\t7D\t30D")
	for _, rule := range r.Rules {
		if rule.Err != "" {
			fmt.Fprintf(tw, "%s\t%s\t-\t%s\terror\t-\t-\n", rule.Name, rule.Kind, f.Bytes(rule.Max))
			continue
		}
		status := "ok"
		if rule.OverMax {
			status = "over"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", rule.Name, rule.Kind, f.Bytes(rule.Allocated), f.Bytes(rule.Max), status, growth(f, rule.Growth7d), growth(f, rule.Growth30d))
	}
	tw.Flush()
	for _, rule := range r.Rules {
		fmt.Fprintf(w, "\n%s  %s\n", rule.Name, rule.Path)
		if rule.Err != "" {
			fmt.Fprintf(w, "  not measured: %s\n", rule.Err)
			continue
		}
		if rule.CloudOnly > 0 {
			fmt.Fprintf(w, "  cloud only: %s not on disk\n", f.Bytes(rule.CloudOnly))
		}
		for _, u := range rule.Largest {
			line := fmt.Sprintf("  %10s  %s  %s", f.Bytes(u.Allocated), u.ModTime.Local().Format("2006-01-02"), u.RelPath)
			if !u.Freeable {
				line += " (not freeable)"
			}
			fmt.Fprintln(w, line)
		}
		if len(rule.Skipped) > 0 {
			keys := make([]string, 0, len(rule.Skipped))
			for k := range rule.Skipped {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, len(keys))
			for i, k := range keys {
				parts[i] = fmt.Sprintf("%s: %d", k, rule.Skipped[k])
			}
			fmt.Fprintf(w, "  skipped  %s\n", strings.Join(parts, ", "))
		}
		for _, root := range rule.UnmeasuredRoots {
			fmt.Fprintf(w, "  not measured (privacy protected): %s\n", root)
		}
	}
}

func growth(f Format, g *int64) string {
	if g == nil {
		return "-"
	}
	return f.Signed(*g)
}
