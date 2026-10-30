package calculator

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"strconv"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
)

type Calculator struct{}

func (*Calculator) Name() string { return "calculator" }
func (c *Calculator) Definition() model.ToolDefinition {
	return model.ToolDefinition{Name: c.Name(), Description: "计算两个整数的和", Parameters: json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"],"additionalProperties":false}`)}
}
func (*Calculator) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	invalid := func(message string, cause error) (string, error) {
		return "", &tool.Error{Kind: tool.ValidationError, Message: message, Cause: cause}
	}
	if b := bytes.TrimSpace(raw); len(b) == 0 || b[0] != '{' {
		return invalid("expected object", nil)
	}
	var args struct {
		A *int `json:"a"`
		B *int `json:"b"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&args); err != nil {
		return "", &tool.Error{Kind: tool.DecodeError, Message: "invalid JSON arguments", Cause: err}
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return invalid("expected exactly one JSON object", err)
	}
	if args.A == nil || args.B == nil {
		return invalid("a and b are required integers", nil)
	}
	a, b := *args.A, *args.B
	if (b > 0 && a > math.MaxInt-b) || (b < 0 && a < math.MinInt-b) {
		return invalid("integer overflow", nil)
	}
	return strconv.Itoa(a + b), nil
}
