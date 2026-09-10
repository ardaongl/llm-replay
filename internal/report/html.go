package report

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardao/llm-replay/internal/artifact"
	"github.com/ardao/llm-replay/internal/ui"
)

type HTMLOptions struct {
	RunDirs      []string
	OutputPath   string
	MaxResults   int
	FailuresOnly bool
}

type HTMLResult struct {
	OutputPath string
	Included   int
	Total      int
	Truncated  bool
}

func GenerateHTML(options HTMLOptions) (HTMLResult, error) {
	if options.OutputPath == "" {
		return HTMLResult{}, errors.New("HTML output path is required")
	}
	if !strings.EqualFold(filepath.Ext(options.OutputPath), ".html") {
		return HTMLResult{}, errors.New("HTML output path must end with .html")
	}
	if options.MaxResults == 0 {
		options.MaxResults = 5000
	}
	if options.MaxResults < 0 {
		return HTMLResult{}, errors.New("max results cannot be negative")
	}
	comparison, err := artifact.LoadComparison(options.RunDirs)
	if err != nil {
		return HTMLResult{}, err
	}
	records := comparison.Records
	if options.FailuresOnly {
		filtered := make([]artifact.Record, 0)
		for _, record := range records {
			if artifact.IsFailure(record) {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}
	total := len(records)
	if len(records) > options.MaxResults {
		records = records[:options.MaxResults]
	}
	data := comparison.ReportData(records, total)
	html, err := ui.RenderPage(data, true)
	if err != nil {
		return HTMLResult{}, err
	}
	absOutput, err := filepath.Abs(options.OutputPath)
	if err != nil {
		return HTMLResult{}, fmt.Errorf("resolve HTML output: %w", err)
	}
	file, err := os.OpenFile(absOutput, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return HTMLResult{}, fmt.Errorf("create HTML report: %w", err)
	}
	if _, err := file.Write(html); err != nil {
		_ = file.Close()
		_ = os.Remove(absOutput)
		return HTMLResult{}, fmt.Errorf("write HTML report: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(absOutput)
		return HTMLResult{}, fmt.Errorf("flush HTML report: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(absOutput)
		return HTMLResult{}, fmt.Errorf("close HTML report: %w", err)
	}
	return HTMLResult{OutputPath: absOutput, Included: len(records), Total: total, Truncated: len(records) < total}, nil
}
