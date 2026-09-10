package metrics

import (
	"math"
	"sort"

	"github.com/ardao/llm-replay/internal/domain"
)

type Summary struct {
	TotalRequests int     `json:"total_requests"`
	SuccessCount  int     `json:"success_count"`
	ErrorCount    int     `json:"error_count"`
	TimeoutCount  int     `json:"timeout_count"`
	SuccessRate   float64 `json:"success_rate"`
	ErrorRate     float64 `json:"error_rate"`
	TimeoutRate   float64 `json:"timeout_rate"`
	LatencyAvgMS  float64 `json:"latency_avg_ms"`
	LatencyP50MS  int64   `json:"latency_p50_ms"`
	LatencyP95MS  int64   `json:"latency_p95_ms"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
}

// Aggregator incrementally collects run metrics. Only successful requests
// contribute latency and token usage; failed calls only affect reliability.
type Aggregator struct {
	totalRequests int
	successCount  int
	errorCount    int
	timeoutCount  int
	latencyTotal  int64
	latencies     []int64
	inputTokens   int64
	outputTokens  int64
}

func (a *Aggregator) Add(status domain.Status, value domain.RecordMetrics) {
	a.totalRequests++
	switch status {
	case domain.StatusSuccess:
		a.successCount++
		a.latencyTotal += value.LatencyMs
		a.latencies = append(a.latencies, value.LatencyMs)
		a.inputTokens += int64(value.InputTokens)
		a.outputTokens += int64(value.OutputTokens)
	case domain.StatusTimeout:
		a.timeoutCount++
	default:
		a.errorCount++
	}
}

func (a *Aggregator) Summary() Summary {
	result := Summary{
		TotalRequests: a.totalRequests,
		SuccessCount:  a.successCount,
		ErrorCount:    a.errorCount,
		TimeoutCount:  a.timeoutCount,
		InputTokens:   a.inputTokens,
		OutputTokens:  a.outputTokens,
		TotalTokens:   a.inputTokens + a.outputTokens,
	}
	if a.totalRequests > 0 {
		denominator := float64(a.totalRequests)
		result.SuccessRate = float64(a.successCount) / denominator
		result.ErrorRate = float64(a.errorCount) / denominator
		result.TimeoutRate = float64(a.timeoutCount) / denominator
	}
	if a.successCount > 0 {
		result.LatencyAvgMS = float64(a.latencyTotal) / float64(a.successCount)
		ordered := append([]int64(nil), a.latencies...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
		result.LatencyP50MS = Percentile(ordered, 0.50)
		result.LatencyP95MS = Percentile(ordered, 0.95)
	}
	return result
}

// Percentile uses the nearest-rank method and expects sorted values.
func Percentile(sortedValues []int64, percentile float64) int64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sortedValues[0]
	}
	if percentile >= 1 {
		return sortedValues[len(sortedValues)-1]
	}
	rank := int(math.Ceil(float64(len(sortedValues)) * percentile))
	if rank < 1 {
		rank = 1
	}
	return sortedValues[rank-1]
}
