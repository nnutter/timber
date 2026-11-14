package quotas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// refreshEvery is the ordinary wait between reads of the services.
	refreshEvery = time.Minute
	// refreshBackoff is the longer wait after a read fails.
	refreshBackoff = 5 * time.Minute
	// ageEvery moves the reset clocks without asking the services again.
	ageEvery = 30 * time.Second
)

// Reader loads one snapshot.
// fresh asks the services again, still no more often than their minimum gap.
type Reader func(context.Context, bool) (Snapshot, error)

// Run keeps the report on screen until the user quits.
// It reads again each minute and moves the clocks in between, because the
// plan to spend changes as a window runs down.
func Run(ctx context.Context, input io.Reader, output io.Writer, read Reader) error {
	if read == nil {
		read = NewCollector().Reports
	}
	if ctx == nil {
		ctx = context.Background()
	}
	model := newScreen(func(fresh bool) (Snapshot, error) {
		return read(ctx, fresh)
	})
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithContext(ctx)}
	if input != nil {
		options = append(options, tea.WithInput(input))
	}
	if output != nil {
		options = append(options, tea.WithOutput(output))
	}
	_, err := tea.NewProgram(model, options...).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("quotas: %w", err)
	}
	return nil
}

type screen struct {
	read     func(bool) (Snapshot, error)
	now      time.Time
	width    int
	snapshot Snapshot
	ready    bool
	reading  bool
	problem  string
}

func newScreen(read func(bool) (Snapshot, error)) *screen {
	return new(screen{
		read:    read,
		now:     time.Now().UTC(),
		width:   80,
		reading: true,
	})
}

func (s *screen) Init() tea.Cmd {
	return tea.Batch(s.fetch(true), scheduleAge())
}

func (s *screen) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		return s.onKey(message)
	case tea.WindowSizeMsg:
		if message.Width >= 24 {
			s.width = message.Width
		}
	case ageMsg:
		s.now = time.Time(message)
		return s, scheduleAge()
	case refreshMsg:
		return s.refresh()
	case fetchedMsg:
		return s.took(message)
	}
	return s, nil
}

func (s *screen) View() string {
	var out strings.Builder
	switch {
	case s.ready:
		out.WriteString(Render(s.snapshot, s.now, s.width))
		out.WriteString(ageLine(s.snapshot, s.now))
		if s.problem != "" {
			out.WriteString("\n\n")
			out.WriteString(mutedStyle.Render(s.problem))
		}
	case s.problem != "":
		out.WriteString(mutedStyle.Render(s.problem))
	default:
		out.WriteString("Reading usage…")
	}
	out.WriteString("\n\n")
	out.WriteString(mutedStyle.Render("r refresh · q quit"))
	return out.String()
}

func (s *screen) fetch(fresh bool) tea.Cmd {
	read := s.read
	return func() tea.Msg {
		if read == nil {
			return fetchedMsg{err: errors.New("no usage reader"), at: time.Now()}
		}
		snapshot, err := read(fresh)
		return fetchedMsg{snapshot: snapshot, err: err, at: time.Now()}
	}
}

func (s *screen) onKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c", "esc":
		return s, tea.Quit
	case "r":
		return s.refresh()
	}
	return s, nil
}

func (s *screen) refresh() (tea.Model, tea.Cmd) {
	if s.reading {
		return s, nil
	}
	s.reading = true
	return s, s.fetch(true)
}

func (s *screen) took(message fetchedMsg) (tea.Model, tea.Cmd) {
	s.reading = false
	s.now = message.at
	if message.err != nil {
		s.problem = message.err.Error()
		return s, scheduleRefresh(refreshBackoff)
	}
	s.snapshot = message.snapshot
	s.ready = true
	s.problem = ""
	return s, scheduleRefresh(refreshEvery)
}

func ageLine(snapshot Snapshot, now time.Time) string {
	if len(Ordered(snapshot.Reports, now)) == 0 {
		return ""
	}
	age := Age(snapshot.FetchedAt, now)
	if age == "" {
		return ""
	}
	return "\n\n" + mutedStyle.Render(age)
}

func scheduleAge() tea.Cmd {
	return tea.Tick(ageEvery, func(at time.Time) tea.Msg {
		return ageMsg(at)
	})
}

func scheduleRefresh(after time.Duration) tea.Cmd {
	return tea.Tick(after, func(time.Time) tea.Msg {
		return refreshMsg{}
	})
}

type ageMsg time.Time

type refreshMsg struct{}

type fetchedMsg struct {
	snapshot Snapshot
	err      error
	at       time.Time
}
