package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/maccam912/yac"
)

// Calculator returns a tool that evaluates mathematical expressions.
//
// Supported operations: +, -, *, /, ^ (power), parentheses, and
// common functions (sqrt, abs, sin, cos, tan, log, log10).
//
// The model sends an expression string and receives the numeric result.
//
// Example:
//
//	agent := yac.Agent{
//	    Tools: []*yac.Tool{tools.Calculator()},
//	}
//	reply, _ := agent.Send(ctx, "What is (3 + 4) * 2?")
func Calculator() *yac.Tool {
	return &yac.Tool{
		Name:        "calculator",
		Description: "Evaluate a mathematical expression. Supports +, -, *, /, ^ (power), parentheses, and functions: sqrt, abs, sin, cos, tan, log, log10. Examples: '(3 + 4) * 2', 'sqrt(144)', '2^10'.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "The mathematical expression to evaluate, e.g. '(3 + 4) * 2'",
				},
			},
			"required": []string{"expression"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Expression == "" {
				return "", fmt.Errorf("expression is required")
			}

			result, err := Eval(params.Expression)
			if err != nil {
				return "", fmt.Errorf("evaluation error: %w", err)
			}

			// Format nicely: drop trailing zeros for whole numbers.
			if result == math.Trunc(result) && !math.IsInf(result, 0) {
				return strconv.FormatFloat(result, 'f', 0, 64), nil
			}
			return strconv.FormatFloat(result, 'f', -1, 64), nil
		},
	}
}

// ---------------------------------------------------------------------------
// Expression evaluator — recursive descent parser
// ---------------------------------------------------------------------------

// Eval parses and evaluates a mathematical expression string.
// It is exported so users can call it directly if needed.
//
//	result, err := tools.Eval("(3 + 4) * 2") // 14
//	result, err := tools.Eval("sqrt(144)")    // 12
//	result, err := tools.Eval("2^10")         // 1024
func Eval(expr string) (float64, error) {
	p := &parser{input: expr}
	result := p.parseExpr()
	if p.err != nil {
		return 0, p.err
	}
	p.skipWhitespace()
	if p.pos < len(p.input) {
		return 0, fmt.Errorf("unexpected character at position %d: %q", p.pos, string(p.input[p.pos]))
	}
	return result, nil
}

type parser struct {
	input string
	pos   int
	err   error
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *parser) peek() byte {
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *parser) consume() byte {
	ch := p.input[p.pos]
	p.pos++
	return ch
}

// parseExpr handles + and - (lowest precedence).
func (p *parser) parseExpr() float64 {
	left := p.parseTerm()
	for p.err == nil {
		switch p.peek() {
		case '+':
			p.consume()
			left += p.parseTerm()
		case '-':
			p.consume()
			left -= p.parseTerm()
		default:
			return left
		}
	}
	return left
}

// parseTerm handles * and /.
func (p *parser) parseTerm() float64 {
	left := p.parsePower()
	for p.err == nil {
		switch p.peek() {
		case '*':
			p.consume()
			left *= p.parsePower()
		case '/':
			p.consume()
			divisor := p.parsePower()
			if divisor == 0 {
				p.err = fmt.Errorf("division by zero")
				return 0
			}
			left /= divisor
		default:
			return left
		}
	}
	return left
}

// parsePower handles ^ (right-associative).
func (p *parser) parsePower() float64 {
	base := p.parseUnary()
	if p.err != nil {
		return base
	}
	if p.peek() == '^' {
		p.consume()
		exp := p.parsePower() // right-associative
		return math.Pow(base, exp)
	}
	return base
}

// parseUnary handles unary + and -.
func (p *parser) parseUnary() float64 {
	switch p.peek() {
	case '+':
		p.consume()
		return p.parseUnary()
	case '-':
		p.consume()
		return -p.parseUnary()
	default:
		return p.parsePrimary()
	}
}

// parsePrimary handles numbers, parenthesized expressions, and functions.
func (p *parser) parsePrimary() float64 {
	if p.err != nil {
		return 0
	}

	ch := p.peek()

	// Parenthesized expression.
	if ch == '(' {
		p.consume()
		val := p.parseExpr()
		if p.peek() != ')' {
			p.err = fmt.Errorf("missing closing parenthesis at position %d", p.pos)
			return 0
		}
		p.consume()
		return val
	}

	// Number.
	if ch == '.' || (ch >= '0' && ch <= '9') {
		return p.parseNumber()
	}

	// Function name (letters).
	if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
		return p.parseFunction()
	}

	p.err = fmt.Errorf("unexpected character at position %d: %q", p.pos, string(ch))
	return 0
}

func (p *parser) parseNumber() float64 {
	p.skipWhitespace()
	start := p.pos
	for p.pos < len(p.input) && (p.input[p.pos] == '.' || (p.input[p.pos] >= '0' && p.input[p.pos] <= '9')) {
		p.pos++
	}
	val, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		p.err = fmt.Errorf("invalid number at position %d", start)
		return 0
	}
	return val
}

func (p *parser) parseFunction() float64 {
	p.skipWhitespace()
	start := p.pos
	for p.pos < len(p.input) && ((p.input[p.pos] >= 'a' && p.input[p.pos] <= 'z') || (p.input[p.pos] >= 'A' && p.input[p.pos] <= 'Z') || (p.input[p.pos] >= '0' && p.input[p.pos] <= '9')) {
		p.pos++
	}
	name := strings.ToLower(p.input[start:p.pos])

	// Expect '(' after function name.
	if p.peek() != '(' {
		// Could be a constant.
		switch name {
		case "pi":
			return math.Pi
		case "e":
			return math.E
		default:
			p.err = fmt.Errorf("unknown identifier %q at position %d", name, start)
			return 0
		}
	}

	p.consume() // '('
	arg := p.parseExpr()
	if p.peek() != ')' {
		p.err = fmt.Errorf("missing closing parenthesis for %s() at position %d", name, p.pos)
		return 0
	}
	p.consume() // ')'

	switch name {
	case "sqrt":
		return math.Sqrt(arg)
	case "abs":
		return math.Abs(arg)
	case "sin":
		return math.Sin(arg)
	case "cos":
		return math.Cos(arg)
	case "tan":
		return math.Tan(arg)
	case "log", "ln":
		return math.Log(arg)
	case "log10":
		return math.Log10(arg)
	case "floor":
		return math.Floor(arg)
	case "ceil":
		return math.Ceil(arg)
	case "round":
		return math.Round(arg)
	default:
		p.err = fmt.Errorf("unknown function %q at position %d", name, start)
		return 0
	}
}
