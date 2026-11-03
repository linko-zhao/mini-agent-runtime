package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

type registration struct {
	spec   Spec
	target Invokable
}

type Registry struct {
	mu      sync.RWMutex
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

func (r *Registry) Register(ctx context.Context, t Invokable) error {
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
		spec:   cloneSpec(s),
		target: t,
	}
	return nil
}

func (r *Registry) Specs(visible []string) ([]Spec, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := append([]string(nil), visible...)
	if visible == nil {
		for name := range r.entries {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]Spec, 0, len(names))
	last := ""
	for _, name := range names {
		if name == last {
			continue
		}
		last = name
		e, ok := r.entries[name]
		if !ok {
			return nil, fmt.Errorf("tool: unregistered name %q", name)
		}
		out = append(out, cloneSpec(e.spec))
	}
	return out, nil
}

func (r *Registry) Tools() map[string]Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Tool, len(r.entries))
	for name, e := range r.entries {
		out[name] = &legacyAdapter{spec: cloneSpec(e.spec), target: e.target}
	}
	return out
}

func Definitions(specs []Spec) []model.ToolDefinition {
	defs := make([]model.ToolDefinition, 0, len(specs))
	for _, s := range specs {
		defs = append(defs, model.ToolDefinition{
			Name:        s.Name,
			Description: s.Description,
			Parameters:  append(json.RawMessage(nil), s.InputSchema...),
		})
	}
	return defs
}
