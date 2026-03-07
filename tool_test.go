package yac

import (
	"context"
	"encoding/json"
	"testing"
)

func TestToolExecute(t *testing.T) {
	tool := &Tool{
		Name:        "greet",
		Description: "Greet someone by name",
		Parameters: Schema{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []string{"name"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Name string `json:"name"`
			}
			json.Unmarshal(args, &p)
			return "Hello, " + p.Name + "!", nil
		},
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"Alice"}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "Hello, Alice!" {
		t.Errorf("got %q, want %q", result, "Hello, Alice!")
	}
}

func TestSchemaMarshaling(t *testing.T) {
	s := Schema{
		"type": "object",
		"properties": map[string]any{
			"location": map[string]any{"type": "string", "description": "City"},
		},
		"required": []string{"location"},
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded map[string]any
	json.Unmarshal(data, &decoded)
	if decoded["type"] != "object" {
		t.Errorf("expected type 'object', got %v", decoded["type"])
	}
}

func TestWithTools(t *testing.T) {
	a := &Tool{Name: "a"}
	b := &Tool{Name: "b"}

	cfg := &sendConfig{}
	WithTools(a, b)(cfg)

	if len(cfg.tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(cfg.tools))
	}
	if cfg.tools[0].Name != "a" || cfg.tools[1].Name != "b" {
		t.Error("tools not set correctly")
	}
}

func TestWithToolChoice(t *testing.T) {
	cfg := &sendConfig{}
	WithToolChoice(Required)(cfg)

	if cfg.toolChoice == nil || *cfg.toolChoice != Required {
		t.Error("toolChoice not set to Required")
	}
}

func TestForceToolUseOption(t *testing.T) {
	cfg := &sendConfig{}
	ForceToolUse("get_weather")(cfg)

	if cfg.forceTool != "get_weather" {
		t.Errorf("expected 'get_weather', got %q", cfg.forceTool)
	}
}

func TestResolveToolChoiceAuto(t *testing.T) {
	tc := Auto
	result := resolveToolChoice(&sendConfig{toolChoice: &tc})
	if result != "auto" {
		t.Errorf("expected 'auto', got %v", result)
	}
}

func TestResolveToolChoiceNone(t *testing.T) {
	tc := None
	result := resolveToolChoice(&sendConfig{toolChoice: &tc})
	if result != "none" {
		t.Errorf("expected 'none', got %v", result)
	}
}

func TestResolveToolChoiceRequired(t *testing.T) {
	tc := Required
	result := resolveToolChoice(&sendConfig{toolChoice: &tc})
	if result != "required" {
		t.Errorf("expected 'required', got %v", result)
	}
}

func TestResolveToolChoiceForce(t *testing.T) {
	result := resolveToolChoice(&sendConfig{forceTool: "weather"})
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["type"] != "function" {
		t.Errorf("expected type 'function', got %v", m["type"])
	}
	fn := m["function"].(map[string]string)
	if fn["name"] != "weather" {
		t.Errorf("expected 'weather', got %q", fn["name"])
	}
}

func TestResolveToolChoiceDefault(t *testing.T) {
	result := resolveToolChoice(&sendConfig{})
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestFindTool(t *testing.T) {
	tools := []*Tool{{Name: "a"}, {Name: "b"}, {Name: "c"}}

	if f := findTool(tools, "b"); f == nil || f.Name != "b" {
		t.Error("expected to find tool 'b'")
	}
	if f := findTool(tools, "z"); f != nil {
		t.Error("expected nil for unknown tool")
	}
}
