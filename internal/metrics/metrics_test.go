package metrics

import (
	"math"
	"testing"

	"github.com/ardao/llm-replay/internal/domain"
)

func TestPercentileP50P95(t *testing.T) {
	values := make([]int64, 100)
	for i := range values {
		values[i] = int64(i + 1)
	}
	if got := Percentile(values, 0.50); got != 50 {
		t.Fatalf("P50 = %d, want 50", got)
	}
	if got := Percentile(values, 0.95); got != 95 {
		t.Fatalf("P95 = %d, want 95", got)
	}
	if got := Percentile(nil, 0.95); got != 0 {
		t.Fatalf("empty percentile = %d, want 0", got)
	}
}

func TestAggregator(t *testing.T) {
	var aggregator Aggregator
	aggregator.Add(domain.StatusSuccess, domain.RecordMetrics{LatencyMs: 100, InputTokens: 10, OutputTokens: 5})
	aggregator.Add(domain.StatusSuccess, domain.RecordMetrics{LatencyMs: 300, InputTokens: 20, OutputTokens: 7})
	aggregator.Add(domain.StatusError, domain.RecordMetrics{LatencyMs: 9000, InputTokens: 999, OutputTokens: 999})
	aggregator.Add(domain.StatusTimeout, domain.RecordMetrics{LatencyMs: 8000, InputTokens: 888, OutputTokens: 888})

	summary := aggregator.Summary()
	if summary.TotalRequests != 4 || summary.SuccessCount != 2 || summary.ErrorCount != 1 || summary.TimeoutCount != 1 {
		t.Fatalf("unexpected reliability counts: %#v", summary)
	}
	if summary.SuccessRate != 0.5 || summary.ErrorRate != 0.25 || summary.TimeoutRate != 0.25 {
		t.Fatalf("unexpected reliability rates: %#v", summary)
	}
	if summary.LatencyAvgMS != 200 || summary.LatencyP50MS != 100 || summary.LatencyP95MS != 300 {
		t.Fatalf("unexpected latency metrics: %#v", summary)
	}
	if summary.InputTokens != 30 || summary.OutputTokens != 12 || summary.TotalTokens != 42 {
		t.Fatalf("failed requests affected token totals: %#v", summary)
	}
}

func TestAggregatorEmpty(t *testing.T) {
	summary := new(Aggregator).Summary()
	if summary.TotalRequests != 0 || math.IsNaN(summary.SuccessRate) || math.IsNaN(summary.LatencyAvgMS) {
		t.Fatalf("unexpected empty summary: %#v", summary)
	}
}
