package autoreview

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
)

type failingReader struct {
	output string
	err    error
}

func (r failingReader) ReadFile(context.Context, string, string) (string, error) {
	return r.output, r.err
}

func (r failingReader) Search(context.Context, string, string) (string, error) {
	return r.output, r.err
}

func TestOpenAIToolFailuresTerminate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader ContextReader
		max    int
		want   string
	}{
		{"reader error", failingReader{err: errors.New("denied")}, 0, "denied"},
		{"context limit", failingReader{output: strings.Repeat("x", MaxContextBytes)}, 0, "context budget"},
		{"request limit", testReader{}, 1, "request budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"r","model":"m","status":"completed","output":[{"type":"function_call","status":"completed","call_id":"c","name":"read_file","arguments":"{\"revision\":\"base\",\"path\":\"x\"}"}]}`))
			}))
			defer server.Close()
			o := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m", MaxRequests: tc.max}
			v, err := o.Evaluate(t.Context(), Input{}, tc.reader)
			require.ErrorContains(t, err, tc.want)
			require.Equal(t, Verdict{}, v)
			require.Equal(t, 1, count)
		})
	}
}

func TestOpenAIRejectsNegativeOutputBudget(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
	}))
	defer server.Close()
	o := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m", MaxOutputTokens: -1}
	_, err := o.Evaluate(t.Context(), Input{}, nil)
	require.ErrorContains(t, err, "invalid model or request budget")
	require.Equal(t, 0, count)
}

func TestOpenAINoRetriesAndCancellation(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		http.Error(w, "unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()
	o := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m"}
	_, err := o.Evaluate(t.Context(), Input{}, nil)
	require.Error(t, err)
	require.Equal(t, 1, count)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = o.Evaluate(ctx, Input{}, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, count)
}
