package quotas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	json "encoding/json/v2"
)

const (
	readLimit = 1024 * 1024
	readWait  = 15 * time.Second
	userAgent = "timber-quotas"
)

// NewHTTPClient refuses redirects so a bearer token never follows one to another host.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: readWait,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, wait time.Duration) (any, error) {
	if client == nil {
		client = NewHTTPClient()
	}
	if wait <= 0 {
		wait = readWait
	}
	ctx, cancel := context.WithTimeoutCause(ctx, wait, SaidError{Text: fmt.Sprintf("No answer in %d s.", int(wait.Seconds()))})
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, SaidError{Text: "It could not be reached: try again later."}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, requestFailure(ctx, err, wait)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		return nil, StatusError{Status: response.StatusCode, Text: answered(response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, readLimit+1))
	if err != nil {
		return nil, SaidError{Text: "It could not be reached: try again later."}
	}
	if len(body) > readLimit {
		return nil, SaidError{Text: "It answered with more than this plugin reads."}
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, SaidError{Text: "It answered with something this plugin could not read."}
	}
	return decoded, nil
}

func requestFailure(ctx context.Context, err error, wait time.Duration) error {
	if said, ok := errors.AsType[SaidError](context.Cause(ctx)); ok {
		return said
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return SaidError{Text: fmt.Sprintf("No answer in %d s.", int(wait.Seconds()))}
	}
	return SaidError{Text: "It could not be reached: try again later."}
}

func answered(status int) string {
	if status == http.StatusUnauthorized || forbidden(status) {
		return fmt.Sprintf("It refused the sign-in (%d): sign in again in Hermes.", status)
	}
	return fmt.Sprintf("It answered %d.", status)
}

func forbidden(status int) bool { return status == http.StatusForbidden }

func readFailure(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))
	}
	name := strings.TrimPrefix(fmt.Sprintf("%T", err), "*")
	return fmt.Sprintf("It could not be read (%s): try again later.", name)
}
