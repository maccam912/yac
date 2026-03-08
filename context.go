package yac

import "encoding/json"

// charsPerToken is a rough approximation used for token estimation.
// Most LLM tokenizers average ~4 characters per token for English text.
// This is deliberately conservative (lower = more aggressive estimation)
// to reduce the chance of exceeding the actual limit.
const charsPerToken = 4

// EstimateTokens returns an approximate token count for a slice of
// messages. The estimate is based on character length divided by a
// constant factor. It's intentionally rough — a real tokenizer (like
// tiktoken) can be swapped in later without changing the API.
func EstimateTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		// Role + content.
		total += len(m.Role) + len(m.Content)

		// Tool calls add significant tokens (function name + args JSON).
		for _, tc := range m.ToolCalls {
			total += len(tc.ID) + len(tc.Type)
			total += len(tc.Function.Name) + len(tc.Function.Arguments)
		}

		// ToolCallID on tool-result messages.
		total += len(m.ToolCallID)

		// Overhead per message for role markers, JSON framing, etc.
		// The OpenAI tokenizer adds ~4 tokens per message for framing.
		total += charsPerToken * 4
	}

	return total / charsPerToken
}

// EstimateToolTokens returns an approximate token count for tool
// definitions that will be included in the request.
func EstimateToolTokens(tools []*Tool) int {
	total := 0
	for _, t := range tools {
		total += len(t.Name) + len(t.GetDescription())
		if t.Parameters != nil {
			b, _ := json.Marshal(t.Parameters)
			total += len(b)
		}
		// Overhead for the tool definition structure.
		total += charsPerToken * 4
	}
	return total / charsPerToken
}

// trimMessages removes the oldest non-system messages from the
// conversation to bring the estimated token count at or below maxTokens.
//
// The function preserves:
//   - The system message (if present, always index 0 with role "system")
//   - The most recent user message (last message is always kept)
//   - Tool call/result pairs — if a tool result is kept, the assistant
//     message that triggered it is also kept (and vice versa)
//
// If maxTokens is 0 or negative, the messages are returned unchanged.
// The toolTokens parameter accounts for token budget consumed by tool
// definitions in the request.
func trimMessages(messages []Message, maxTokens int, toolTokens int) []Message {
	if maxTokens <= 0 {
		return messages
	}

	budget := maxTokens - toolTokens
	if budget <= 0 {
		// Even without messages we'd exceed the limit — nothing we can
		// do here, just return the messages and let the API error.
		return messages
	}

	if EstimateTokens(messages) <= budget {
		return messages
	}

	// Separate the system message (if present) from the rest.
	var system []Message
	conversation := messages
	if len(messages) > 0 && messages[0].Role == "system" {
		system = messages[:1]
		conversation = messages[1:]
	}

	// We always keep the system message and at least the last message.
	// Trim from the front of the conversation slice.
	for len(conversation) > 1 {
		candidate := make([]Message, 0, len(system)+len(conversation))
		candidate = append(candidate, system...)
		candidate = append(candidate, conversation...)

		if EstimateTokens(candidate) <= budget {
			return candidate
		}

		// Remove the oldest message. If it's part of a tool-call
		// cluster (assistant with tool_calls followed by tool results),
		// remove the entire cluster.
		removed := 1
		if conversation[0].Role == "assistant" && len(conversation[0].ToolCalls) > 0 {
			// Count subsequent tool results.
			for removed < len(conversation) && conversation[removed].Role == "tool" {
				removed++
			}
		}

		// Never remove more messages than would leave the
		// conversation empty — the trailing cluster (the tool
		// exchange we're still processing) must be preserved.
		if removed >= len(conversation) {
			break
		}

		// Don't remove the last remaining user message — without
		// it the model loses the question it's trying to answer.
		if conversation[0].Role == "user" {
			hasOtherUser := false
			for j := removed; j < len(conversation); j++ {
				if conversation[j].Role == "user" {
					hasOtherUser = true
					break
				}
			}
			if !hasOtherUser {
				break
			}
		}

		conversation = conversation[removed:]
	}

	// Return whatever is left (system + last message).
	result := make([]Message, 0, len(system)+len(conversation))
	result = append(result, system...)
	result = append(result, conversation...)
	return result
}

// StripToolClusters removes completed tool-call exchanges from the
// message history. A "cluster" is an assistant message with ToolCalls
// followed by one or more tool-result messages. Only clusters that are
// fully resolved are removed — if the last message is part of an
// in-progress cluster (i.e. we're mid-tool-loop), it's preserved.
//
// This is used automatically by AggressiveTrim, but is also exported
// so callers can preview the effect or use it independently.
func StripToolClusters(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	result := make([]Message, 0, len(messages))

	i := 0
	for i < len(messages) {
		m := messages[i]

		// Check if this is the start of a tool cluster.
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// Count how many tool results follow.
			clusterEnd := i + 1
			for clusterEnd < len(messages) && messages[clusterEnd].Role == "tool" {
				clusterEnd++
			}

			// Only strip if the cluster is "complete" — there are
			// messages after it (meaning the model already processed
			// the tool results and moved on).
			if clusterEnd < len(messages) {
				// Skip the entire cluster.
				i = clusterEnd
				continue
			}
		}

		result = append(result, m)
		i++
	}

	return result
}
