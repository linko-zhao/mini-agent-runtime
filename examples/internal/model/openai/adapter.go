// Package openai translates the course Runtime model contract to the
// non-streaming Chat Completions HTTP protocol described on Day 02/03.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

type Config struct {
	APIKey, Model, BaseURL string
	Client                 *http.Client
}
type Adapter struct {
	config      Config
	definitions []model.ToolDefinition
}

func New(config Config) (*Adapter, error) {
	if config.APIKey == "" || config.Model == "" || config.BaseURL == "" {
		return nil, fmt.Errorf("openai adapter: API key, model and base URL are required")
	}
	if config.Client == nil {
		config.Client = http.DefaultClient
	}
	return &Adapter{config: config}, nil
}
func (a *Adapter) WithTools(defs []model.ToolDefinition) (model.Model, error) {
	copyDefs := make([]model.ToolDefinition, 0, len(defs))
	seen := map[string]bool{}
	for _, d := range defs {
		var schema map[string]any
		if d.Name == "" || seen[d.Name] {
			return nil, fmt.Errorf("openai adapter: empty or duplicate tool name %q", d.Name)
		}
		if err := json.Unmarshal(d.Parameters, &schema); err != nil || schema["type"] != "object" {
			return nil, fmt.Errorf("openai adapter: tool %q requires an object schema", d.Name)
		}
		seen[d.Name] = true
		d.Parameters = append(json.RawMessage(nil), d.Parameters...)
		copyDefs = append(copyDefs, d)
	}
	next := *a
	next.definitions = copyDefs
	return &next, nil
}
func (a *Adapter) Generate(ctx context.Context, messages []model.Message) (*model.Response, error) {
	body, err := encodeRequest(a.config.Model, messages, a.definitions)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	resp, err := a.config.Client.Do(req)
	if err != nil {
		return nil, &model.ModelError{Code: "transport_error", Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpError(resp.StatusCode)
	}
	return decodeResponse(resp.Body)
}
func httpError(status int) error {
	code, retry := "provider_error", false
	switch {
	case status == 401:
		code = "unauthorized"
	case status == 429:
		code = "rate_limited"
		retry = true
	case status >= 500:
		code = "provider_unavailable"
		retry = true
	}
	return &model.ModelError{StatusCode: status, Code: code, Retryable: retry}
}
