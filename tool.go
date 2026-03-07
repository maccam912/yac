package yac

import (
	"context"
	"encoding/json"
)

// Schema is a JSON Schema object describing tool parameters.
// Use standard JSON Schema keys like "type", "properties", "required".
//
// Example:
//
//	yac.Schema{
//	    "type": "object",
//	    "properties": map[string]any{
//	        "location": map[string]any{
//	            "type":        "string",
//	            "description": "City name, e.g. 'San Francisco, CA'",
//	        },
//	    },
//	    "required": []string{"location"},
//	}
type Schema map[string]any

// Tool defines a function that an agent can call during a conversation.
//
// Name and Description are sent to the model so it knows when and how
// to use the tool. Parameters is a JSON Schema describing the expected
// arguments. Execute is the Go function that runs when the model
// invokes the tool.
//
// Example:
//
//	weatherTool := &yac.Tool{
//	    Name:        "get_weather",
//	    Description: "Get the current weather for a location",
//	    Parameters: yac.Schema{
//	        "type": "object",
//	        "properties": map[string]any{
//	            "location": map[string]any{
//	                "type":        "string",
//	                "description": "City name",
//	            },
//	        },
//	        "required": []string{"location"},
//	    },
//	    Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
//	        var p struct{ Location string `json:"location"` }
//	        json.Unmarshal(args, &p)
//	        return fmt.Sprintf("72°F in %s", p.Location), nil
//	    },
//	}
type Tool struct {
	// Name is the function name exposed to the model (e.g. "get_weather").
	Name string

	// Description explains what the tool does. The model reads this to
	// decide when to use the tool, so be clear and specific.
	Description string

	// Parameters is a JSON Schema object describing the tool's arguments.
	Parameters Schema

	// Execute is called when the model invokes this tool. It receives
	// the raw JSON arguments and returns the result as a string.
	// The context supports cancellation and timeouts.
	Execute func(ctx context.Context, args json.RawMessage) (string, error)
}

// ToolChoice controls whether and how the model uses tools on a given turn.
type ToolChoice int

const (
	// Auto lets the model decide whether to use tools (default behavior).
	Auto ToolChoice = iota

	// None disables tool use for this turn, even if tools are available.
	None

	// Required forces the model to call at least one tool.
	Required
)

// sendConfig holds per-turn settings collected from SendOption functions.
type sendConfig struct {
	tools      []*Tool
	toolChoice *ToolChoice
	forceTool  string
}

// SendOption is a functional option that configures a single Agent.Send call.
type SendOption func(*sendConfig)

// WithTools overrides the agent's default tools for this turn.
// Only the specified tools will be available to the model.
//
//	reply, err := agent.Send(ctx, "Look this up", yac.WithTools(searchTool))
func WithTools(tools ...*Tool) SendOption {
	return func(c *sendConfig) {
		c.tools = tools
	}
}

// WithToolChoice sets the tool choice mode for this turn.
//
//	reply, err := agent.Send(ctx, "Just chat", yac.WithToolChoice(yac.None))
//	reply, err := agent.Send(ctx, "Use a tool", yac.WithToolChoice(yac.Required))
func WithToolChoice(choice ToolChoice) SendOption {
	return func(c *sendConfig) {
		c.toolChoice = &choice
	}
}

// ForceToolUse forces the model to call a specific tool by name.
//
//	reply, err := agent.Send(ctx, "Check weather", yac.ForceToolUse("get_weather"))
func ForceToolUse(name string) SendOption {
	return func(c *sendConfig) {
		c.forceTool = name
	}
}

// resolveToolChoice converts sendConfig into the API tool_choice value.
// Returns nil if no explicit choice was made (API default is "auto").
func resolveToolChoice(cfg *sendConfig) any {
	if cfg.forceTool != "" {
		return map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": cfg.forceTool,
			},
		}
	}
	if cfg.toolChoice != nil {
		switch *cfg.toolChoice {
		case Auto:
			return "auto"
		case None:
			return "none"
		case Required:
			return "required"
		}
	}
	return nil
}

// findTool looks up a tool by name in a slice.
func findTool(tools []*Tool, name string) *Tool {
	for _, t := range tools {
		if t.Name == name {
			return t
		}
	}
	return nil
}
