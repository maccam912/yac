// Package openai implements yac.Provider for OpenAI-compatible APIs
// (OpenAI, OpenRouter, Azure OpenAI, local servers, etc.).
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/maccam912/yac"
)

// DefaultBaseURL is the OpenAI API base.
const DefaultBaseURL = "https://api.openai.com/v1"

// DefaultEndpointPath is the endpoint used for chat-style completions.
const DefaultEndpointPath = "/chat/completions"

// Provider talks to any OpenAI-compatible chat completions endpoint.
type Provider struct {
	APIKey       string
	Model        string
	BaseURL      string       // defaults to DefaultBaseURL
	EndpointPath string       // defaults to DefaultEndpointPath
	Client       *http.Client // defaults to http.DefaultClient
}

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return DefaultBaseURL
}

func (p *Provider) endpointPath() string {
	if p.EndpointPath != "" {
		return p.EndpointPath
	}
	return DefaultEndpointPath
}

func (p *Provider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

// Complete sends a completion request and returns the first choice.
func (p *Provider) Complete(ctx context.Context, req *yac.Request) (*yac.Response, error) {
	body := chatRequest{
		Model:    p.Model,
		Messages: toAPIMsgs(req.Messages),
	}
	if len(req.Tools) > 0 {
		body.Tools = toAPITools(req.Tools)
	}
	if req.Config != nil {
		body.Temperature = req.Config.Temperature
		body.MaxTokens = req.Config.MaxTokens
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	url := strings.TrimRight(p.baseURL(), "/") + p.endpointPath()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	httpResp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: do request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: status %d: %s", httpResp.StatusCode, respBody)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	choice := chatResp.Choices[0]
	msg := yac.Message{
		Role:    yac.Role(choice.Message.Role),
		Content: choice.Message.Content,
	}
	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, yac.ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}

	return &yac.Response{
		Message: msg,
		Usage: yac.Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
		},
	}, nil
}

type chatRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	Tools       []apiTool    `json:"tools,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	MaxTokens   *int         `json:"max_tokens,omitempty"`
}

type apiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type apiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function apiToolFunction `json:"function"`
}

type apiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type apiTool struct {
	Type     string         `json:"type"`
	Function apiToolFuncDef `json:"function"`
}

type apiToolFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   apiUsage     `json:"usage"`
}

type chatChoice struct {
	Message apiMessage `json:"message"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func toAPIMsgs(msgs []yac.Message) []apiMessage {
	out := make([]apiMessage, len(msgs))
	for i, m := range msgs {
		am := apiMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolID,
		}
		for _, tc := range m.ToolCalls {
			am.ToolCalls = append(am.ToolCalls, apiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: apiToolFunction{
					Name:      tc.Name,
					Arguments: tc.Args,
				},
			})
		}
		out[i] = am
	}
	return out
}

func toAPITools(tools []yac.ToolDef) []apiTool {
	out := make([]apiTool, len(tools))
	for i, t := range tools {
		out[i] = apiTool{
			Type: "function",
			Function: apiToolFuncDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return out
}
