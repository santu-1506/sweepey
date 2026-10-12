// Package ai adds optional Claude-powered judgement on top of sweepr's
// heuristics. It only ever sees file paths, names and sizes (home directory
// shortened to ~), never file contents, and it never deletes anything: its
// verdicts change which items are pre-selected, and the user still confirms.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

const defaultModel = "claude-opus-5-5"

// Models that accept server-side refusal fallbacks in "default" mode.
var fallbackModels = map[string]bool{
	"claude-opus-5-5": true, "claude-opus-5": true, "claude-fable-5-1": true, "claude-sonnet-5-5": true,
}

type Client struct {
	c     anthropic.Client
	model string
}

// New builds a client. Credentials come from ANTHROPIC_API_KEY or an
// `ant auth login` profile; SWEEPR_AI_MODEL overrides the model.
func New() *Client {
	model := os.Getenv("SWEEPR_AI_MODEL")
	if model == "" {
		model = defaultModel
	}
	return &Client{c: anthropic.NewClient(), model: model}
}

func (c *Client) Model() string { return c.model }

// structured sends one request and decodes the JSON reply into out. The
// response format is enforced by the API's structured outputs.
func (c *Client) structured(ctx context.Context, system, user string, schema map[string]any,
	effort anthropic.BetaOutputConfigEffort, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user)),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: effort,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
	}
	if fallbackModels[c.model] {
		// If a safety classifier declines, Anthropic re-runs the request on a
		// suitable fallback model instead of returning a refusal.
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
	}

	resp, err := c.c.Beta.Messages.New(ctx, params)
	if err != nil {
		return explain(err)
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return errors.New("the model declined this request")
	case anthropic.BetaStopReasonMaxTokens:
		return errors.New("the response was cut off (too many items); try a smaller selection")
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	if err := json.Unmarshal([]byte(text.String()), out); err != nil {
		return fmt.Errorf("unexpected AI response: %w", err)
	}
	return nil
}

func explain(err error) error {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case 401, 403:
			return errors.New("not authorized: set ANTHROPIC_API_KEY (or run `ant auth login`) to use AI features")
		case 404:
			return fmt.Errorf("model not found; check SWEEPR_AI_MODEL")
		case 429:
			return errors.New("rate limited by the API; try again in a minute")
		case 529, 500, 502, 503:
			return errors.New("the AI service is temporarily unavailable; try again shortly")
		}
		return fmt.Errorf("AI request failed (HTTP %d)", apierr.StatusCode)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("the AI request timed out")
	}
	if strings.Contains(err.Error(), "API key") || strings.Contains(err.Error(), "credentials") {
		return errors.New("no credentials: set ANTHROPIC_API_KEY (or run `ant auth login`) to use AI features")
	}
	return err
}

// Small helpers for writing JSON schemas inline.
func obj(props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}
func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func enum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "enum": vals, "description": desc}
}
func num(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func arr(items map[string]any) map[string]any { return map[string]any{"type": "array", "items": items} }
