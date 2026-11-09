package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTool struct {
	spec    Spec
	metaErr error
	invokes int
	got     context.Context
}

func (t *fakeTool) Meta(context.Context) (Spec, error) { return t.spec, t.metaErr }
func (t *fakeTool) Invoke(ctx context.Context, _ json.RawMessage) (string, error) {
	t.invokes++
	t.got = ctx
	return "ok", nil
}
func fake(name string) *fakeTool {
	return &fakeTool{spec: Spec{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}}
}
func TestRegistryContract(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	b, a := fake("b"), fake("a")
	for _, f := range []*fakeTool{b, a} {
		if err := r.Register(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Register(ctx, fake("a")); err == nil {
		t.Fatal("accepted duplicate")
	}
	if err := r.Register(ctx, fake(" ")); err == nil {
		t.Fatal("accepted empty name")
	}
	if err := r.Register(ctx, &fakeTool{metaErr: errors.New("metadata failure")}); err == nil {
		t.Fatal("lost meta error")
	}
	specs, err := r.Specs(nil)
	if err != nil || len(specs) != 2 || specs[0].Name != "a" || specs[1].Name != "b" {
		t.Fatalf("%+v %v", specs, err)
	}
	if a.invokes+b.invokes != 0 {
		t.Fatal("enumeration executed tools")
	}
	specs[0].InputSchema[0] = 'x'
	a.spec.InputSchema[0] = 'x'
	again, _ := r.Specs(nil)
	if !json.Valid(again[0].InputSchema) {
		t.Fatal("schema aliases mutable input or output")
	}
	visible, _ := r.Specs([]string{"b"})
	if len(visible) != 1 || visible[0].Name != "b" {
		t.Fatal(visible)
	}
	if target, ok := r.Lookup("a"); !ok || target != a {
		t.Fatal("visibility changed registration")
	}
	empty, _ := r.Specs([]string{})
	if len(empty) != 0 {
		t.Fatal("empty visibility exposed tools")
	}
	if _, err := r.Specs([]string{"missing"}); err == nil {
		t.Fatal("accepted unknown visible name")
	}
}
func TestAdaptPreservesInvocationContext(t *testing.T) {
	f := fake("a")
	adapted, err := Adapt(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "request")
	if _, err := adapted.Execute(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if f.got != ctx || f.invokes != 1 {
		t.Fatal("adapter lost execution context")
	}
	d := adapted.Definition()
	d.Parameters[0] = 'x'
	if !json.Valid(adapted.Definition().Parameters) {
		t.Fatal("definition not copied")
	}
}
