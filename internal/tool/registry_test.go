package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type registryTestTool struct {
	spec    Spec
	metaErr error
	invokes int
}

func (t *registryTestTool) Meta(context.Context) (Spec, error) { return t.spec, t.metaErr }
func (t *registryTestTool) Invoke(context.Context, json.RawMessage) (string, error) {
	t.invokes++
	return "ok", nil
}

func registryTestFake(name string) *registryTestTool {
	return &registryTestTool{spec: Spec{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}}
}

func TestRegistryContract(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	b, a := registryTestFake("b"), registryTestFake("a")
	for _, target := range []*registryTestTool{b, a} {
		if err := r.Register(ctx, target); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"", " \t\n"} {
		if err := r.Register(ctx, registryTestFake(name)); err == nil {
			t.Fatalf("accepted empty name %q", name)
		}
	}
	duplicate := registryTestFake("a")
	if err := r.Register(ctx, duplicate); err == nil {
		t.Fatal("accepted duplicate name")
	}
	if err := r.Register(ctx, &registryTestTool{metaErr: errors.New("metadata failure")}); err == nil {
		t.Fatal("discarded metadata error")
	}

	first, err := r.Specs(nil)
	if err != nil || len(first) != 2 || first[0].Name != "a" || first[1].Name != "b" {
		t.Fatalf("unexpected specs: %+v, %v", first, err)
	}
	second, err := r.Specs(nil)
	if err != nil || len(second) != 2 || second[0].Name != "a" || second[1].Name != "b" {
		t.Fatalf("unstable enumeration: %+v, %v", second, err)
	}
	if a.invokes+b.invokes != 0 {
		t.Fatal("enumeration invoked a tool")
	}

	first[0].InputSchema[0] = 'x'
	a.spec.InputSchema[0] = 'x'
	third, err := r.Specs(nil)
	if err != nil || !json.Valid(third[0].InputSchema) {
		t.Fatalf("registry spec aliases mutable input or output: %+v, %v", third, err)
	}

	visible, err := r.Specs([]string{"b"})
	if err != nil || len(visible) != 1 || visible[0].Name != "b" {
		t.Fatalf("unexpected visible specs: %+v, %v", visible, err)
	}
	if _, err := r.Specs([]string{}); err != nil {
		t.Fatal(err)
	} else if empty, _ := r.Specs([]string{}); len(empty) != 0 {
		t.Fatalf("empty visibility returned specs: %+v", empty)
	}
	if _, err := r.Specs([]string{"missing"}); err == nil {
		t.Fatal("accepted an unregistered visible name")
	}

	registered := r.Tools()["a"]
	if registered == nil {
		t.Fatal("registered tool missing from execution map")
	}
	if _, err := registered.Execute(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if a.invokes != 1 || duplicate.invokes != 0 {
		t.Fatalf("duplicate registration replaced target: original=%d duplicate=%d", a.invokes, duplicate.invokes)
	}
}
