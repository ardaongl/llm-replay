package provider

import (
	"errors"
	"fmt"
	"time"
)

type ErrorType string

const (
	ErrAuthentication ErrorType = "authentication_error"
	ErrRateLimit      ErrorType = "rate_limit"
	ErrTimeout        ErrorType = "timeout"
	ErrProviderDown   ErrorType = "provider_error"
	ErrInvalidRequest ErrorType = "invalid_request"
	ErrNetwork        ErrorType = "network_error"
	ErrParse          ErrorType = "parse_error"
	ErrUnknown        ErrorType = "unknown"
)

// Error is the normalized error returned by every provider adapter.
type Error struct {
	Type       ErrorType
	StatusCode int
	Message    string
	RetryAfter time.Duration
	Underlying error
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("provider %s (HTTP %d): %s", e.Type, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("provider %s: %s", e.Type, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Underlying
}

// TypeOf extracts a normalized error type and falls back to ErrUnknown.
func TypeOf(err error) ErrorType {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return providerErr.Type
	}
	return ErrUnknown
}

// IsRetryable reports whether replay may safely retry the failed call.
func IsRetryable(err error) bool {
	switch TypeOf(err) {
	case ErrRateLimit, ErrTimeout, ErrProviderDown, ErrNetwork:
		return true
	default:
		return false
	}
}
