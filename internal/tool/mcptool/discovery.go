package mcptool

import (
	"context"
	"fmt"

	"github.com/linko-zhao/mini-agent-runtime/internal/tool"
)

func Discover(ctx context.Context, caller Caller, prefix string, maxPages int) ([]tool.Invokable, error) {
	if caller == nil || !prefixPattern.MatchString(prefix) || maxPages <= 0 {
		return nil, fmt.Errorf("%w: invalid discovery configuration", ErrDiscovery)
	}
	tools := make([]tool.Invokable, 0)
	seenNames := make(map[string]bool)
	seenCursors := map[string]bool{"": true}
	cursor := ""
	for range maxPages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := caller.ListTools(ctx, cursor)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, fmt.Errorf("mcptool: listing tools: %w", err)
		}
		if page.Tools == nil {
			return nil, fmt.Errorf("%w: tools array required", ErrDiscovery)
		}
		for _, d := range page.Tools {
			if seenNames[d.Name] {
				return nil, fmt.Errorf("%w: duplicate remote name", ErrDiscovery)
			}
			t, err := New(caller, prefix, d)
			if err != nil {
				return nil, err
			}
			seenNames[d.Name] = true
			tools = append(tools, t)
		}
		if page.NextCursor == "" {
			return tools, nil
		}
		if seenCursors[page.NextCursor] {
			return nil, fmt.Errorf("%w: repeated cursor", ErrDiscovery)
		}
		seenCursors[page.NextCursor] = true
		cursor = page.NextCursor
	}
	return nil, fmt.Errorf("%w: page budget exceeded", ErrDiscovery)
}
