package autoreview

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func encodeReviewInput(o OpenAI, input Input) ([]byte, error) {
	if strings.TrimSpace(o.Model) == "" || isLess(o.MaxRequests, 0) || isLess(o.MaxOutputTokens, 0) {
		return nil, errors.New("invalid model or request budget")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode input: %w", err)
	}
	if len(data) > MaxInputBytes {
		return nil, errors.New("input exceeds limit")
	}
	return data, nil
}

func newResponseParams(o OpenAI, data []byte) (responses.ResponseNewParams, int) {
	maxOutputTokens := int64(o.MaxOutputTokens)
	if maxOutputTokens == 0 {
		maxOutputTokens = DefaultMaxOutputTokens
	}
	p := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(o.Model),
		Instructions:    openai.String(instructions),
		Store:           openai.Bool(false),
		MaxOutputTokens: openai.Int(maxOutputTokens),
		Truncation:      responses.ResponseNewParamsTruncationDisabled,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfMessage(string(data), responses.EasyInputMessageRoleUser),
			},
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:   "verdict",
					Schema: verdictSchema(),
					Strict: openai.Bool(true),
				},
			},
		},
	}
	p.Include = []responses.ResponseIncludable{"reasoning.encrypted_content"}
	p.Tools = []responses.ToolUnionParam{functionTool("read_file", "path"), functionTool("search", "query")}
	maxRequests := o.MaxRequests
	if maxRequests == 0 {
		maxRequests = DefaultMaxRequests
	}
	return p, maxRequests
}

func collectVerdict(ctx context.Context, o OpenAI, reader ContextReader, p responses.ResponseNewParams, maxRequests, contextBytes int) (Verdict, error) {
	var zero Verdict
	calls := 0
	seen := map[string]bool{}
	for request := range maxRequests {
		verdict, done, err := reviewTurn(ctx, o, reader, &p, seen, &calls, &contextBytes, maxRequests, request)
		if err != nil {
			return zero, err
		}
		if done {
			return verdict, nil
		}
	}
	return zero, errors.New("request budget exhausted")
}

func reviewTurn(ctx context.Context, o OpenAI, reader ContextReader, p *responses.ResponseNewParams, seen map[string]bool, calls, contextBytes *int, maxRequests, request int) (Verdict, bool, error) {
	var zero Verdict
	resp, err := fetchReviewResponse(ctx, o, *p)
	if err != nil {
		return zero, false, err
	}
	if resp.Status != "completed" {
		verdict, err := incompleteVerdict(resp)
		return verdict, true, err
	}
	text, count, pending, err := consumeReviewOutput(resp, p, seen)
	if err != nil {
		return zero, false, err
	}
	if count > 0 && isLess(0, len(pending)) {
		return zero, false, errors.New("mixed verdict and calls")
	}
	if len(pending) > 0 {
		err := runReviewCalls(ctx, reader, p, pending, calls, contextBytes, maxRequests, request)
		return zero, false, err
	}
	verdict, err := decodeReviewVerdict(text, resp, count)
	return verdict, true, err
}

func fetchReviewResponse(ctx context.Context, o OpenAI, p responses.ResponseNewParams) (*responses.Response, error) {
	resp, err := o.Client.Responses.New(ctx, p, option.WithMaxRetries(0))
	if err != nil {
		return nil, fmt.Errorf("responses: %w", err)
	}
	if resp == nil {
		return nil, errors.New("missing response")
	}
	if strings.TrimSpace(resp.ID) == "" || isEmpty(strings.TrimSpace(string(resp.Model))) {
		return nil, errors.New("missing response metadata")
	}
	return resp, nil
}

func incompleteVerdict(resp *responses.Response) (Verdict, error) {
	if resp.Status != "incomplete" {
		return Verdict{}, errors.New("invalid response status")
	}
	// Truncation cannot approve anything, but it is still a review result.
	// Abstain so one stack cannot discard the whole assessment run.
	reason := "Model response was incomplete."
	if detail := strings.TrimSpace(resp.IncompleteDetails.Reason); detail != "" {
		reason = "Model response was incomplete: " + detail + "."
	}
	return Verdict{
		Decision:   "needs-human-review",
		Reason:     reason,
		Model:      string(resp.Model),
		ResponseID: resp.ID,
	}, nil
}

func consumeReviewOutput(resp *responses.Response, p *responses.ResponseNewParams, seen map[string]bool) (string, int, []responses.ResponseOutputItemUnion, error) {
	var text string
	count := 0
	var pending []responses.ResponseOutputItemUnion
	for _, item := range resp.Output {
		next, isMessage, err := applyReviewItem(item, p, seen, &pending)
		if err != nil {
			return "", 0, nil, err
		}
		if isMessage {
			text = next
			count++
		}
	}
	return text, count, pending, nil
}

func applyReviewItem(item responses.ResponseOutputItemUnion, p *responses.ResponseNewParams, seen map[string]bool, pending *[]responses.ResponseOutputItemUnion) (string, bool, error) {
	switch item.Type {
	case "reasoning":
		appendReasoning(p, item)
	case "function_call":
		if err := appendFunctionCall(p, seen, pending, item); err != nil {
			return "", false, err
		}
	case "message":
		text, err := messageText(item)
		return text, true, err
	default:
		return "", false, errors.New("unexpected response item")
	}
	return "", false, nil
}

func appendReasoning(p *responses.ResponseNewParams, item responses.ResponseOutputItemUnion) {
	param := item.AsReasoning().ToParam()
	p.Input.OfInputItemList = append(p.Input.OfInputItemList, responses.ResponseInputItemUnionParam{OfReasoning: &param})
}

func appendFunctionCall(p *responses.ResponseNewParams, seen map[string]bool, pending *[]responses.ResponseOutputItemUnion, item responses.ResponseOutputItemUnion) error {
	if item.Status != "completed" || isEmpty(strings.TrimSpace(item.CallID)) || isTrue(seen[item.CallID]) {
		return errors.New("invalid or duplicate tool call")
	}
	seen[item.CallID] = true
	*pending = append(*pending, item)
	call := item.AsFunctionCall().ToParam()
	p.Input.OfInputItemList = append(p.Input.OfInputItemList, responses.ResponseInputItemUnionParam{OfFunctionCall: &call})
	return nil
}

func messageText(item responses.ResponseOutputItemUnion) (string, error) {
	if item.Role != "assistant" ||
		!isSame(item.Status, "completed") ||
		!isSame(len(item.Content), 1) ||
		!isSame(item.Content[0].Type, "output_text") {
		return "", errors.New("invalid message")
	}
	return item.Content[0].Text, nil
}

func runReviewCalls(ctx context.Context, reader ContextReader, p *responses.ResponseNewParams, pending []responses.ResponseOutputItemUnion, calls, contextBytes *int, maxRequests, request int) error {
	if request+1 >= maxRequests {
		return errors.New("request budget exhausted")
	}
	*calls += len(pending)
	if *calls > 16 {
		return errors.New("tool call budget exhausted")
	}
	for _, call := range pending {
		if err := appendToolResult(ctx, reader, p, call, contextBytes); err != nil {
			return err
		}
	}
	return nil
}

func appendToolResult(ctx context.Context, reader ContextReader, p *responses.ResponseNewParams, call responses.ResponseOutputItemUnion, contextBytes *int) error {
	if reader == nil {
		return errors.New("missing context reader")
	}
	result, err := readTool(ctx, reader, call)
	if err != nil {
		return err
	}
	*contextBytes += len(result)
	if *contextBytes > MaxContextBytes {
		return errors.New("context budget exceeded")
	}
	output := responses.ResponseInputItemParamOfFunctionCallOutput(result)
	output.OfFunctionCallOutput.CallID = param.NewOpt(call.CallID)
	p.Input.OfInputItemList = append(p.Input.OfInputItemList, output)
	return nil
}

func decodeReviewVerdict(text string, resp *responses.Response, count int) (Verdict, error) {
	var zero Verdict
	if count != 1 {
		return zero, errors.New("expected one final message")
	}
	var result struct {
		Decision string    `json:"decision"`
		Reason   string    `json:"reason"`
		Evidence *[]string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(text), &result, json.RejectUnknownMembers(true)); err != nil {
		return zero, fmt.Errorf("decode verdict: %w", err)
	}
	if result.Evidence == nil {
		return zero, errors.New("missing evidence")
	}
	v := Verdict{
		Decision:   result.Decision,
		Reason:     result.Reason,
		Evidence:   *result.Evidence,
		Model:      string(resp.Model),
		ResponseID: resp.ID,
	}
	if err := v.Validate(); err != nil {
		return zero, fmt.Errorf("invalid verdict: %w", err)
	}
	return v, nil
}
