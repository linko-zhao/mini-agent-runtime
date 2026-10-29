package calculator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

// calculator 最小实现：args {"a":1,"b":1}，返回 a+b。
type Calculator struct{}

func (c *Calculator) Name() string { return "calculator" }

func (c *Calculator) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	d := json.NewDecoder(bytes.NewReader(args))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return "", &tool.Error{
				Kind: tool.ValidationError,
				Message: "arguments has unexpected field",
			}
		}
		return "", &tool.Error{
			Kind: tool.DecodeError,
			Message: "arguments must be a valid JSON object",
			Cause: err,
		}
	}
	return fmt.Sprint(p.A + p.B), nil
}

func (c *Calculator) Definition() model.ToolDefinition {
	return model.ToolDefinition{
		Name:        "calculator",
		Description: "计算两个整数的和",
		Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "a": {"type": "integer"},
                "b": {"type": "integer"}
            },
            "required": ["a", "b"],
            "additionalProperties": false
        }`),
	}
}
