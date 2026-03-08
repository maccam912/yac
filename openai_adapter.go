package yac

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OpenAIAdapter connects to OpenAI or any OpenAI-compatible API
// (e.g. OpenRouter, Ollama, Azure OpenAI, vLLM, etc.).
type OpenAIAdapter struct {
	// APIKey is the bearer token for authentication.
	APIKey string

	// BaseURL is the API base (e.g. "https://api.openai.com/v1").
	// Swap this out for compatible providers.
	BaseURL string

	// Model is the model identifier sent with every request
	// (e.g. "gpt-4o", "deepseek-chat").
	Model string

	// OrgID is an optional organization ID sent via the
	// OpenAI-Organization header. Leave empty if not needed.
	OrgID string
}

// --- API request/response types (OpenAI-specific, not exported) ---

type apiToolDef struct {
	Type     string          `json:"type"`
	Function apiToolFunction `json:"function"`
}

type apiToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  Schema `json:"parameters"`
}

type chatRequest struct {
	Model      string       `json:"model"`
	Messages   []Message    `json:"messages"`
	Tools      []apiToolDef `json:"tools,omitempty"`
	ToolChoice any          `json:"tool_choice,omitempty"`
}

type chatResponse struct {
	Choices    []chatChoice `json:"choices"`
	Completion string       `json:"completion,omitempty"`
	Reasoning  string       `json:"reasoning,omitempty"`
	Error      *apiError    `json:"error,omitempty"`
}

type chatChoice struct {
	Message Message `json:"message"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// --- Adapter implementation ---

// SendMessage implements the Adapter interface. It posts the conversation
// to the chat completions endpoint and returns the model's reply.
func (a *OpenAIAdapter) SendMessage(ctx context.Context, req *ChatRequest) (Message, error) {
	ctx, span := tracer.Start(ctx, "adapter.send",
		trace.WithAttributes(
			attribute.String("yac.adapter.model", a.Model),
			attribute.Int("yac.adapter.message_count", len(req.Messages)),
			attribute.Int("yac.adapter.tool_count", len(req.Tools)),
		),
	)
	defer span.End()

	// Convert Tool definitions to API format.
	var apiTools []apiToolDef
	for _, t := range req.Tools {
		apiTools = append(apiTools, apiToolDef{
			Type: "function",
			Function: apiToolFunction{
				Name:        t.Name,
				Description: t.GetDescription(),
				Parameters:  t.Parameters,
			},
		})
	}

	apiReq := chatRequest{
		Model:      a.Model,
		Messages:   req.Messages,
		Tools:      apiTools,
		ToolChoice: req.ToolChoice,
	}

	payload, err := json.Marshal(apiReq)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Message{}, fmt.Errorf("marshal request: %w", err)
	}

	url := a.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Message{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.APIKey)
	if a.OrgID != "" {
		httpReq.Header.Set("OpenAI-Organization", a.OrgID)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Message{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Message{}, fmt.Errorf("read response: %w", err)
	}

	span.SetAttributes(
		attribute.Int("yac.adapter.http_status", resp.StatusCode),
		attribute.Int("yac.adapter.response_bytes", len(body)),
	)

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(body))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Message{}, err
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Message{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		err := fmt.Errorf("API error: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Message{}, err
	}

	if len(chatResp.Choices) == 0 {
		// Some OpenAI-compatible gateways return top-level completion
		// fields instead of choices[]. Prefer completion and fall back to
		// reasoning if completion is blank.
		content := chatResp.Completion
		if content == "" {
			content = chatResp.Reasoning
		}
		if content == "" {
			err := fmt.Errorf("no choices in response")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return Message{}, err
		}
		return Message{Role: "assistant", Content: content}, nil
	}

	msg := chatResp.Choices[0].Message
	span.SetAttributes(
		attribute.Int("yac.adapter.tool_calls", len(msg.ToolCalls)),
		attribute.String("yac.adapter.reply_preview", truncate(msg.Content, 100)),
	)

	return msg, nil
}
