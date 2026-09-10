package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/metrics"
)

type DatasetSummary struct {
	Path               string
	FileSizeBytes      int64
	TotalRecords       int
	SuccessfulCaptures int
	FailedCaptures     int
	TimeoutCaptures    int
	InputTokensP50     int64
	InputTokensP95     int64
	InputTokensMax     int64
	OutputTokensP50    int64
	OutputTokensP95    int64
	OutputTokensMax    int64
	ValidJSONOutputs   int
	EvaluationRecords  int
}

func InspectDataset(path string) (DatasetSummary, error) {
	info, err := os.Stat(path)
	if err != nil {
		return DatasetSummary{}, fmt.Errorf("inspect dataset file: %w", err)
	}
	summary := DatasetSummary{Path: path, FileSizeBytes: info.Size()}
	var inputTokens []int64
	var outputTokens []int64
	err = dataset.ReadFile(path, func(record domain.Record) error {
		summary.TotalRecords++
		switch record.Status {
		case domain.StatusSuccess:
			summary.SuccessfulCaptures++
			inputTokens = append(inputTokens, int64(record.Metrics.InputTokens))
			outputTokens = append(outputTokens, int64(record.Metrics.OutputTokens))
			if json.Valid([]byte(record.Response.Content)) {
				summary.ValidJSONOutputs++
			}
		case domain.StatusTimeout:
			summary.TimeoutCaptures++
		default:
			summary.FailedCaptures++
		}
		if record.Evaluation != nil {
			summary.EvaluationRecords++
		}
		return nil
	})
	if err != nil {
		return DatasetSummary{}, err
	}
	sort.Slice(inputTokens, func(i, j int) bool { return inputTokens[i] < inputTokens[j] })
	sort.Slice(outputTokens, func(i, j int) bool { return outputTokens[i] < outputTokens[j] })
	summary.InputTokensP50 = metrics.Percentile(inputTokens, 0.50)
	summary.InputTokensP95 = metrics.Percentile(inputTokens, 0.95)
	summary.OutputTokensP50 = metrics.Percentile(outputTokens, 0.50)
	summary.OutputTokensP95 = metrics.Percentile(outputTokens, 0.95)
	if len(inputTokens) > 0 {
		summary.InputTokensMax = inputTokens[len(inputTokens)-1]
	}
	if len(outputTokens) > 0 {
		summary.OutputTokensMax = outputTokens[len(outputTokens)-1]
	}
	return summary, nil
}

func PrintDatasetSummary(output io.Writer, summary DatasetSummary) {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "Dataset Overview:\t%s\n", summary.Path)
	fmt.Fprintf(writer, "File Size:\t%s\n", formatBytes(summary.FileSizeBytes))
	fmt.Fprintf(writer, "Total Requests:\t%d\n", summary.TotalRecords)
	fmt.Fprintf(writer, "Successful Captures:\t%d\n", summary.SuccessfulCaptures)
	fmt.Fprintf(writer, "Failed Captures:\t%d\n", summary.FailedCaptures)
	fmt.Fprintf(writer, "Timeout Captures:\t%d\n", summary.TimeoutCaptures)
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Token Distribution\tP50\tP95\tMax")
	fmt.Fprintf(writer, "Input Tokens\t%d\t%d\t%d\n", summary.InputTokensP50, summary.InputTokensP95, summary.InputTokensMax)
	fmt.Fprintf(writer, "Output Tokens\t%d\t%d\t%d\n", summary.OutputTokensP50, summary.OutputTokensP95, summary.OutputTokensMax)
	fmt.Fprintln(writer)
	fmt.Fprintf(writer, "Valid JSON Outputs:\t%d (%s)\n", summary.ValidJSONOutputs, formatRate(summary.ValidJSONOutputs, summary.SuccessfulCaptures))
	fmt.Fprintf(writer, "Contains Evaluation:\t%d (%s)\n", summary.EvaluationRecords, formatRate(summary.EvaluationRecords, summary.TotalRecords))
	_ = writer.Flush()
}

func formatRate(value, total int) string {
	if total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", float64(value)/float64(total)*100)
}

func formatBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := int64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}
