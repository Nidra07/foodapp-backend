// Package errors defines a single error taxonomy shared by every module.
// Application/domain code returns *AppError; the HTTP layer maps AppError
// -> standardized JSON error envelope + status code. This keeps error
// handling consistent across all 60+ domain modules instead of each one
// inventing its own error shape.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	CodeValidation      Code = "VALIDATION_ERROR"
	CodeNotFound        Code = "NOT_FOUND"
	CodeConflict        Code = "CONFLICT"
	CodeUnauthorized    Code = "UNAUTHORIZED"
	CodeForbidden       Code = "FORBIDDEN"
	CodeRateLimited     Code = "RATE_LIMITED"
	CodeInternal        Code = "INTERNAL_ERROR"
	CodeUnavailable     Code = "SERVICE_UNAVAILABLE"
	CodeIdempotentReplay Code = "IDEMPOTENT_REPLAY"
	CodePreconditionFail Code = "PRECONDITION_FAILED"
)

var codeToStatus = map[Code]int{
	CodeValidation:       http.StatusBadRequest,
	CodeNotFound:         http.StatusNotFound,
	CodeConflict:         http.StatusConflict,
	CodeUnauthorized:     http.StatusUnauthorized,
	CodeForbidden:        http.StatusForbidden,
	CodeRateLimited:      http.StatusTooManyRequests,
	CodeInternal:         http.StatusInternalServerError,
	CodeUnavailable:      http.StatusServiceUnavailable,
	CodeIdempotentReplay: http.StatusOK,
	CodePreconditionFail: http.StatusPreconditionFailed,
}

// AppError is the canonical error type. Message is safe to show the
// client; the wrapped Err (if any) is logged but never serialized.
type AppError struct {
	Code    Code
	Message string
	Details map[string]interface{}
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

func (e *AppError) HTTPStatus() int {
	if s, ok := codeToStatus[e.Code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code Code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

func WithDetails(code Code, message string, details map[string]interface{}) *AppError {
	return &AppError{Code: code, Message: message, Details: details}
}

// Convenience constructors for the most common cases.
func NotFound(resource string) *AppError {
	return New(CodeNotFound, fmt.Sprintf("%s not found", resource))
}

func Validation(message string, details map[string]interface{}) *AppError {
	return WithDetails(CodeValidation, message, details)
}

func Unauthorized(message string) *AppError {
	if message == "" {
		message = "authentication required"
	}
	return New(CodeUnauthorized, message)
}

func Forbidden(message string) *AppError {
	if message == "" {
		message = "you do not have permission to perform this action"
	}
	return New(CodeForbidden, message)
}

func Internal(err error) *AppError {
	return Wrap(CodeInternal, "an unexpected error occurred", err)
}

// As unwraps err into an *AppError if possible.
func As(err error) (*AppError, bool) {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
