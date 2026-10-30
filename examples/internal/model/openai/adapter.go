package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

type ChatModelConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

type ChatModel struct {
	config ChatModelConfig
	tools  []ChatCompletionTool
}

func NewChatModel(config *ChatModelConfig) (*ChatModel, error) {
	if config == nil {
		return nil, fmt.Errorf("openai: nil config")
	}
	if config.APIKey == "" || config.BaseURL == "" || config.Model == "" {
		return nil, fmt.Errorf("openai: missing required configurations")
	}

	var httpClient *http.Client
	if config.HTTPClient == nil {
		httpClient = &http.Client{}
	} else {
		httpClient = config.HTTPClient
	}

	return &ChatModel{config: ChatModelConfig{
		APIKey:     config.APIKey,
		BaseURL:    config.BaseURL,
		Model:      config.Model,
		HTTPClient: httpClient,
	}}, nil
}

func (cm *ChatModel) Generate(ctx context.Context, messages []model.Message) (*model.Response, error) {
	payload, err := convertRequest(ctx, cm, messages)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cm.config.BaseURL, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cm.config.APIKey))

	resp, err := cm.config.HTTPClient.Do(req)
	if err != nil {
		return nil, wrapTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newHTTPError(resp.StatusCode, err)
	}

	return convertResponse(ctx, resp)
}

func (cm *ChatModel) WithTools(defs []model.ToolDefinition) (model.Model, error) {
	m := *cm
	m.tools = nil
	seen := map[string]bool{}
	for _, d := range defs {
		if d.Name == "" {
			return nil, fmt.Errorf("openai: empty tool name")
		}
		if seen[d.Name] {
			return nil, fmt.Errorf("openai: duplicate tool name %q", d.Name)
		}
		seen[d.Name] = true
		if len(d.Parameters) > 0 {
			var schema map[string]any
			if err := json.Unmarshal(d.Parameters, &schema); err != nil {
				return nil, fmt.Errorf("openai: tool %q parameters must be a JSON object: %w", d.Name, err)
			}
			if schema["type"] != "object" {
				return nil, fmt.Errorf("openai: tool %q parameters must be a JSON Schema object with type \"object\"", d.Name)
			}
		}
		m.tools = append(m.tools, ChatCompletionTool{
			Type:     "function",
			Function: d,
		})
	}
	return &m, nil
}
