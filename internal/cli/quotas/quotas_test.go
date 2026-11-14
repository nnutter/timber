package quotas

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/quotas"
)

func TestQuotasPrintsThePlanToSpendFirst(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var fresh bool
	command := New(Dependencies{
		Now: func() time.Time { return now },
		Read: func(_ context.Context, force bool) (quotas.Snapshot, error) {
			fresh = force
			return quotas.Snapshot{FetchedAt: now, Reports: []quotas.Report{
				{Name: "Short", OK: true, Limits: []quotas.Limit{{
					Label: "Ten minutes", Percent: 50, Window: 10 * time.Minute, ResetsAt: now.Add(9 * time.Minute),
				}}},
				{Name: "Long", OK: true, Limits: []quotas.Limit{{
					Label: "Hour", Percent: 20, Window: time.Hour, ResetsAt: now.Add(30 * time.Minute),
				}}},
			}}, nil
		},
		Width: func(io.Writer) int { return 40 },
	})
	command.SetIn(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetArgs([]string{})

	require.NoError(t, command.ExecuteContext(t.Context()))

	text := ansi.Strip(stdout.String())
	longAt := strings.Index(text, "Long")
	shortAt := strings.Index(text, "Short")
	require.NotEqual(t, -1, longAt)
	require.NotEqual(t, -1, shortAt)
	assert.Less(t, longAt, shortAt)
	assert.True(t, fresh)
	assert.NotContains(t, text, "just now")
}

func TestQuotasSaysWhenNothingIsSignedIn(t *testing.T) {
	t.Parallel()
	command := New(Dependencies{
		Read: func(context.Context, bool) (quotas.Snapshot, error) {
			return quotas.Snapshot{}, nil
		},
	})
	command.SetIn(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	require.NoError(t, command.ExecuteContext(t.Context()))
	assert.Contains(t, ansi.Strip(stdout.String()), "Nothing to report yet")
}

func TestQuotasReturnsAFailedRead(t *testing.T) {
	t.Parallel()
	command := New(Dependencies{
		Read: func(context.Context, bool) (quotas.Snapshot, error) {
			return quotas.Snapshot{}, errors.New("offline")
		},
	})
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	err := command.ExecuteContext(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offline")
}

func TestQuotasTUINeedsATerminal(t *testing.T) {
	t.Parallel()
	command := New(Dependencies{
		Read: func(context.Context, bool) (quotas.Snapshot, error) {
			require.Fail(t, "read the services")
			return quotas.Snapshot{}, nil
		},
		RunTUI: func(context.Context, io.Reader, io.Writer, quotas.Reader) error {
			require.Fail(t, "started the tui")
			return nil
		},
	})
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--tui"})

	err := command.ExecuteContext(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terminal")
}

func TestQuotasJSONPrintsTheReportOnce(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	snapshot := quotas.Snapshot{FetchedAt: now, Reports: []quotas.Report{{
		ID: "plan", Name: "Plan", OK: true,
		Limits: []quotas.Limit{{Label: "Week", Percent: 10}},
	}}}
	var fresh bool
	command := New(Dependencies{
		Read: func(_ context.Context, force bool) (quotas.Snapshot, error) {
			fresh = force
			return snapshot, nil
		},
		Now:   func() time.Time { require.Fail(t, "read the clock for a JSON report"); return now },
		Width: func(io.Writer) int { require.Fail(t, "measured the terminal for a JSON report"); return 40 },
	})
	command.SetIn(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetArgs([]string{"--json"})

	require.NoError(t, command.ExecuteContext(t.Context()))

	encoded, err := quotas.Encode(snapshot)
	require.NoError(t, err)
	assert.True(t, fresh)
	assert.Equal(t, string(encoded)+"\n", stdout.String())
}

func TestQuotasJSONReturnsAFailedRead(t *testing.T) {
	t.Parallel()
	command := New(Dependencies{
		Read: func(context.Context, bool) (quotas.Snapshot, error) {
			return quotas.Snapshot{}, errors.New("offline")
		},
	})
	command.SetIn(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetArgs([]string{"--json"})

	err := command.ExecuteContext(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offline")
	assert.NotContains(t, stdout.String(), "{")
}

func TestQuotasJSONAndTUICannotCombine(t *testing.T) {
	t.Parallel()
	command := New(Dependencies{
		Read: func(context.Context, bool) (quotas.Snapshot, error) {
			require.Fail(t, "read the services")
			return quotas.Snapshot{}, nil
		},
		RunTUI: func(context.Context, io.Reader, io.Writer, quotas.Reader) error {
			require.Fail(t, "started the tui")
			return nil
		},
	})
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--json", "--tui"})

	err := command.ExecuteContext(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "json")
	assert.Contains(t, err.Error(), "tui")
}
