package quotas

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScreenSaysItIsReadingUntilTheFirstAnswer(t *testing.T) {
	t.Parallel()
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	view := model.View()

	assert.Contains(t, view, "Reading usage…")
	assert.Contains(t, view, "q quit")
}

func TestScreenQuitsOnTheQuitKeys(t *testing.T) {
	t.Parallel()
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyEsc},
	} {
		_, command := model.Update(key)
		require.NotNil(t, command)
		_, quit := command().(tea.QuitMsg)
		assert.True(t, quit, key.String())
	}
}

func TestScreenShowsAFailedRead(t *testing.T) {
	t.Parallel()
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	next, _ := model.Update(fetchedMsg{err: errors.New("offline"), at: time.Now()})

	view := ansi.Strip(next.View())
	assert.Contains(t, view, "offline")
	assert.NotContains(t, view, "Reading usage…")
}

func TestScreenRefreshReadsTheServicesAgain(t *testing.T) {
	t.Parallel()
	var fresh []bool
	model := newScreen(func(force bool) (Snapshot, error) {
		fresh = append(fresh, force)
		return Snapshot{}, nil
	})
	opened, _ := model.Update(fetchedMsg{at: time.Now(), snapshot: Snapshot{}})

	reading, command := opened.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	require.NotNil(t, command)
	pressed, ok := command().(fetchedMsg)
	require.True(t, ok)
	settled, _ := reading.Update(pressed)

	_, command = settled.Update(refreshMsg{})
	require.NotNil(t, command)
	_, ok = command().(fetchedMsg)
	require.True(t, ok)
	assert.Equal(t, []bool{true, true}, fresh)
}

func TestScreenMovesThePlanToSpendAsTheWindowRunsDown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	// Short wastes less of its ten minutes than Long wastes of its hour.
	// Once Short's reset passes, the remainder would be thrown away, so it
	// becomes the plan to spend.
	short := Report{Name: "Short", OK: true, Limits: []Limit{{
		Label: "Ten minutes", Percent: 50, Window: 10 * time.Minute, ResetsAt: now.Add(9 * time.Minute),
	}}}
	long := Report{Name: "Long", OK: true, Limits: []Limit{{
		Label: "Hour", Percent: 20, Window: time.Hour, ResetsAt: now.Add(30 * time.Minute),
	}}}
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	opened, _ := model.Update(fetchedMsg{
		at:       now,
		snapshot: Snapshot{FetchedAt: now, Reports: []Report{short, long}},
	})
	first := ansi.Strip(opened.View())
	longAt := strings.Index(first, "Long")
	shortAt := strings.Index(first, "Short")
	require.NotEqual(t, -1, longAt)
	require.NotEqual(t, -1, shortAt)
	assert.Less(t, longAt, shortAt)

	later, _ := opened.Update(ageMsg(now.Add(9 * time.Minute)))
	second := ansi.Strip(later.View())
	longAt = strings.Index(second, "Long")
	shortAt = strings.Index(second, "Short")
	require.NotEqual(t, -1, longAt)
	require.NotEqual(t, -1, shortAt)
	assert.Less(t, shortAt, longAt)
}
