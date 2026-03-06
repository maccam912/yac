package yac

import (
	"encoding/json"
	"fmt"
)

// ValidateToolCall validates a tool call payload against a minimal subset of
// JSON Schema supported by ToolDef.Parameters.
func ValidateToolCall(call ToolCall, def ToolDef) error {
	if def.Name == "" {
		return fmt.Errorf("unknown tool: %s", call.Name)
	}
	if len(def.Parameters) == 0 {
		return nil
	}

	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	if schema.Type != "" && schema.Type != "object" {
		return fmt.Errorf("unsupported top-level schema type %q", schema.Type)
	}

	var args map[string]json.RawMessage
	if err := json.Unmarshal([]byte(call.Args), &args); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	for _, name := range schema.Required {
		if _, ok := args[name]; !ok {
			return fmt.Errorf("missing required argument %q", name)
		}
	}

	for name, raw := range args {
		prop, ok := schema.Properties[name]
		if !ok {
			continue
		}
		var propSchema struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(prop, &propSchema); err != nil {
			return fmt.Errorf("invalid property schema for %q: %w", name, err)
		}
		if propSchema.Type == "" {
			continue
		}
		if err := validateType(name, raw, propSchema.Type); err != nil {
			return err
		}
	}

	return nil
}

func validateType(name string, raw json.RawMessage, want string) error {
	switch want {
	case "string":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("argument %q must be a string", name)
		}
	case "number":
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("argument %q must be a number", name)
		}
	case "integer":
		var v int64
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("argument %q must be an integer", name)
		}
	case "boolean":
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("argument %q must be a boolean", name)
		}
	case "object":
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("argument %q must be an object", name)
		}
	case "array":
		var v []any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("argument %q must be an array", name)
		}
	}
	return nil
}
