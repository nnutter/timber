package autoreview

import (
	"context"
	"errors"
	"strings"
)

const PolicyVersion = "refactor-v1"

// MaxInputBytes limits the JSON-encoded input, including JSON overhead, to 256 KiB.
const MaxInputBytes = 262144

// MaxContextBytes limits the initial JSON input plus raw tool result bytes to 512 KiB.
const MaxContextBytes = 524288

type Input struct {
	Subject  string   `json:"subject"`
	Messages []string `json:"messages"`
	Diff     string   `json:"diff"`
}

type Verdict struct {
	Decision   string   `json:"decision"`
	Reason     string   `json:"reason"`
	Evidence   []string `json:"evidence"`
	Model      string   `json:"model"`
	ResponseID string   `json:"responseId"`
}

func (v Verdict) Validate() error {
	switch v.Decision {
	case "approve", "needs-human-review", "not-eligible":
	default:
		return errors.New("invalid decision")
	}
	if strings.TrimSpace(v.Reason) == "" {
		return errors.New("empty reason")
	}
	if strings.TrimSpace(v.Model) == "" || strings.TrimSpace(v.ResponseID) == "" {
		return errors.New("missing response metadata")
	}
	if v.Decision == "approve" && len(v.Evidence) == 0 {
		return errors.New("approval requires evidence")
	}
	for _, e := range v.Evidence {
		if strings.TrimSpace(e) == "" {
			return errors.New("blank evidence")
		}
	}
	return nil
}

type ContextReader interface {
	ReadFile(context.Context, string, string) (string, error)
	Search(context.Context, string, string) (string, error)
}
type Evaluator interface {
	Evaluate(context.Context, Input, ContextReader) (Verdict, error)
}
