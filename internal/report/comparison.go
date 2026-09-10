package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ardao/llm-replay/internal/replay"
)

type Candidate struct {
	Name    string
	RunDir  string
	Summary replay.ModelSummary
}

func LoadCandidates(path string) ([]Candidate, error) {
	summaryPath := path
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect run path: %w", err)
	}
	if info.IsDir() {
		summaryPath = filepath.Join(path, "summary.json")
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil, fmt.Errorf("read run summary: %w", err)
	}
	var runSummary replay.RunSummary
	if err := json.Unmarshal(data, &runSummary); err != nil {
		return nil, fmt.Errorf("decode run summary: %w", err)
	}
	if len(runSummary.Models) == 0 {
		return nil, fmt.Errorf("run summary %q contains no models", summaryPath)
	}
	names := make([]string, 0, len(runSummary.Models))
	for name := range runSummary.Models {
		names = append(names, name)
	}
	sort.Strings(names)
	candidates := make([]Candidate, 0, len(names))
	for _, name := range names {
		candidates = append(candidates, Candidate{Name: name, RunDir: filepath.Dir(summaryPath), Summary: runSummary.Models[name]})
	}
	return candidates, nil
}

func PrintComparison(output io.Writer, candidates []Candidate) {
	if len(candidates) == 0 {
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "RESULTS COMPARISON")
	fmt.Fprint(writer, "Metric")
	for _, candidate := range candidates {
		fmt.Fprintf(writer, "\t%s", candidate.Name)
	}
	fmt.Fprintln(writer)
	writeFloatRow(writer, "Success Rate", candidates, func(c Candidate) (float64, bool) { return c.Summary.SuccessRate * 100, true }, "%0.1f%%")
	writeOptionalRateRow(writer, "JSON Validity", candidates, func(c Candidate) *float64 { return c.Summary.Evaluations.JSONValidRate })
	writeOptionalRateRow(writer, "Schema Adherence", candidates, func(c Candidate) *float64 { return c.Summary.Evaluations.SchemaAdherenceRate })
	writeOptionalRateRow(writer, "Exact Match", candidates, func(c Candidate) *float64 { return c.Summary.Evaluations.ExactMatchRate })
	writeFloatRow(writer, "Average Latency", candidates, func(c Candidate) (float64, bool) { return c.Summary.LatencyAvgMS, true }, "%.1f ms")
	writeFloatRow(writer, "P50 Latency", candidates, func(c Candidate) (float64, bool) { return float64(c.Summary.LatencyP50MS), true }, "%.0f ms")
	writeFloatRow(writer, "P95 Latency", candidates, func(c Candidate) (float64, bool) { return float64(c.Summary.LatencyP95MS), true }, "%.0f ms")
	writeFloatRow(writer, "Input Tokens", candidates, func(c Candidate) (float64, bool) { return float64(c.Summary.InputTokens), true }, "%.0f")
	writeFloatRow(writer, "Output Tokens", candidates, func(c Candidate) (float64, bool) { return float64(c.Summary.OutputTokens), true }, "%.0f")
	writeFloatRow(writer, "Estimated Cost", candidates, func(c Candidate) (float64, bool) { return c.Summary.EstimatedCostUSD, c.Summary.PricingAvailable }, "$%.6f")
	_ = writer.Flush()

	fmt.Fprintln(output, "\nRun artifacts:")
	for _, candidate := range candidates {
		fmt.Fprintf(output, "  %s: %s\n", candidate.Name, candidate.RunDir)
	}
}

func writeOptionalRateRow(writer io.Writer, label string, candidates []Candidate, value func(Candidate) *float64) {
	writeFloatRow(writer, label, candidates, func(candidate Candidate) (float64, bool) {
		rate := value(candidate)
		if rate == nil {
			return 0, false
		}
		return *rate * 100, true
	}, "%.1f%%")
}

func writeFloatRow(writer io.Writer, label string, candidates []Candidate, value func(Candidate) (float64, bool), format string) {
	fmt.Fprint(writer, label)
	baseline, baselineOK := value(candidates[0])
	for index, candidate := range candidates {
		current, ok := value(candidate)
		if !ok {
			fmt.Fprint(writer, "\tn/a")
			continue
		}
		cell := fmt.Sprintf(format, current)
		if index > 0 && baselineOK {
			cell += " " + relativeChange(current, baseline)
		}
		fmt.Fprintf(writer, "\t%s", cell)
	}
	fmt.Fprintln(writer)
}

func relativeChange(current, baseline float64) string {
	if baseline == 0 {
		if current == 0 {
			return "(0.0%)"
		}
		return "(n/a)"
	}
	change := (current - baseline) / baseline * 100
	return fmt.Sprintf("(%+.1f%%)", change)
}

type Progress struct {
	output io.Writer
	total  int
	label  string
}

func NewProgress(output io.Writer, label string, total int) *Progress {
	return &Progress{output: output, label: label, total: total}
}

func (p *Progress) Update(completed int) {
	const width = 30
	filled := 0
	if p.total > 0 {
		filled = completed * width / p.total
		if filled > width {
			filled = width
		}
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	fmt.Fprintf(p.output, "\r%s [%s] %d/%d", p.label, bar, completed, p.total)
	if completed >= p.total {
		fmt.Fprintln(p.output)
	}
}
