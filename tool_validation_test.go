package yac_test

import (
	"testing"

	"github.com/maccam912/yac"
)

func TestValidateToolCall(t *testing.T) {
	def := yac.ToolDef{
		Name:       "run_python",
		Parameters: []byte(`{"type":"object","properties":{"code":{"type":"string"}},"required":["code"]}`),
	}

	if err := yac.ValidateToolCall(yac.ToolCall{Name: "run_python", Args: `{"code":"print(1)"}`}, def); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := yac.ValidateToolCall(yac.ToolCall{Name: "run_python", Args: `{"code":1}`}, def); err == nil {
		t.Fatal("expected type validation error")
	}
}
