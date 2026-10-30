package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/tool"
)

type Document struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Content string `json:"content"`
}
type Hit struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

// Store 实现负责后端访问与凭据；工具只依赖此端口。Runtime 传递取消与超时。
type Store interface {
	Search(context.Context, string) ([]Hit, error)
	Get(context.Context, string) (Document, error)
}
type SearchKnowledge struct{ Store Store }

func (*SearchKnowledge) Meta(ctx context.Context) (tool.Spec, error) {
	if err := ctx.Err(); err != nil {
		return tool.Spec{}, err
	}
	return tool.Spec{Name: "search_knowledge", Description: "按 query 检索文档 ID 和摘要", InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)}, nil
}
func (t *SearchKnowledge) Invoke(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := decode(args, &input); err != nil {
		return "", err
	}
	if input.Query == "" {
		return "", &tool.Error{Kind: tool.ValidationError, Message: "query required"}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if t.Store == nil {
		return "", fmt.Errorf("knowledge: missing store")
	}
	hits, err := t.Store.Search(ctx, input.Query)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(hits)
	return string(b), err
}

type GetDocument struct{ Store Store }

func (*GetDocument) Meta(ctx context.Context) (tool.Spec, error) {
	if err := ctx.Err(); err != nil {
		return tool.Spec{}, err
	}
	return tool.Spec{Name: "get_document", Description: "按文档 ID 读取正文", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`)}, nil
}
func (t *GetDocument) Invoke(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decode(args, &input); err != nil {
		return "", err
	}
	if input.ID == "" {
		return "", &tool.Error{Kind: tool.ValidationError, Message: "id required"}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if t.Store == nil {
		return "", fmt.Errorf("knowledge: missing store")
	}
	doc, err := t.Store.Get(ctx, input.ID)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(doc)
	return string(b), err
}
func decode(raw json.RawMessage, out any) error {
	fail := func(cause error) error {
		return &tool.Error{Kind: tool.DecodeError, Message: "expected one valid arguments object", Cause: cause}
	}
	b := bytes.TrimSpace(raw)
	if len(b) == 0 || b[0] != '{' {
		return fail(nil)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return fail(err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fail(err)
	}
	return nil
}
