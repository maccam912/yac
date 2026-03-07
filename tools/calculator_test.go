package tools

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

// --- Eval tests (expression evaluator) ---

func TestEvalBasicArithmetic(t *testing.T) {
	tests := []struct {
		expr string
		want float64
	}{
		{"1 + 2", 3},
		{"10 - 3", 7},
		{"4 * 5", 20},
		{"20 / 4", 5},
		{"2 + 3 * 4", 14},   // precedence: * before +
		{"(2 + 3) * 4", 20}, // parentheses override
		{"10 / 2 + 3", 8},
		{"10 / (2 + 3)", 2},
		{"1 + 2 + 3 + 4", 10},
		{"100", 100},
		{"3.14", 3.14},
	}

	for _, tt := range tests {
		got, err := Eval(tt.expr)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", tt.expr, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestEvalPower(t *testing.T) {
	tests := []struct {
		expr string
		want float64
	}{
		{"2^10", 1024},
		{"3^2", 9},
		{"2^3^2", 512}, // right-associative: 2^(3^2) = 2^9 = 512
	}

	for _, tt := range tests {
		got, err := Eval(tt.expr)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", tt.expr, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestEvalUnary(t *testing.T) {
	tests := []struct {
		expr string
		want float64
	}{
		{"-5", -5},
		{"+5", 5},
		{"--5", 5}, // double negative
		{"-(-3)", 3},
		{"-(2 + 3)", -5},
	}

	for _, tt := range tests {
		got, err := Eval(tt.expr)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", tt.expr, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestEvalFunctions(t *testing.T) {
	tests := []struct {
		expr string
		want float64
	}{
		{"sqrt(144)", 12},
		{"sqrt(2)", math.Sqrt(2)},
		{"abs(-42)", 42},
		{"abs(42)", 42},
		{"floor(3.7)", 3},
		{"ceil(3.2)", 4},
		{"round(3.5)", 4},
		{"log10(100)", 2},
		{"sin(0)", 0},
		{"cos(0)", 1},
	}

	for _, tt := range tests {
		got, err := Eval(tt.expr)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", tt.expr, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestEvalConstants(t *testing.T) {
	got, err := Eval("pi")
	if err != nil {
		t.Fatalf("Eval(pi) error: %v", err)
	}
	if math.Abs(got-math.Pi) > 1e-9 {
		t.Errorf("Eval(pi) = %v, want %v", got, math.Pi)
	}

	got, err = Eval("e")
	if err != nil {
		t.Fatalf("Eval(e) error: %v", err)
	}
	if math.Abs(got-math.E) > 1e-9 {
		t.Errorf("Eval(e) = %v, want %v", got, math.E)
	}
}

func TestEvalComplex(t *testing.T) {
	tests := []struct {
		expr string
		want float64
	}{
		{"sqrt(3^2 + 4^2)", 5},                        // pythagorean theorem
		{"2 * pi", 2 * math.Pi},                       // tau
		{"(1 + sqrt(5)) / 2", (1 + math.Sqrt(5)) / 2}, // golden ratio
	}

	for _, tt := range tests {
		got, err := Eval(tt.expr)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", tt.expr, err)
			continue
		}
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Eval(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestEvalErrors(t *testing.T) {
	errCases := []string{
		"",
		"1 / 0",
		"1 +",
		"(1 + 2",
		"foo(1)",
		"1 2",
	}

	for _, expr := range errCases {
		_, err := Eval(expr)
		if err == nil {
			t.Errorf("Eval(%q) expected error, got nil", expr)
		}
	}
}

// --- Calculator tool tests ---

func TestCalculatorTool(t *testing.T) {
	calc := Calculator()

	if calc.Name != "calculator" {
		t.Errorf("expected name 'calculator', got %q", calc.Name)
	}
	if calc.Description == "" {
		t.Error("expected non-empty description")
	}

	// Test Execute with a simple expression.
	result, err := calc.Execute(context.Background(), json.RawMessage(`{"expression":"(3 + 4) * 2"}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "14" {
		t.Errorf("got %q, want %q", result, "14")
	}
}

func TestCalculatorToolDecimal(t *testing.T) {
	calc := Calculator()

	result, err := calc.Execute(context.Background(), json.RawMessage(`{"expression":"10 / 3"}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "3.3333333333333335" {
		t.Errorf("got %q", result)
	}
}

func TestCalculatorToolEmptyExpression(t *testing.T) {
	calc := Calculator()

	_, err := calc.Execute(context.Background(), json.RawMessage(`{"expression":""}`))
	if err == nil {
		t.Error("expected error for empty expression")
	}
}

func TestCalculatorToolBadJSON(t *testing.T) {
	calc := Calculator()

	_, err := calc.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}
