// Package mcptool adapts the Day12 MCP client to the existing Day08 tool contract.
package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/mcp"
	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
)

var (
	ErrDiscovery          = errors.New("mcptool: discovery rejected")
	ErrRemoteExecution    = errors.New("mcptool: remote execution failed")
	ErrUnsupportedContent = errors.New("mcptool: unsupported content")
	ErrInvalidResult      = errors.New("mcptool: invalid result")
	prefixPattern         = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)
	namePattern           = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// Caller is implemented by an already-initialized *mcp.Client.
// The host owns initialization, transport cleanup and recovery.
type Caller interface {
	ListTools(context.Context, string) (mcp.ToolsPage, error)
	CallTool(context.Context, string, json.RawMessage) (mcp.ToolResult, error)
}

// ResultEnvelope is the explicit projection into the existing string result.
// Non-text blocks are rejected, not silently flattened or discarded.
type ResultEnvelope struct {
	Text              []string        `json:"text"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
}

type remoteTool struct {
	caller     Caller
	remoteName string
	spec       tool.Spec
}

func localName(prefix, name string) (string, error) {
	local := prefix + "__" + name
	// Prefixes deliberately exclude underscores so namespace boundaries are unique.
	if !prefixPattern.MatchString(prefix) || !namePattern.MatchString(name) || len(local) > 64 {
		return "", fmt.Errorf("%w: incompatible local tool name", ErrDiscovery)
	}
	return local, nil
}

// New snapshots metadata. It does not initialize a client, execute a tool,
// or derive authorization/retry policy from remote annotations.
func New(caller Caller, prefix string, d mcp.Descriptor) (tool.Invokable, error) {
	if caller == nil {
		return nil, fmt.Errorf("%w: nil caller", ErrDiscovery)
	}
	name, err := localName(prefix, d.Name)
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if json.Unmarshal(d.InputSchema, &schema) != nil || schema["type"] != "object" {
		return nil, fmt.Errorf("%w: object input schema required", ErrDiscovery)
	}
	return &remoteTool{caller: caller, remoteName: d.Name, spec: tool.Spec{
		Name: name, Description: d.Description, InputSchema: append(json.RawMessage(nil), d.InputSchema...),
	}}, nil
}
func (t *remoteTool) Meta(ctx context.Context) (tool.Spec, error) {
	if err := ctx.Err(); err != nil {
		return tool.Spec{}, err
	}
	spec := t.spec
	spec.InputSchema = append(json.RawMessage(nil), spec.InputSchema...)
	return spec, nil
}
func failure(message string, cause error) error {
	return &tool.Error{Kind: tool.ExecutionError, Message: message, Cause: cause}
}
func (t *remoteTool) Invoke(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result, err := t.caller.CallTool(ctx, t.remoteName, append(json.RawMessage(nil), args...))
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	if err != nil {
		return "", failure("remote tool request failed", err)
	}
	if result.IsError {
		return "", failure("remote tool execution failed", ErrRemoteExecution)
	}
	if result.Content == nil {
		return "", failure("invalid remote result", ErrInvalidResult)
	}
	envelope := ResultEnvelope{Text: make([]string, 0, len(result.Content))}
	for _, raw := range result.Content {
		var block struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if json.Unmarshal(raw, &block) != nil || block.Type == "" {
			return "", failure("invalid remote result", ErrInvalidResult)
		}
		if block.Type != "text" {
			return "", failure("remote content type is not supported", ErrUnsupportedContent)
		}
		if block.Text == nil {
			return "", failure("invalid remote result", ErrInvalidResult)
		}
		envelope.Text = append(envelope.Text, *block.Text)
	}
	if len(result.StructuredContent) > 0 {
		var object map[string]json.RawMessage
		if json.Unmarshal(result.StructuredContent, &object) != nil || object == nil {
			return "", failure("invalid remote result", ErrInvalidResult)
		}
		envelope.StructuredContent = append(json.RawMessage(nil), result.StructuredContent...)
	}
	output, err := json.Marshal(envelope)
	if err != nil {
		return "", failure("invalid remote result", ErrInvalidResult)
	}
	return string(output), nil
}
