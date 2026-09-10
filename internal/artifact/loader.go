package artifact

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/replay"
)

const (
	maxResultLineSize         = 64 * 1024 * 1024
	maxComparisonArtifactSize = 512 * 1024 * 1024
)

func LoadComparison(paths []string) (*Comparison, error) {
	if len(paths) == 0 {
		return nil, errors.New("at least one run directory is required")
	}
	comparison := &Comparison{}
	models := make(map[string]struct{})
	var artifactBytes int64
	for _, path := range paths {
		size, err := runArtifactSize(path)
		if err != nil {
			return nil, err
		}
		artifactBytes += size
		if artifactBytes > maxComparisonArtifactSize {
			return nil, fmt.Errorf("comparison artifacts exceed the 512 MiB safety limit")
		}
		run, datasetHash, order, err := loadRun(path)
		if err != nil {
			return nil, err
		}
		if comparison.DatasetSHA256 == "" {
			comparison.DatasetSHA256 = datasetHash
		} else if datasetHash != comparison.DatasetSHA256 {
			return nil, fmt.Errorf("run %q uses dataset %s; expected %s", path, datasetHash, comparison.DatasetSHA256)
		}
		if _, exists := models[run.Name]; exists {
			return nil, fmt.Errorf("duplicate model %q in comparison", run.Name)
		}
		models[run.Name] = struct{}{}
		comparison.Runs = append(comparison.Runs, run)
		if len(comparison.Runs) == 1 {
			comparison.Records = make([]Record, 0, len(order))
			for _, id := range order {
				result := run.Results[id]
				comparison.Records = append(comparison.Records, Record{
					ID:       id,
					Request:  result.Request,
					Baseline: result.BaselineResponse,
					Models:   []ModelResult{{Name: run.Name, Result: result}},
				})
			}
			continue
		}
		if err := joinRun(comparison, run); err != nil {
			return nil, err
		}
	}
	sort.Slice(comparison.Records, func(i, j int) bool { return comparison.Records[i].ID < comparison.Records[j].ID })
	return comparison, nil
}

func loadRun(path string) (Run, string, []string, error) {
	absPath, err := resolveRunDir(path)
	if err != nil {
		return Run{}, "", nil, err
	}
	var config replay.RunConfig
	if err := readJSON(filepath.Join(absPath, "config.json"), &config); err != nil {
		return Run{}, "", nil, err
	}
	var summary replay.RunSummary
	if err := readJSON(filepath.Join(absPath, "summary.json"), &summary); err != nil {
		return Run{}, "", nil, err
	}
	if config.SchemaVersion != domain.SchemaVersion || summary.SchemaVersion != domain.SchemaVersion {
		return Run{}, "", nil, fmt.Errorf("run %q uses unsupported schema version", path)
	}
	if config.DatasetSHA256 == "" || summary.DatasetSHA256 == "" || config.DatasetSHA256 != summary.DatasetSHA256 {
		return Run{}, "", nil, fmt.Errorf("run %q has inconsistent dataset hashes", path)
	}
	if config.Model == "" {
		return Run{}, "", nil, fmt.Errorf("run %q has no model in config.json", path)
	}
	modelSummary, ok := summary.Models[config.Model]
	if !ok || len(summary.Models) != 1 {
		return Run{}, "", nil, fmt.Errorf("run %q summary must contain exactly configured model %q", path, config.Model)
	}
	results, order, err := readResults(filepath.Join(absPath, "results.jsonl"), config.Model)
	if err != nil {
		return Run{}, "", nil, err
	}
	if summary.TotalRequests != len(results) {
		return Run{}, "", nil, fmt.Errorf("run %q summary contains %d requests but results contains %d", path, summary.TotalRequests, len(results))
	}
	return Run{Dir: absPath, Name: config.Model, Config: config, Summary: modelSummary, Results: results}, config.DatasetSHA256, order, nil
}

func resolveRunDir(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve run path %q: %w", path, err)
	}
	canonical, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve run path %q: %w", path, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect run path %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("run path %q is not a directory", path)
	}
	return canonical, nil
}

func runArtifactSize(path string) (int64, error) {
	directory, err := resolveRunDir(path)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, name := range []string{"config.json", "summary.json", "results.jsonl"} {
		info, statErr := os.Stat(filepath.Join(directory, name))
		if statErr != nil {
			return 0, fmt.Errorf("inspect artifact %q: %w", filepath.Join(directory, name), statErr)
		}
		if !info.Mode().IsRegular() {
			return 0, fmt.Errorf("artifact %q is not a regular file", filepath.Join(directory, name))
		}
		total += info.Size()
	}
	return total, nil
}

func readJSON(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read artifact %q: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode artifact %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode artifact %q: trailing JSON data", path)
	}
	return nil
}

func readResults(path, expectedModel string) (map[string]replay.Result, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open artifact %q: %w", path, err)
	}
	defer file.Close()
	results := make(map[string]replay.Result)
	var order []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxResultLineSize)
	line := 0
	for scanner.Scan() {
		line++
		var result replay.Result
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			return nil, nil, fmt.Errorf("decode %s line %d: %w", path, line, err)
		}
		if result.SchemaVersion != domain.SchemaVersion || result.RecordID == "" {
			return nil, nil, fmt.Errorf("invalid result at %s line %d", path, line)
		}
		if !ValidStatus(string(result.Status)) || result.Status == "" {
			return nil, nil, fmt.Errorf("result %q has invalid status %q", result.RecordID, result.Status)
		}
		modelName := result.Model
		if !strings.Contains(modelName, "/") {
			modelName = result.Provider + "/" + modelName
		}
		if modelName != expectedModel {
			return nil, nil, fmt.Errorf("result %q uses model %q; expected %q", result.RecordID, modelName, expectedModel)
		}
		if _, exists := results[result.RecordID]; exists {
			return nil, nil, fmt.Errorf("duplicate record_id %q in %s", result.RecordID, path)
		}
		results[result.RecordID] = result
		order = append(order, result.RecordID)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read artifact %q: %w", path, err)
	}
	return results, order, nil
}

func joinRun(comparison *Comparison, run Run) error {
	known := make(map[string]int, len(comparison.Records))
	for index, record := range comparison.Records {
		known[record.ID] = index
	}
	var missing []string
	for id := range known {
		if _, exists := run.Results[id]; !exists {
			missing = append(missing, id)
		}
	}
	var extra []string
	for id := range run.Results {
		if _, exists := known[id]; !exists {
			extra = append(extra, id)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		sort.Strings(missing)
		sort.Strings(extra)
		return fmt.Errorf("run %q record set differs: missing=%v extra=%v", run.Dir, missing, extra)
	}
	for id, index := range known {
		result := run.Results[id]
		if !reflect.DeepEqual(comparison.Records[index].Request, result.Request) ||
			!reflect.DeepEqual(comparison.Records[index].Baseline, result.BaselineResponse) {
			return fmt.Errorf("run %q record %q does not match the request and baseline in the first run", run.Dir, id)
		}
		comparison.Records[index].Models = append(comparison.Records[index].Models, ModelResult{Name: run.Name, Result: result})
	}
	return nil
}

func (c *Comparison) ReportData(records []Record, total int) ReportData {
	data := ReportData{
		DatasetSHA256: c.DatasetSHA256,
		TotalRecords:  total,
		Included:      len(records),
		Truncated:     len(records) < total,
		Runs:          make([]RunView, 0, len(c.Runs)),
		Latency:       latencyChart(records, c.Runs),
		Records:       make([]RecordView, 0, len(records)),
	}
	for _, run := range c.Runs {
		data.Runs = append(data.Runs, RunView{Name: run.Name, Config: run.Config, Summary: run.Summary})
	}
	for _, record := range records {
		data.Records = append(data.Records, recordView(record))
	}
	return data
}

func recordView(record Record) RecordView {
	view := RecordView{ID: record.ID, Request: record.Request, Baseline: record.Baseline, Models: make([]ModelResultView, 0, len(record.Models))}
	for _, model := range record.Models {
		evaluations := make([]EvaluationView, 0, len(model.Result.Evaluations))
		for _, result := range model.Result.Evaluations {
			evaluations = append(evaluations, EvaluationView{
				Name: result.Name, Passed: result.Passed, Skipped: result.Skipped, Score: result.Score, Details: result.Details, Error: result.Error,
			})
		}
		view.Models = append(view.Models, ModelResultView{
			Name: model.Name, Status: model.Result.Status, Candidate: model.Result.CandidateResponse,
			Metrics: model.Result.Metrics, Evaluations: evaluations, Error: model.Result.Error,
		})
	}
	return view
}

func latencyChart(records []Record, runs []Run) LatencyChart {
	const bins = 12
	chart := LatencyChart{Bins: bins, Series: make([]LatencySeries, len(runs))}
	latencies := make(map[string][]int64, len(runs))
	for index, run := range runs {
		chart.Series[index] = LatencySeries{Name: run.Name, Counts: make([]int, bins)}
	}
	for _, record := range records {
		for _, model := range record.Models {
			if model.Result.Status == domain.StatusTimeout {
				for index := range chart.Series {
					if chart.Series[index].Name == model.Name {
						chart.Series[index].TimeoutCount++
					}
				}
			}
			if model.Result.Status == domain.StatusSuccess {
				latencies[model.Name] = append(latencies[model.Name], model.Result.Metrics.LatencyMs)
				if model.Result.Metrics.LatencyMs > chart.MaxMS {
					chart.MaxMS = model.Result.Metrics.LatencyMs
				}
			}
		}
	}
	if chart.MaxMS == 0 {
		chart.MaxMS = 1
	}
	for _, record := range records {
		for _, model := range record.Models {
			if model.Result.Status != domain.StatusSuccess {
				continue
			}
			for index := range chart.Series {
				if chart.Series[index].Name == model.Name {
					bin := int(model.Result.Metrics.LatencyMs * bins / (chart.MaxMS + 1))
					chart.Series[index].Counts[bin]++
					break
				}
			}
		}
	}
	for index := range chart.Series {
		values := latencies[chart.Series[index].Name]
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		chart.Series[index].P50MS = percentile(values, 0.50)
		chart.Series[index].P90MS = percentile(values, 0.90)
		chart.Series[index].P95MS = percentile(values, 0.95)
		chart.Series[index].P99MS = percentile(values, 0.99)
	}
	return chart
}

func percentile(values []int64, quantile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1)*quantile + 0.5)
	return values[index]
}

func IsFailure(record Record) bool {
	for _, model := range record.Models {
		if model.Result.Status != domain.StatusSuccess {
			return true
		}
		for _, result := range model.Result.Evaluations {
			if !result.Skipped && (!result.Passed || result.Error != "") {
				return true
			}
		}
	}
	return false
}

func evaluatorMatches(resultName, queryName string) bool {
	queryName = strings.ToLower(queryName)
	aliases := map[string]string{"json": evaluation.JSONValidName, "schema": evaluation.SchemaAdherenceName, "match": evaluation.ExactMatchName}
	if alias, ok := aliases[queryName]; ok {
		queryName = alias
	}
	return resultName == queryName
}
