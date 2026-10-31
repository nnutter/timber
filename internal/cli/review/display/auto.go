package display

import (
	"fmt"
	"io"
	"strings"

	"github.com/nnutter/timber/internal/display"
	"github.com/nnutter/timber/internal/timber/review"
)

// WriteAuto keeps recommendations distinct from persisted model approvals.
func WriteAuto(output io.Writer, asJSON bool, result review.AutoResult) error {
	if asJSON {
		return display.WriteJSON(output, result)
	}
	var text strings.Builder
	if result.DryRun {
		fmt.Fprintln(&text, "Automatic refactor assessment: dry run; no approvals recorded.")
	} else {
		fmt.Fprintln(&text, "Automatic refactor assessment complete.")
	}
	for _, entry := range result.Entries {
		label := "Skipped"
		if verdict := entry.Evaluation; verdict != nil {
			label = autoDecisionLabel(verdict.Decision)
			if verdict.Decision == "approve" && !entry.Approved {
				label = "Would approve"
			}
			if entry.Approved {
				label = "Auto-Approved"
			}
		}
		fmt.Fprintf(&text, "%s  %s  %s\n  %s\n",
			label, ShortStack(entry.Stack), safeText(entry.Stack.Subject), safeText(entry.Reason))
		if verdict := entry.Evaluation; verdict != nil {
			fmt.Fprintf(&text, "  Model: %s · Response: %s\n", safeText(verdict.Model), safeText(verdict.ResponseID))
			for _, evidence := range verdict.Evidence {
				fmt.Fprintln(&text, "  - "+safeText(evidence))
			}
		}
	}
	_, err := fmt.Fprint(output, text.String())
	return err
}
