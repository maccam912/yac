package yac

// Message represents a single message in a conversation.
type Message struct {
	// Role is one of "system", "user", "assistant", or "tool".
	Role string `json:"role"`

	// Content is the text content of the message.
	// May be empty on assistant messages that contain only tool calls.
	Content string `json:"content,omitempty"`

	// ToolCalls is populated when the assistant wants to invoke tools.
	// Only present on messages with Role "assistant".
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID identifies which tool call this message is responding to.
	// Only present on messages with Role "tool".
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall represents a single tool invocation requested by the model.
type ToolCall struct {
	// ID is a unique identifier for this call, assigned by the model.
	// Used to match tool results back to their corresponding calls.
	ID string `json:"id"`

	// Type is always "function" in the current OpenAI API.
	Type string `json:"type"`

	// Function contains the function name and serialized arguments.
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the name and arguments of a function the model
// wants to execute.
type FunctionCall struct {
	// Name is the tool name the model wants to call.
	Name string `json:"name"`

	// Arguments is a JSON string of the arguments. Must be parsed by
	// the tool's Execute function (e.g. via json.Unmarshal).
	Arguments string `json:"arguments"`
}
