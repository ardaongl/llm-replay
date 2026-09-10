package provider

import (
	"context"

	"github.com/ardao/llm-replay/internal/domain"
)

// Provider converts normalized requests into provider-specific API calls.
type Provider interface {
	Name() string
	Generate(ctx context.Context, request domain.Request) (*domain.Response, *domain.RecordMetrics, error)
}
