package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type observingStore struct {
	got   context.Context
	calls int
}

func (s *observingStore) Search(c context.Context, q string) ([]Hit, error) {
	s.got = c
	s.calls++
	return []Hit{{ID: "d", Summary: q}}, nil
}
func (s *observingStore) Get(c context.Context, id string) (Document, error) {
	s.got = c
	s.calls++
	return Document{ID: id, Content: "body"}, nil
}
func TestStoreAdapters(t *testing.T) {
	s := &observingStore{}
	search, get := &SearchKnowledge{Store: s}, &GetDocument{Store: s}
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "request")
	if _, err := search.Meta(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := get.Meta(ctx); err != nil {
		t.Fatal(err)
	}
	if s.calls != 0 {
		t.Fatal("metadata accessed store")
	}
	out, err := search.Invoke(ctx, json.RawMessage(`{"query":"runtime"}`))
	if err != nil || s.got != ctx {
		t.Fatalf("%q %v", out, err)
	}
	var hits []Hit
	if err := json.Unmarshal([]byte(out), &hits); err != nil || len(hits) != 1 || hits[0].ID != "d" {
		t.Fatal(out)
	}
	out, err = get.Invoke(ctx, json.RawMessage(`{"id":"d"}`))
	if err != nil || s.got != ctx {
		t.Fatalf("%q %v", out, err)
	}
	var doc Document
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Content != "body" {
		t.Fatal(out)
	}
	before := s.calls
	if _, err := search.Invoke(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("accepted missing query")
	}
	if _, err := get.Invoke(ctx, json.RawMessage(`{"id":"d","extra":1}`)); err == nil {
		t.Fatal("accepted extra field")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := get.Invoke(cancelled, json.RawMessage(`{"id":"d"}`)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if s.calls != before {
		t.Fatal("invalid/cancelled call accessed store")
	}
}
func TestFakeStore(t *testing.T) {
	ctx := context.Background()
	s := NewFakeStore([]Document{{ID: "b", Summary: "runtime"}, {ID: "a", Content: "runtime"}})
	hits, err := s.Search(ctx, "runtime")
	if err != nil || len(hits) != 2 || hits[0].ID != "a" {
		t.Fatalf("%+v %v", hits, err)
	}
	doc, err := s.Get(ctx, hits[0].ID)
	if err != nil || doc.Content != "runtime" {
		t.Fatalf("%+v %v", doc, err)
	}
}
