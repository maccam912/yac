package yac

// Role identifies who authored a message in a conversation.
type Role string

const (
	System    Role = "system"
	User      Role = "user"
	Assistant Role = "assistant"
	Tool      Role = "tool"
)

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID   string // unique ID assigned by the provider
	Name string // tool function name
	Args string // raw JSON arguments
}

// Message is a single entry in a conversation.
type Message struct {
	Role      Role
	Content   string
	ToolCalls []ToolCall // non-nil only on assistant messages requesting tool use
	ToolID    string     // set only on tool-result messages, references ToolCall.ID
}
