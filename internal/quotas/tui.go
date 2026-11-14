package quotas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	// defaultPoll is the wait between reads until a key chooses another.
	defaultPoll = 5 * time.Minute
	// ageEvery moves the reset clocks without asking the services again.
	ageEvery = 30 * time.Second

	screenWidth  = 80
	screenHeight = 40
)

// pollChoice is one key on the quota screen and the wait it selects.
type pollChoice struct {
	key      string
	label    string
	interval time.Duration
}

// polls are the waits the screen offers, in the order the footer shows them.
var polls = []pollChoice{
	{key: "1", label: "1m", interval: time.Minute},
	{key: "5", label: "5m", interval: 5 * time.Minute},
	{key: "0", label: "10m", interval: 10 * time.Minute},
}

// Reader loads one snapshot.
// fresh asks the services again, still no more often than their minimum gap.
type Reader func(context.Context, bool) (Snapshot, error)

// Run keeps the report on screen until the user quits.
// It reads again every five minutes, or every one or ten when that key is
// chosen, and moves the clocks in between, because the plan to spend changes
// as a window runs down. The wheel and a touch drag scroll the report; the
// program has to ask for those events or the terminal keeps them.
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
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx)}
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
	height   int
	view     viewport.Model
	dragY    int
	dragging bool
	snapshot Snapshot
	ready    bool
	reading  bool
	problem  string
	interval time.Duration
	// pollID is the timer the screen is waiting on. An older timer is ignored.
	pollID int
}

func newScreen(read func(bool) (Snapshot, error)) *screen {
	return new(screen{
		read:     read,
		now:      time.Now().UTC(),
		width:    screenWidth,
		height:   screenHeight,
		view:     viewport.New(screenWidth, contentHeight(screenHeight)),
		reading:  true,
		interval: defaultPoll,
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
		s.resize(message)
		return s, nil
	case tea.MouseMsg:
		return s.onMouse(message)
	case ageMsg:
		s.now = time.Time(message)
		s.sync()
		return s, scheduleAge()
	case refreshMsg:
		if message.id != s.pollID {
			return s, nil
		}
		return s.refresh()
	case fetchedMsg:
		return s.took(message)
	}
	return s, nil
}

func (s *screen) View() string {
	s.sync()
	var out strings.Builder
	out.WriteString(s.view.View())
	out.WriteString("\n")
	out.WriteString(s.footer())
	return out.String()
}

// armPoll waits, then reads again. A newer call retires the previous wait.
func (s *screen) armPoll(after time.Duration) tea.Cmd {
	s.pollID++
	id := s.pollID
	return tea.Tick(after, func(time.Time) tea.Msg {
		return refreshMsg{id: id}
	})
}

func (s *screen) body() string {
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
	return out.String()
}

func (s *screen) choosePoll(interval time.Duration) (tea.Model, tea.Cmd) {
	if s.interval == interval {
		return s, nil
	}
	s.interval = interval
	if s.reading {
		return s, nil
	}
	return s, s.armPoll(s.interval)
}

func (s *screen) drag(message tea.MouseMsg) {
	if tea.MouseEvent(message).IsWheel() {
		return
	}
	if startingDrag(message) {
		s.dragging = true
		s.dragY = message.Y
		return
	}
	if message.Action == tea.MouseActionRelease {
		s.dragging = false
		return
	}
	s.followDrag(message)
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

func (s *screen) followDrag(message tea.MouseMsg) {
	if !s.dragging {
		return
	}
	if message.Action != tea.MouseActionMotion {
		return
	}
	delta := s.dragY - message.Y
	s.dragY = message.Y
	s.scrollBy(delta)
}

func (s *screen) footer() string {
	var out strings.Builder
	out.WriteString(mutedStyle.Render("r refresh"))
	for _, choice := range polls {
		out.WriteString(mutedStyle.Render(" · "))
		out.WriteString(s.pollLabel(choice))
	}
	out.WriteString(mutedStyle.Render(" · q quit"))
	return out.String()
}

func (s *screen) onKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c", "esc":
		return s, tea.Quit
	case "r":
		return s.refresh()
	}
	interval, chosen := pollInterval(message.String())
	if chosen {
		return s.choosePoll(interval)
	}
	var command tea.Cmd
	s.view, command = s.view.Update(message)
	return s, command
}

func (s *screen) onMouse(message tea.MouseMsg) (tea.Model, tea.Cmd) {
	s.drag(message)
	var command tea.Cmd
	s.view, command = s.view.Update(message)
	return s, command
}

func (s *screen) pollLabel(choice pollChoice) string {
	label := choice.key + " " + choice.label
	if choice.interval == s.interval {
		return "[" + label + "]"
	}
	return mutedStyle.Render(label)
}

func (s *screen) refresh() (tea.Model, tea.Cmd) {
	if s.reading {
		return s, nil
	}
	s.reading = true
	return s, s.fetch(true)
}

func (s *screen) resize(message tea.WindowSizeMsg) {
	if message.Width >= minWidth {
		s.width = message.Width
	}
	if message.Height > 1 {
		s.height = message.Height
	}
	s.sync()
}

func (s *screen) scrollBy(delta int) {
	if delta > 0 {
		s.view.ScrollDown(delta)
		return
	}
	if delta < 0 {
		s.view.ScrollUp(-delta)
	}
}

func (s *screen) sync() {
	s.view.Width = max(s.width, 1)
	s.view.Height = contentHeight(s.height)
	s.view.SetContent(s.body())
}

func (s *screen) took(message fetchedMsg) (tea.Model, tea.Cmd) {
	s.reading = false
	s.now = message.at
	if message.err != nil {
		s.problem = message.err.Error()
	} else {
		s.snapshot = message.snapshot
		s.ready = true
		s.problem = ""
	}
	s.sync()
	return s, s.armPoll(s.interval)
}

func startingDrag(message tea.MouseMsg) bool {
	if message.Button != tea.MouseButtonLeft {
		return false
	}
	return message.Action == tea.MouseActionPress
}

func contentHeight(height int) int {
	if height <= 1 {
		return 1
	}
	return height - 1
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

func pollInterval(key string) (time.Duration, bool) {
	for _, choice := range polls {
		if choice.key == key {
			return choice.interval, true
		}
	}
	return 0, false
}

type ageMsg time.Time

type refreshMsg struct {
	id int
}

type fetchedMsg struct {
	snapshot Snapshot
	err      error
	at       time.Time
}
