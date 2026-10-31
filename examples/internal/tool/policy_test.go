package tool

import (
 "context"
 "encoding/json"
 "errors"
 "testing"
 "time"
 "github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

type policyFake struct { fn func(context.Context,json.RawMessage)(string,error) }
func (f policyFake) Name() string { return "lookup" }
func (f policyFake) Definition() model.ToolDefinition { return model.ToolDefinition{Name:"lookup"} }
func (f policyFake) Execute(ctx context.Context,a json.RawMessage)(string,error) { return f.fn(ctx,a) }

func TestPolicyReadRetryAndBudget(t *testing.T) {
 transient:=errors.New("transient")
 calls:=0
 wrapped,err:=WithPolicy(policyFake{func(_ context.Context,a json.RawMessage)(string,error){
  calls++
  if string(a)!="{}" { t.Fatalf("mutated args: %s",a) }
  a[0]='x'
  if calls==1 { return "",transient }
  return "ok",nil
 }},Policy{MaxAttempts:2,ReadOnly:true,Retryable:func(e error)bool{return errors.Is(e,transient)},MaxResultBytes:2})
 if err!=nil {t.Fatal(err)}
 out,err:=wrapped.Execute(context.Background(),json.RawMessage("{}"))
 if err!=nil||out!="ok"||calls!=2||wrapped.Name()!="lookup"||wrapped.Definition().Name!="lookup" {t.Fatalf("%q %v calls=%d",out,err,calls)}
 budget,_:=WithPolicy(policyFake{func(context.Context,json.RawMessage)(string,error){return "中文",nil}},Policy{MaxResultBytes:5})
 out,err=budget.Execute(context.Background(),nil)
 var known *Error
 if out!=""||!errors.As(err,&known)||known.Kind!=ExecutionError {t.Fatalf("%q %v",out,err)}
}

func TestPolicyNoUnsafeRetry(t *testing.T) {
 for _,p:=range []Policy{{MaxAttempts:2},{MaxAttempts:2,ReadOnly:true},{Timeout:-1},{RetryDelay:-1},{MaxResultBytes:-1}} {
  if _,err:=WithPolicy(policyFake{},p);err==nil {t.Fatalf("accepted %+v",p)}
 }
 for _,failure:=range []error{&Error{Kind:ValidationError,Message:"bad input"},context.Canceled,context.DeadlineExceeded,errors.New("permanent")} {
  calls:=0
  wrapped,_:=WithPolicy(policyFake{func(context.Context,json.RawMessage)(string,error){calls++;return "",failure}},Policy{MaxAttempts:3,ReadOnly:true,Retryable:func(e error)bool{return e!=failure||failure.Error()!="permanent"}})
  _,err:=wrapped.Execute(context.Background(),nil)
  if !errors.Is(err,failure)||calls!=1 {t.Fatalf("%v calls=%d",err,calls)}
 }
}

func TestPolicyCancellationAndTimeout(t *testing.T) {
 calls:=0
 wrapped,_:=WithPolicy(policyFake{func(ctx context.Context,_ json.RawMessage)(string,error){calls++;<-ctx.Done();return "late",nil}},Policy{Timeout:5*time.Millisecond})
 _,err:=wrapped.Execute(context.Background(),nil)
 if !errors.Is(err,context.DeadlineExceeded)||calls!=1 {t.Fatalf("%v calls=%d",err,calls)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 _,err=wrapped.Execute(ctx,nil)
 if !errors.Is(err,context.Canceled)||calls!=1 {t.Fatalf("%v calls=%d",err,calls)}
}

func TestPolicyCancellationDuringRetryWait(t *testing.T) {
 ctx,cancel:=context.WithCancel(context.Background())
 defer cancel()
 calls:=0
 failure:=errors.New("temporary")
 wrapped,_:=WithPolicy(policyFake{func(context.Context,json.RawMessage)(string,error){
  calls++
  go func(){time.Sleep(time.Millisecond);cancel()}()
  return "",failure
 }},Policy{MaxAttempts:3,ReadOnly:true,Retryable:func(error)bool{return true},RetryDelay:time.Hour})
 _,err:=wrapped.Execute(ctx,nil)
 if !errors.Is(err,context.Canceled)||calls!=1 {t.Fatalf("%v calls=%d",err,calls)}
}

func TestPolicyDefaultNoRetry(t *testing.T) {
 calls:=0
 cause:=errors.New("transient")
 wrapped,_:=WithPolicy(policyFake{func(context.Context,json.RawMessage)(string,error){calls++;return "",cause}},Policy{})
 _,err:=wrapped.Execute(context.Background(),nil)
 if !errors.Is(err,cause)||calls!=1 {t.Fatalf("%v calls=%d",err,calls)}
}
