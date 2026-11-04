package mcp

import (
	"context"
	"fmt"
)

type Client struct {
	transport Transport
	gate chan struct{}
	state string
}

func NewClient(t Transport) (*Client, error) {
	if t == nil {
		return nil, fmt.Errorf("mcp: nil transport")
	}
	return &Client{transport: t}, nil
}

func (c *Client) enter(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case c.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-c.gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) leave() { <-c.gate }

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	var result InitializeResult
	if err := c.enter(ctx); err != nil {
		return result, err
	}
	defer c.leave()
	if c.state != "new" {
		return result, ErrSessionState
	}
}
