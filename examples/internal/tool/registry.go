package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

type registration struct {
	spec   Spec
	target Invokable
}

// Registry 保存完整执行集合。Specs 为模型可见集合的独立快照，不授予执行权限。
type Registry struct {
	mu      sync.RWMutex
	entries map[string]registration
}

func NewRegistry() *Registry { return &Registry{entries: make(map[string]registration)} }
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

// Register 读取一次元信息；不调用 Invoke。重复名称绝不覆盖。
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
	if err = validateSpec(s); err != nil {
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
	r.entries[s.Name] = registration{spec: cloneSpec(s), target: t}
	return nil
}
func (r *Registry) Lookup(name string) (Invokable, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[name]
	return entry.target, ok
}

// Specs(nil) 枚举全部；非 nil 空切片返回空集合。未知名称返回错误。
// 返回值按名称排序且深拷贝 Schema；调用方修改不会改变注册表。
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

// Tools 为旧 Agent 构造 API 提供所有已注册工具的适配快照。
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
		defs = append(defs, model.ToolDefinition{Name: s.Name, Description: s.Description, Parameters: append(json.RawMessage(nil), s.InputSchema...)})
	}
	return defs
}
