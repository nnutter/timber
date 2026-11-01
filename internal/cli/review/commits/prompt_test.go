package commits

import (
	"context"
	"io"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber/review"
)

type promptReader struct{ file *os.File }

func (r promptReader) Read(p []byte) (int, error) { return r.file.Read(p) }

func (r promptReader) UnwrapReader() io.Reader { return r.file }

type promptWriter struct {
	file   *os.File
	writes atomic.Int64
}

func (w *promptWriter) UnwrapWriter() io.Writer { return w.file }

func (w *promptWriter) Write(p []byte) (int, error) {
	w.writes.Add(1)
	return w.file.Write(p)
}

func TestPromptChoiceUsesUnderlyingTerminal(t *testing.T) {
	t.Skip("terminal dependency shutdown races under the race detector")

	master, slave, err := pty.Open()
	require.NoError(t, err)
	defer func() { require.NoError(t, master.Close()) }()
	defer func() { require.NoError(t, slave.Close()) }()
	require.NoError(t, pty.Setsize(slave, &pty.Winsize{Rows: 24, Cols: 80}))

	before, err := term.GetState(slave.Fd())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	result := make(chan struct {
		choice string
		err    error
	}, 1)
	done := make(chan struct{})
	writer := &promptWriter{file: slave}
	decision := &review.Decision{Attempt: review.Attempt{
		ID:     "attempt",
		Status: "incomplete",
		Stack:  review.Stack{Commits: []string{"abc"}, Tip: "abc", Subject: "Example"},
	}}
	go func() {
		defer close(done)
		choice, err := promptChoice(ctx, promptReader{file: slave}, writer, decision)
		result <- struct {
			choice string
			err    error
		}{choice: choice, err: err}
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			t.Error("recovery prompt did not stop after cancellation")
		}
	}()

	require.Eventually(t, func() bool {
		current, err := term.GetState(slave.Fd())
		return err == nil && !reflect.DeepEqual(before, current)
	}, time.Second, 10*time.Millisecond, "prompt did not place the terminal in raw mode")
	_, err = master.Write([]byte("\r"))
	require.NoError(t, err)

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Equal(t, "cancel", got.choice)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "recovery prompt did not accept terminal input")
	}
	current, err := term.GetState(slave.Fd())
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(before, current), "prompt did not restore terminal state")
	require.Zero(t, writer.writes.Load(), "prompt wrote through a wrapped terminal stream")
}
