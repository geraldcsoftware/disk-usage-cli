//go:build darwin

package report

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/geraldcsoftware/disk-usage-cli/internal/config"
	"github.com/geraldcsoftware/disk-usage-cli/internal/state"
)

// RenderPrompt produces the Starship segment from the last status. It prints
// the stale text when the status has outlived stale_after, nothing when no
// status exists, and appends over_max_suffix whenever rules are over maximum.
// Placeholders: {free_pct} {free_gb} {over_max_count}.
func RenderPrompt(st *state.Status, p config.Prompt, now time.Time) string {
	if st == nil {
		return ""
	}
	if now.After(st.StaleAfter) {
		return strings.TrimSpace(p.TextWhenStale)
	}
	var text string
	switch st.Disk.State {
	case "warn":
		text = p.TextWhenWarn
	case "critical":
		text = p.TextWhenCritical
	default:
		text = p.TextWhenOK
	}
	if st.Summary.OverMaxCount > 0 {
		text += p.OverMaxSuffix
	}
	r := strings.NewReplacer(
		"{free_pct}", fmt.Sprintf("%.0f", st.Disk.FreePct),
		"{free_gb}", fmt.Sprintf("%.0f", float64(st.Disk.UsableFreeBytes)/1e9),
		"{over_max_count}", strconv.Itoa(st.Summary.OverMaxCount),
	)
	return strings.TrimSpace(r.Replace(text))
}
