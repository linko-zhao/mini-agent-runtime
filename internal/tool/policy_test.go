package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/linko-zhao/mini-agent-runtime/internal/model"
)

type policyTestTool struct {
	fn func(context.Context, json.RawMessage) (string, error)
}

func (f policyTestTool) Name() string { return "lookup" }
func (f policyTestTool) Definition() model.ToolDefinition {
	return model.ToolDefinition{Name: "lookup"}
}
func (f policyTestTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return f.fn(ctx, args)
}

func TestPolicyReadRetryAndBudget(t *testing.T) {
	transient := errors.New("transient")
	calls := 0
	wrapped, err := WithPolicy(policyTestTool{func(_ context.Context, args json.RawMessage) (string, error) {
		calls++
		if string(args) != "{}" {
			t.Fatalf("mutated args: %s", args)
		}
		args[0] = 'x'
		if calls == 1 {
			return "", transient
		}
		return "ok", nil
	}}, Policy{MaxAttempts: 2, ReadOnly: true, Retryable: func(err error) bool { return errors.Is(err, transient) }, MaxResultBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	out, err := wrapped.Execute(context.Background(), json.RawMessage("{}"))
	if err != nil || out != "ok" || calls != 2 || wrapped.Name() != "lookup" || wrapped.Definition().Name != "lookup" {
		t.Fatalf("%q %v calls=%d", out, err, calls)
	}

	budget, err := WithPolicy(policyTestTool{func(context.Context, json.RawMessage) (string, error) { return "中文", nil }}, Policy{MaxResultBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	out, err = budget.Execute(context.Background(), nil)
	var known *Error
	if out != "" || !errors.As(err, &known) || known.Kind != ExecutionError {
		t.Fatalf("%q %v", out, err)
	}
}

func TestPolicyNoUnsafeRetry(t *testing.T) {
	for _, policy := range []Policy{{MaxAttempts: 2}, {MaxAttempts: 2, ReadOnly: true}, {Timeout: -1}, {RetryDelay: -1}, {MaxResultBytes: -1}} {
		if _, err := WithPolicy(policyTestTool{}, policy); err == nil {
			t.Fatalf("accepted %+v", policy)
		}
	}
	for _, failure := range []error{&Error{Kind: ValidationError, Message: "bad input"}, context.Canceled, context.DeadlineExceeded, errors.New("permanent")} {
		calls := 0
		wrapped, err := WithPolicy(policyTestTool{func(context.Context, json.RawMessage) (string, error) {
			calls++
			return "", failure
		}}, Policy{MaxAttempts: 3, ReadOnly: true, Retryable: func(err error) bool { return err != failure || failure.Error() != "permanent" }})
		if err != nil {
			t.Fatal(err)
		}
		_, err = wrapped.Execute(context.Background(), nil)
		if !errors.Is(err, failure) || calls != 1 {
			t.Fatalf("%v calls=%d", err, calls)
		}
	}
}

func TestPolicyCancellationAndTimeout(t *testing.T) {
	calls := 0
	wrapped, err := WithPolicy(policyTestTool{func(ctx context.Context, _ json.RawMessage) (string, error) {
		calls++
		<-ctx.Done()
		return "late", nil
	}}, Policy{Timeout: 5 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wrapped.Execute(context.Background(), nil)
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("%v calls=%d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = wrapped.Execute(ctx, nil)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("%v calls=%d", err, calls)
	}
}

func TestPolicyCancellationDuringRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	failure := errors.New("temporary")
	wrapped, err := WithPolicy(policyTestTool{func(context.Context, json.RawMessage) (string, error) {
		calls++
		go func() { time.Sleep(time.Millisecond); cancel() }()
		return "", failure
	}}, Policy{MaxAttempts: 3, ReadOnly: true, Retryable: func(error) bool { return true }, RetryDelay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wrapped.Execute(ctx, nil)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("%v calls=%d", err, calls)
	}
}

func TestPolicyDefaultNoRetry(t *testing.T) {
	calls := 0
	cause := errors.New("transient")
	wrapped, err := WithPolicy(policyTestTool{func(context.Context, json.RawMessage) (string, error) {
		calls++
		return "", cause
	}}, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wrapped.Execute(context.Background(), nil)
	if !errors.Is(err, cause) || calls != 1 {
		t.Fatalf("%v calls=%d", err, calls)
	}
}
