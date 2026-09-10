package domain

import (
	"errors"
	"fmt"
	"time"
)

const SchemaVersion = "1"

type Status string

const (
	StatusSuccess Status = "success"
	StatusError   Status = "error"
	StatusTimeout Status = "timeout"
)

// Record is one normalized request-response pair in a JSONL dataset.
type Record struct {
	SchemaVersion string              `json:"schema_version"`
	ID            string              `json:"id"`
	Timestamp     time.Time           `json:"timestamp"`
	Provider      string              `json:"provider"`
	Model         string              `json:"model"`
	Request       Request             `json:"request"`
	Response      Response            `json:"response"`
	Metrics       RecordMetrics       `json:"metrics"`
	Evaluation    *ExpectedEvaluation `json:"evaluation,omitempty"`
	Status        Status              `json:"status"`
}

// Request is the provider-independent request representation.
type Request struct {
	Messages    []Message      `json:"messages"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	MaxTokens   *int           `json:"max_tokens,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Message is a text-only chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response is the provider-independent response representation.
type Response struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason"`
	Raw          string `json:"raw,omitempty"`
}

// RecordMetrics contains metrics captured for a single request.
type RecordMetrics struct {
	LatencyMs    int64 `json:"latency_ms"`
	InputTokens  int   `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
}

// ExpectedEvaluation contains optional assertions for candidate responses.
type ExpectedEvaluation struct {
	Schema   map[string]any `json:"schema,omitempty"`
	Expected string         `json:"expected,omitempty"`
}

// Validate checks the stable dataset contract without applying provider rules.
func (r Record) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %q", SchemaVersion)
	}
	if r.ID == "" {
		return errors.New("id is required")
	}
	if r.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	if r.Provider == "" {
		return errors.New("provider is required")
	}
	if r.Model == "" {
		return errors.New("model is required")
	}
	if err := r.Request.Validate(); err != nil {
		return err
	}
	if r.Metrics.LatencyMs < 0 || r.Metrics.InputTokens < 0 || r.Metrics.OutputTokens < 0 {
		return errors.New("metrics cannot contain negative values")
	}
	switch r.Status {
	case StatusSuccess, StatusError, StatusTimeout:
	default:
		return fmt.Errorf("status %q is invalid", r.Status)
	}

	return nil
}

// Validate checks request fields shared by all provider adapters.
func (r Request) Validate() error {
	if len(r.Messages) == 0 {
		return errors.New("request.messages must contain at least one message")
	}
	for i, message := range r.Messages {
		switch message.Role {
		case "system", "user", "assistant":
		default:
			return fmt.Errorf("request.messages[%d].role %q is invalid", i, message.Role)
		}
	}
	if r.MaxTokens != nil && *r.MaxTokens <= 0 {
		return errors.New("request.max_tokens must be greater than zero")
	}
	return nil
}
