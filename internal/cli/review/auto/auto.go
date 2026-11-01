// Package auto evaluates refactor stacks under delegated approval policy.
package auto

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/review/display"
	coreconfig "github.com/nnutter/timber/internal/timber/config"
	"github.com/nnutter/timber/internal/timber/review"
	"github.com/nnutter/timber/internal/timber/review/autoreview"
)

// Command owns only CLI configuration; the review service owns approval.
type Command struct {
	Service     review.Service
	Output      io.Writer
	LoadModels  func() (coreconfig.Models, error)
	Model       string
	MaxRequests int
	Options     review.AutoOptions
}

// RunE resolves the model and its provider before sending committed source.
func (c *Command) RunE(cmd *cobra.Command, _ []string) error {
	if c.Options.Timeout <= 0 || c.MaxRequests <= 0 {
		return fmt.Errorf("--timeout and --max-requests must be positive")
	}
	models, err := c.models()
	if err != nil {
		return err
	}
	selector, err := selectModel(models, c.Model)
	if err != nil {
		return err
	}
	resolved := coreconfig.Resolved{}
	if len(models.Providers) > 0 {
		resolved, err = models.Resolve(selector)
		if err != nil {
			return err
		}
		if err := resolved.Validate(); err != nil {
			return err
		}
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		if apiKey, err = coreconfig.Value(resolved.APIKey); err != nil {
			return fmt.Errorf("api key: %w", err)
		}
	}
	if apiKey == "" {
		return fmt.Errorf("auto requires OPENAI_API_KEY or a provider apiKey in the models file")
	}
	headers, err := coreconfig.Values(resolved.Headers)
	if err != nil {
		return err
	}
	client := openai.NewClient(clientOptions(apiKey, cmp.Or(os.Getenv("OPENAI_BASE_URL"), resolved.BaseURL), headers)...)
	evaluator := autoreview.OpenAI{
		Client:          client,
		Model:           cmp.Or(resolved.Model, selector),
		MaxRequests:     c.MaxRequests,
		MaxOutputTokens: resolved.MaxTokens,
	}
	// A stack assessment takes long enough that silence looks like a hang.
	progress := cmd.ErrOrStderr()
	c.Options.Progress = func(p review.AutoProgress) {
		// Progress is best effort; a failed write must not stop the run.
		_, _ = fmt.Fprintf(progress, "Stack %d/%d: assessing %s\n", p.Index, p.Total, display.ShortStack(p.Stack))
	}
	result, err := c.Service.Auto(cmd.Context(), evaluator, c.Options)
	if err != nil {
		return err
	}
	asJSON := false
	if flag, err := cmd.Flags().GetBool("json"); err == nil {
		asJSON = flag
	}
	return display.WriteAuto(c.Output, asJSON, result)
}

func (c *Command) models() (coreconfig.Models, error) {
	if c.LoadModels == nil {
		return coreconfig.Models{}, nil
	}
	return c.LoadModels()
}

// New builds a noninteractive automatic refactor assessment command.
func New(svc review.Service, out io.Writer, loadModels func() (coreconfig.Models, error)) *cobra.Command {
	c := &Command{
		Service:    svc,
		Output:     out,
		LoadModels: loadModels,
	}
	cmd := &cobra.Command{
		Use: "auto", Short: "Assess Refactor: stacks with a model and approve qualifying changes",
		Long: "Delegate approval of behavior-preserving Refactor: stacks to an OpenAI model.\nReads $XDG_CONFIG_HOME/timber/model.json for the endpoint, key, headers, and output budget of a model.\nUses committed content only. Other changes and uncertain assessments remain unapproved.\nUse --dry-run to assess without recording approval. Source context is sent to the API.",
		Args: cobra.NoArgs, RunE: c.RunE,
	}
	cmd.Flags().StringVar(&c.Model, "model", "", "Model or provider/model from the models file (or set TIMBER_REVIEW_MODEL)")
	cmd.Flags().BoolVar(&c.Options.DryRun, "dry-run", false, "Assess without changing review approval state")
	cmd.Flags().StringVar(&c.Options.Start, "start", "", "First commit to assess, including its complete stack")
	cmd.Flags().StringVar(&c.Options.End, "end", "", "Last commit to assess, including its complete stack")
	cmd.Flags().DurationVar(&c.Options.Timeout, "timeout", 3*time.Minute, "Time limit for the entire automatic review run")
	cmd.Flags().IntVar(&c.MaxRequests, "max-requests", autoreview.DefaultMaxRequests, "Maximum model requests per stack (SDK retries disabled)")
	return cmd
}

// Install adds the automatic assessment command to parent.
func Install(parent *cobra.Command, svc review.Service, out io.Writer, loadModels func() (coreconfig.Models, error)) {
	parent.AddCommand(New(svc, out, loadModels))
}

// selectModel prefers the flag, then the environment, then the only model the
// file describes.
func selectModel(models coreconfig.Models, flag string) (string, error) {
	if selector := cmp.Or(flag, os.Getenv("TIMBER_REVIEW_MODEL")); selector != "" {
		return selector, nil
	}
	if only, ok := models.Only(); ok {
		return only, nil
	}
	if models.Count() == 0 {
		return "", fmt.Errorf("auto requires --model, TIMBER_REVIEW_MODEL, or a models file that describes a model")
	}
	return "", fmt.Errorf("auto requires --model or TIMBER_REVIEW_MODEL: the models file describes %d models", models.Count())
}

// clientOptions applies the provider the model resolved to. The SDK reads other
// provider settings from the environment when an option is absent.
func clientOptions(apiKey, baseURL string, headers map[string]string) []option.RequestOption {
	options := []option.RequestOption{option.WithMaxRetries(0), option.WithAPIKey(apiKey)}
	if baseURL != "" {
		options = append(options, option.WithBaseURL(baseURL))
	}
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		options = append(options, option.WithHeader(name, headers[name]))
	}
	return options
}
