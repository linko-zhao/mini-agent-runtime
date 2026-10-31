package tool

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "time"

 "github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

// Policy applies to the whole logical call, including all retries and waits.
// ReadOnly is an explicit application assertion, never inferred from a model.
type Policy struct {
 Timeout time.Duration
 MaxAttempts int
 ReadOnly bool
 Retryable func(error) bool
 RetryDelay time.Duration
 MaxResultBytes int
}

// WithPolicy preserves the legacy described-tool boundary used by Agent.
// No automatic retry occurs unless both ReadOnly and Retryable permit it.
func WithPolicy(t DescribedTool, p Policy) (DescribedTool, error) {
 if t == nil || p.Timeout < 0 || p.MaxAttempts < 0 || p.RetryDelay < 0 || p.MaxResultBytes < 0 {
  return nil, fmt.Errorf("tool: invalid policy")
 }
 if p.MaxAttempts == 0 { p.MaxAttempts = 1 }
 if p.MaxAttempts > 1 && (!p.ReadOnly || p.Retryable == nil) {
  return nil, fmt.Errorf("tool: retries require explicit read-only and retryable policy")
 }
 return &policyTool{target:t, policy:p}, nil
}

type policyTool struct { target DescribedTool; policy Policy }
func (t *policyTool) Name() string { return t.target.Name() }
func (t *policyTool) Definition() model.ToolDefinition { return t.target.Definition() }
func (t *policyTool) Execute(ctx context.Context, args json.RawMessage) (string,error) {
 if err:=ctx.Err(); err!=nil { return "",err }
 if t.policy.Timeout > 0 {
  var cancel context.CancelFunc
  ctx,cancel=context.WithTimeout(ctx,t.policy.Timeout)
  defer cancel()
 }
 for attempt:=1; attempt<=t.policy.MaxAttempts; attempt++ {
  if err:=ctx.Err();err!=nil { return "",err }
  result,err:=t.target.Execute(ctx,append(json.RawMessage(nil),args...))
  if ctx.Err()!=nil { return "",ctx.Err() }
  if err==nil {
   if t.policy.MaxResultBytes>0 && len(result)>t.policy.MaxResultBytes {
    return "", &Error{Kind:ExecutionError,Message:"tool result exceeds byte budget"}
   }
   return result,nil
  }
  if errors.Is(err,context.Canceled)||errors.Is(err,context.DeadlineExceeded) { return "",err }
  var controlled *Error
  if errors.As(err,&controlled)||attempt==t.policy.MaxAttempts||t.policy.Retryable==nil||!t.policy.Retryable(err) {
   return "",err
  }
  if t.policy.RetryDelay>0 {
   timer:=time.NewTimer(t.policy.RetryDelay)
   select {
   case <-ctx.Done(): timer.Stop(); return "",ctx.Err()
   case <-timer.C:
   }
  }
 }
 panic("unreachable")
}
