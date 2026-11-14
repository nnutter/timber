// Package quotas exposes `timber quotas`.
package quotas

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nnutter/timber/internal/quotas"
	"github.com/nnutter/timber/internal/terminal"
)

// Dependencies connects the quotas command to a usage read.
// Zero values read the signed-in services, the current time, and the terminal.
type Dependencies struct {
	Read        quotas.Reader
	Now         func() time.Time
	Width       func(io.Writer) int
	Interactive func(io.Reader) bool
	RunTUI      func(context.Context, io.Reader, io.Writer, quotas.Reader) error
}

// New constructs the command that shows which subscription to use next.
func New(deps Dependencies) *cobra.Command {
	if deps.Read == nil {
		deps.Read = quotas.NewCollector().Reports
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Width == nil {
		deps.Width = terminalWidth
	}
	if deps.Interactive == nil {
		deps.Interactive = terminal.IsInteractive
	}
	if deps.RunTUI == nil {
		deps.RunTUI = quotas.Run
	}
	var keep, asJSON, rolling bool
	command := &cobra.Command{
		Use:   "quotas",
		Short: "Show AI subscription usage, soonest waste first",
		Long: `Show how full each signed-in AI subscription is.

The plan that would waste the most of its longest window comes first:
unused share divided by the share of that window still left. Spend that
subscription before its reset throws the remainder away. A plan with no
reset clock comes next, least used first. A balance with no limits comes
after those, and a plan that failed comes last.

The report prints once. --tui keeps it on screen and refreshes it
every 5 minutes while the windows run down. Keys 1, 5, and 0 choose
1, 5, and 10 minutes. --json prints the clocks, balances, and failures
once, for another program to draw. It cannot be combined with --tui.
--rolling includes OpenCode Go's 5-hour window, which the report
otherwise leaves out.`,
		Args: cobra.NoArgs,
		RunE: run(deps, &keep, &asJSON, &rolling),
	}
	command.Flags().BoolVar(&keep, "tui", false, "Keep the report on screen and refresh it")
	command.Flags().BoolVar(&asJSON, "json", false, "Print the report as JSON")
	command.Flags().BoolVar(&rolling, "rolling", false, "Show OpenCode Go's 5-hour window")
	command.MarkFlagsMutuallyExclusive("tui", "json")
	return command
}

func run(deps Dependencies, keep, asJSON, rolling *bool) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, _ []string) error {
		read := deps.Read
		if !*rolling {
			read = hideOpenCodeRolling(read)
		}
		if *keep {
			if !deps.Interactive(command.InOrStdin()) {
				return fmt.Errorf("quotas --tui needs a terminal")
			}
			return deps.RunTUI(command.Context(), command.InOrStdin(), command.OutOrStdout(), read)
		}
		snapshot, err := read(command.Context(), true)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(command.OutOrStdout(), snapshot)
		}
		text := quotas.Render(snapshot, deps.Now(), deps.Width(command.OutOrStdout()))
		_, err = fmt.Fprintln(colorprofile.NewWriter(command.OutOrStdout(), os.Environ()), text)
		return err
	}
}

func hideOpenCodeRolling(read quotas.Reader) quotas.Reader {
	return func(ctx context.Context, fresh bool) (quotas.Snapshot, error) {
		snapshot, err := read(ctx, fresh)
		if err != nil {
			return quotas.Snapshot{}, err
		}
		return quotas.OmitOpenCodeRolling(snapshot), nil
	}
}

func printJSON(output io.Writer, snapshot quotas.Snapshot) error {
	encoded, err := quotas.Encode(snapshot)
	if err != nil {
		return fmt.Errorf("encode quotas: %w", err)
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func terminalWidth(output io.Writer) int {
	file, ok := output.(*os.File)
	if !ok {
		return 80
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || narrow(width) {
		return 80
	}
	return width
}

func narrow(width int) bool { return width < 24 }
