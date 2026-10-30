package tool

type ErrorKind string

const (
	DecodeError     ErrorKind = "invalid_json"
	ValidationError ErrorKind = "invalid_arguments"
	NotFoundError   ErrorKind = "tool_not_found"
	ExecutionError  ErrorKind = "execution_failed"
)

type Error struct {
	Kind    ErrorKind
	Message string // 经过控制、可以回填模型的提示
	Cause   error  // 保留原始错误链
}

func (e *Error) Error() string {
	return string(e.Kind) + ": " + e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}
