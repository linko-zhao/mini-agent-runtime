package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

// calculator 最小实现：args {"a":1,"b":1}，返回 a+b。
type Calculator struct{}

func (c *Calculator) Name() string { return "calculator" }

func (c *Calculator) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		A, B int
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", err
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
