package provider

import (
	"errors"
	"testing"
)

func TestTypeOfAndIsRetryable(t *testing.T) {
	tests := []struct {
		errorType ErrorType
		retryable bool
	}{
		{ErrAuthentication, false},
		{ErrRateLimit, true},
		{ErrTimeout, true},
		{ErrProviderDown, true},
		{ErrInvalidRequest, false},
		{ErrNetwork, true},
		{ErrParse, false},
	}

	for _, tt := range tests {
		err := &Error{Type: tt.errorType, Message: "test"}
		if got := TypeOf(err); got != tt.errorType {
			t.Fatalf("TypeOf() = %q, want %q", got, tt.errorType)
		}
		if got := IsRetryable(err); got != tt.retryable {
			t.Fatalf("IsRetryable(%q) = %v, want %v", tt.errorType, got, tt.retryable)
		}
	}

	if got := TypeOf(errors.New("plain error")); got != ErrUnknown {
		t.Fatalf("TypeOf(plain error) = %q, want %q", got, ErrUnknown)
	}
}
