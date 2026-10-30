package openai

import (
	"context"
	"errors"
	"net/http"

	"github.com/linko-zhao/mini-agent-runtime/examples/internal/model"
)

func newHTTPError(status int, cause error) error {
	code := "http_error"
	retryable := false

	switch {
	case status == http.StatusUnauthorized:
		code = "unauthorized"

	case status == http.StatusForbidden:
		code = "forbidden"

	case status == http.StatusTooManyRequests:
		code = "rate_limited"
		retryable = true

	case status >= 500:
		code = "server_error"
		retryable = true

	case status >= 400:
		code = "bad_request"
	}

	return &model.ModelError{
		StatusCode: status,
		Code:       code,
		Retryable:  retryable,
		Err:        cause,
	}
}

func wrapTransportError(err error) error {
	code := "network_error"
	retryable := true

	switch {
	case errors.Is(err, context.Canceled):
		code = "request_canceled"
		retryable = false

	case errors.Is(err, context.DeadlineExceeded):
		code = "timeout"
		retryable = false
	}

	return &model.ModelError{
		Code:      code,
		Retryable: retryable,
		Err:       err,
	}
}
