package tool

import (
	"context"
	"fmt"
	"strings"
)

type Principal struct { 
	ID string 
}
type principalContextKey struct {}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok && strings.TrimSpace(p.ID) != ""
}

type Capability string

const (
	ReadCapability Capability = "read"
	WriteCapability Capability = "write"
)

type Authorizer interface {
	Authorize(context.Context, Call) error
}

type CapabilityAuthorizer struct {
	rules map[string]Capability
	grants map[string]map[Capability]bool
}

func validCapability(c Capability) bool {
	return c == ReadCapability || c == WriteCapability
}

func NewCapabilityAuthorizer(rules map[string]Capability, grants map[string][]Capability) (*CapabilityAuthorizer, error) {
	a := &CapabilityAuthorizer{
		rules: make(map[string]Capability),
		grants: make(map[string]map[Capability]bool),
	}
	for id, capabilities := range grants {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("tool: empty principal")
		}
		a.grants[id] = make(map[Capability]bool)
		for _, capability := range capabilities {
			if !validCapability(capability) {
				return nil, fmt.Errorf("tool: invalid grant")
			}
			a.grants[id][capability] = true
		}
	}
	return a, nil
}

func (a *CapabilityAuthorizer) Authorize(ctx context.Context, call Call) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	principal, ok := PrincipalFromContext(ctx)
	if !ok || a == nil {
		return fmt.Errorf("tool: execution forbidden")
	}
	capability, registered := a.rules[call.Name]
	if !registered || !a.grants[principal.ID][capability] {
		return fmt.Errorf("tool: execution forbidden")
	}
	return nil
}
