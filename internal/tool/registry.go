package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type registration struct {
	spec Spec
	target Invokable
}

type Registry struct {
	mu sync.Mutex
	entries map[string]registration
}

func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]registration),
	}
}

func validateSpec(s Spec) error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("tool: empty name")
	}
	var schema map[string]any
	if err := json.Unmarshal(s.InputSchema, &schema); err != nil || schema["type"] != "object" {
		return fmt.Errorf("tool %q: expected object input schema", s.Name)
	}
	return nil
}

func (r *Registry) Registry(ctx context.Context, t Invokable) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("tool: nil invokable")
	}
	s, err := t.Meta(ctx)
	if err != nil {
		return err
	}
	if err := validateSpec(s); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]registration)
	}
	if _, exists := r.entries[s.Name]; exists {
		return fmt.Errorf("tool: duplicate name %q", s.Name)
	}
	r.entries[s.Name] = registration{
		spec: cloneSpec(s),
		target: t,
	}
	return nil
}
