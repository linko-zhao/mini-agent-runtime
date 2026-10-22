package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

type ChatModelConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

type ChatModel struct {
	config ChatModelConfig
}

func NewChatModel(config *ChatModelConfig) (*ChatModel, error) {
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
	payload := map[string]any{
		"model":    cm.config.Model,
		"messages": messages,
	}
	buf, _ := json.Marshal(payload)

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

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, newHTTPError(resp.StatusCode, err)
	}

	return convertResponse(ctx, resp)
}
