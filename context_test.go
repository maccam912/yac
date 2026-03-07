package yac

import (
	"context"
	"encoding/json"
	"testing"
)

func TestEstimateTokensEmpty(t *testing.T) {
	tokens := EstimateTokens(nil)
	if tokens != 0 {
		t.Errorf("expected 0 tokens for nil messages, got %d", tokens)
	}
}

func TestEstimateTokensSingleMessage(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello, world!"},
	}
	tokens := EstimateTokens(msgs)
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}

func TestEstimateTokensGrowsWithContent(t *testing.T) {
	short := []Message{{Role: "user", Content: "Hi"}}
	long := []Message{{Role: "user", Content: "This is a much longer message that contains many more characters and should result in a higher token estimate"}}

	shortTokens := EstimateTokens(short)
	longTokens := EstimateTokens(long)

	if longTokens <= shortTokens {
		t.Errorf("longer message should produce more tokens: short=%d, long=%d", shortTokens, longTokens)
	}
}

func TestEstimateToolTokens(t *testing.T) {
	tools := []*Tool{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a location",
			Parameters: Schema{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{"type": "string"},
				},
			},
		},
	}

	tokens := EstimateToolTokens(tools)
	if tokens <= 0 {
		t.Errorf("expected positive token count for tools, got %d", tokens)
	}
}

func TestEstimateToolTokensNil(t *testing.T) {
	tokens := EstimateToolTokens(nil)
	if tokens != 0 {
		t.Errorf("expected 0 tokens for nil tools, got %d", tokens)
	}
}

func TestTrimMessagesNoOpWhenUnderLimit(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello!"},
	}

	// Use a very generous limit.
	trimmed := trimMessages(msgs, 100000, 0)
	if len(trimmed) != len(msgs) {
		t.Errorf("expected %d messages (no trim), got %d", len(msgs), len(trimmed))
	}
}

func TestTrimMessagesZeroLimit(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hi"},
	}

	trimmed := trimMessages(msgs, 0, 0)
	if len(trimmed) != len(msgs) {
		t.Errorf("zero limit should return messages unchanged, got %d", len(trimmed))
	}
}

func TestTrimMessagesPreservesSystemAndLastMessage(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "Second question"},
		{Role: "assistant", Content: "Second answer"},
		{Role: "user", Content: "Third question"},
	}

	// Use a very small limit to force aggressive trimming.
	trimmed := trimMessages(msgs, 30, 0)

	// Should at least keep the system message and the last message.
	if len(trimmed) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(trimmed))
	}
	if trimmed[0].Role != "system" {
		t.Error("first message should be system")
	}
	if trimmed[len(trimmed)-1].Content != "Third question" {
		t.Errorf("last message should be preserved, got %q", trimmed[len(trimmed)-1].Content)
	}
}

func TestTrimMessagesRemovesToolClusters(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		// Tool call cluster.
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "greet", Arguments: `{"name":"Alice"}`}},
		}},
		{Role: "tool", Content: "Hello, Alice!", ToolCallID: "call_1"},
		// Regular exchange.
		{Role: "assistant", Content: "I greeted Alice."},
		{Role: "user", Content: "Thanks!"},
	}

	// Force trimming by using a small limit.
	trimmed := trimMessages(msgs, 40, 0)

	// The tool cluster (assistant+tool) should be removed together,
	// not leaving orphaned tool results.
	for i, m := range trimmed {
		if m.Role == "tool" {
			// If there's a tool result, the preceding message should be
			// an assistant with tool calls.
			if i == 0 || trimmed[i-1].Role != "assistant" || len(trimmed[i-1].ToolCalls) == 0 {
				t.Errorf("orphaned tool result at index %d: %+v", i, m)
			}
		}
	}
}

func TestSendWithContextLength(t *testing.T) {
	// Build up a long conversation history to ensure trimming kicks in.
	var msgs []Message
	for i := 0; i < 50; i++ {
		msgs = append(msgs, Message{Role: "user", Content: "A question that is moderately long to consume tokens in the context window."})
		msgs = append(msgs, Message{Role: "assistant", Content: "A response that is also moderately long to consume tokens in the context window."})
	}

	mock := &mockAdapter{
		responses: []Message{{Role: "assistant", Content: "Final answer"}},
	}

	agent := Agent{
		Adapter:       mock,
		ContextLength: 200, // small limit to force trimming
		Messages:      msgs,
	}

	reply, err := agent.Send(context.Background(), "One more question")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if reply.Content != "Final answer" {
		t.Errorf("got %q, want %q", reply.Content, "Final answer")
	}

	// The request sent to the adapter should have fewer messages than
	// the full history.
	req := mock.requests[0]
	if len(req.Messages) >= len(msgs)+1 {
		t.Errorf("expected trimmed messages (%d) to be less than full history (%d)",
			len(req.Messages), len(msgs)+1)
	}
}

func TestSendWithContextLengthToolCall(t *testing.T) {
	greetTool := &Tool{
		Name:       "greet",
		Parameters: Schema{"type": "object"},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "Hello!", nil
		},
	}

	// Build up history.
	var msgs []Message
	for i := 0; i < 20; i++ {
		msgs = append(msgs, Message{Role: "user", Content: "padding message to fill context"})
		msgs = append(msgs, Message{Role: "assistant", Content: "response padding to fill context"})
	}

	mock := &mockAdapter{
		responses: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "greet", Arguments: `{}`}},
			}},
			{Role: "assistant", Content: "Done!"},
		},
	}

	agent := Agent{
		Adapter:       mock,
		ContextLength: 200,
		Messages:      msgs,
		Tools:         []*Tool{greetTool},
	}

	reply, err := agent.Send(context.Background(), "Greet me")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if reply.Content != "Done!" {
		t.Errorf("got %q, want %q", reply.Content, "Done!")
	}
}

func TestStripToolClustersEmpty(t *testing.T) {
	result := StripToolClusters(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

func TestStripToolClustersNoTools(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "Be helpful."},
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello!"},
		{Role: "user", Content: "Bye"},
	}

	result := StripToolClusters(msgs)
	if len(result) != len(msgs) {
		t.Errorf("expected %d messages unchanged, got %d", len(msgs), len(result))
	}
}

func TestStripToolClustersRemovesCompletedCluster(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "Be helpful."},
		{Role: "user", Content: "Greet Alice"},
		// Completed tool cluster.
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "greet", Arguments: `{"name":"Alice"}`}},
		}},
		{Role: "tool", Content: "Hello, Alice!", ToolCallID: "call_1"},
		// Model continued after the tool result.
		{Role: "assistant", Content: "I greeted Alice for you!"},
		{Role: "user", Content: "Thanks"},
	}

	result := StripToolClusters(msgs)

	// The cluster (assistant+tool) should be stripped.
	// Remaining: system, user, assistant(final), user
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(result), result)
	}
	for _, m := range result {
		if m.Role == "tool" {
			t.Error("tool message should have been stripped")
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			t.Error("assistant tool-call message should have been stripped")
		}
	}
}

func TestStripToolClustersPreservesInProgressCluster(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Do something"},
		// In-progress cluster (no messages after the tool results).
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "do_thing", Arguments: `{}`}},
		}},
		{Role: "tool", Content: "Done!", ToolCallID: "call_1"},
	}

	result := StripToolClusters(msgs)

	// The cluster is still in progress (it's at the end), so keep it.
	if len(result) != len(msgs) {
		t.Errorf("expected %d messages (in-progress cluster preserved), got %d", len(msgs), len(result))
	}
}

func TestStripToolClustersMultipleClusters(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "First"},
		// Cluster 1 (completed).
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "tool_a", Arguments: `{}`}},
		}},
		{Role: "tool", Content: "Result A", ToolCallID: "call_1"},
		// Continuation.
		{Role: "assistant", Content: "Got result A."},
		{Role: "user", Content: "Second"},
		// Cluster 2 (completed).
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_2", Type: "function", Function: FunctionCall{Name: "tool_b", Arguments: `{}`}},
		}},
		{Role: "tool", Content: "Result B", ToolCallID: "call_2"},
		// Continuation.
		{Role: "assistant", Content: "Got result B."},
		{Role: "user", Content: "Done"},
	}

	result := StripToolClusters(msgs)

	// Both clusters stripped. Remaining: user, assistant, user, assistant, user = 5
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(result), result)
	}
	for _, m := range result {
		if m.Role == "tool" {
			t.Error("tool message should have been stripped")
		}
	}
}

func TestTrimMessagesPreservesTrailingToolCluster(t *testing.T) {
	// Reproduce the bug: when the only remaining conversation messages
	// are a trailing tool cluster (assistant+tool_calls, tool result),
	// trimMessages used to remove them entirely, leaving only the
	// system message. The model would then see no context at all.
	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Look up the latest news"},
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{
				Name:      "search",
				Arguments: `{"query":"latest news"}`,
			}},
		}},
		{Role: "tool", Content: "Search results: lots of news content here that is quite long...", ToolCallID: "call_1"},
	}

	// Use a tiny limit so trimming is forced.
	trimmed := trimMessages(msgs, 20, 0)

	// Must preserve: system, user question, tool cluster.
	hasToolResult := false
	hasToolCall := false
	hasUser := false
	for _, m := range trimmed {
		if m.Role == "tool" {
			hasToolResult = true
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			hasToolCall = true
		}
		if m.Role == "user" {
			hasUser = true
		}
	}
	if !hasToolResult {
		t.Error("trailing tool result was trimmed — model won't see tool output")
	}
	if !hasToolCall {
		t.Error("trailing tool-call assistant message was trimmed — orphaned tool result")
	}
	if !hasUser {
		t.Error("user message was trimmed — model won't know what question to answer")
	}
	if len(trimmed) != 4 {
		t.Errorf("expected 4 messages (system + user + tool cluster), got %d", len(trimmed))
	}
}

func TestTrimMessagesPreservesLastUserMessage(t *testing.T) {
	// When older history gets trimmed but a user question + tool exchange
	// remains, the user message must be kept even if over budget.
	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Old question"},
		{Role: "assistant", Content: "Old answer"},
		{Role: "user", Content: "Look up the latest news from iran"},
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{
				Name:      "search",
				Arguments: `{"query":"iran news"}`,
			}},
		}},
		{Role: "tool", Content: "Search results: many results here...", ToolCallID: "call_1"},
	}

	// Tiny limit forces trimming of older messages.
	trimmed := trimMessages(msgs, 30, 0)

	// Old Q&A should be trimmed, but user question + tool cluster preserved.
	roles := make([]string, len(trimmed))
	for i, m := range trimmed {
		roles[i] = m.Role
	}

	hasUser := false
	for _, m := range trimmed {
		if m.Role == "user" && m.Content == "Look up the latest news from iran" {
			hasUser = true
		}
	}
	if !hasUser {
		t.Errorf("last user message should be preserved, got roles: %v", roles)
	}
}

func TestSendWithAggressiveTrim(t *testing.T) {
	greetTool := &Tool{
		Name:       "greet",
		Parameters: Schema{"type": "object"},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "Hello!", nil
		},
	}

	// Seed history with a completed tool cluster.
	agent := Agent{
		Adapter: &mockAdapter{
			responses: []Message{{Role: "assistant", Content: "Sure thing!"}},
		},
		AggressiveTrim: true,
		Tools:          []*Tool{greetTool},
		Messages: []Message{
			{Role: "user", Content: "Greet Alice"},
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "greet", Arguments: `{"name":"Alice"}`}},
			}},
			{Role: "tool", Content: "Hello!", ToolCallID: "call_1"},
			{Role: "assistant", Content: "I greeted Alice."},
		},
	}

	mock := agent.Adapter.(*mockAdapter)
	_, err := agent.Send(context.Background(), "Do it again")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// The request should NOT contain the old tool cluster.
	req := mock.requests[0]
	for _, m := range req.Messages {
		if m.Role == "tool" {
			t.Error("aggressive trim should have removed tool messages from the request")
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			t.Error("aggressive trim should have removed tool-call assistant messages from the request")
		}
	}
}
