package autoreview

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRejectsInvalidResponses(t *testing.T) {
	cases := []struct{ name, response string }{
		{"failed", `{"id":"r","model":"m","status":"failed","output":[]}`},
		{"refusal", `{"id":"r","model":"m","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"no"}]}]}`},
		{"unknown item", `{"id":"r","model":"m","status":"completed","output":[{"type":"shell_call"}]}`},
		{"missing evidence", `{"id":"r","model":"m","status":"completed","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"decision\":\"needs-human-review\",\"reason\":\"uncertain\"}"}]}]}`},
		{"extra tool argument", `{"id":"r","model":"m","status":"completed","output":[{"type":"function_call","status":"completed","call_id":"c","name":"read_file","arguments":"{\"revision\":\"base\",\"path\":\"x\",\"query\":\"\"}"}]}`},
		{"bad revision", `{"id":"r","model":"m","status":"completed","output":[{"type":"function_call","status":"completed","call_id":"c","name":"read_file","arguments":"{\"revision\":\"HEAD\",\"path\":\"x\"}"}]}`},
		{"unknown tool", `{"id":"r","model":"m","status":"completed","output":[{"type":"function_call","status":"completed","call_id":"c","name":"shell","arguments":"{\"revision\":\"base\",\"path\":\"x\"}"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			o := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m"}
			v, err := o.Evaluate(t.Context(), Input{}, testReader{})
			require.Error(t, err)
			require.Equal(t, Verdict{}, v)
		})
	}
}

func TestOpenAIRejectsOversizedInput(t *testing.T) {
	o := OpenAI{Model: "m"}
	v, err := o.Evaluate(t.Context(), Input{Diff: strings.Repeat("x", MaxInputBytes)}, nil)
	require.Error(t, err)
	require.Equal(t, Verdict{}, v)
}

func TestOpenAIIncompleteResponseAbstains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_incomplete","model":"m","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`))
	}))
	defer server.Close()
	o := OpenAI{Client: openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL+"/")), Model: "m"}
	v, err := o.Evaluate(t.Context(), Input{}, nil)
	require.NoError(t, err)
	require.Equal(t, "needs-human-review", v.Decision)
	require.Contains(t, v.Reason, "max_output_tokens")
	require.Equal(t, "resp_incomplete", v.ResponseID)
	require.NoError(t, v.Validate())
}
