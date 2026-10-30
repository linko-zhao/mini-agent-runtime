package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// FakeStore 仅供离线演示；构造时复制数据，之后并发只读。
type FakeStore struct{ docs map[string]Document }

func NewFakeStore(docs []Document) *FakeStore {
	s := &FakeStore{docs: make(map[string]Document, len(docs))}
	for _, d := range docs {
		s.docs[d.ID] = d
	}
	return s
}
func (s *FakeStore) Search(ctx context.Context, query string) ([]Hit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hits := make([]Hit, 0)
	for _, d := range s.docs {
		if strings.Contains(d.Summary, query) || strings.Contains(d.Content, query) {
			hits = append(hits, Hit{ID: d.ID, Summary: d.Summary})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].ID < hits[j].ID })
	return hits, nil
}
func (s *FakeStore) Get(ctx context.Context, id string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	d, ok := s.docs[id]
	if !ok {
		return Document{}, fmt.Errorf("knowledge: document not found")
	}
	return d, nil
}
