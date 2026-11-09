package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Principal is supplied by the trusted host after authentication, not by the model.
type Principal struct{ ID string }
type principalContextKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok && strings.TrimSpace(p.ID) != ""
}

var ErrForbidden = errors.New("tool: execution forbidden")

type Capability string

const (
	ReadCapability  Capability = "read"
	WriteCapability Capability = "write"
)

type Authorizer interface {
	Authorize(context.Context, Call) error
}

// CapabilityAuthorizer is an immutable, tool-level teaching policy.
// It is not an authentication service or a resource/tenant-level ACL.
type CapabilityAuthorizer struct {
	rules  map[string]Capability
	grants map[string]map[Capability]bool
}

func validCapability(c Capability) bool { return c == ReadCapability || c == WriteCapability }

// NewCapabilityAuthorizer copies its inputs. Unknown tools/principals are denied.
// Read and write are independent capabilities; write does not imply read.
func NewCapabilityAuthorizer(rules map[string]Capability, grants map[string][]Capability) (*CapabilityAuthorizer, error) {
	a := &CapabilityAuthorizer{rules: make(map[string]Capability), grants: make(map[string]map[Capability]bool)}
	for name, capability := range rules {
		if strings.TrimSpace(name) == "" || !validCapability(capability) {
			return nil, fmt.Errorf("tool: invalid capability rule")
		}
		a.rules[name] = capability
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
		return ErrForbidden
	}
	capability, registered := a.rules[call.Name]
	if !registered || !a.grants[principal.ID][capability] {
		return ErrForbidden
	}
	return nil
}
