package arena

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
)

type DatasetSink struct {
	mu     sync.Mutex
	writer *dataset.Writer
	ids    map[string]struct{}
	saved  map[string]SaveResponse
	count  int
}

func OpenDatasetSink(path string) (*DatasetSink, error) {
	if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return nil, errors.New("arena dataset must use the .jsonl extension")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve arena dataset: %w", err)
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		return nil, fmt.Errorf("inspect arena dataset: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("arena dataset must be an existing regular file, not a symlink")
	}
	canonical, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("resolve arena dataset: %w", err)
	}
	ids := make(map[string]struct{})
	count := 0
	if err := dataset.ReadFile(canonical, func(record domain.Record) error {
		if _, duplicate := ids[record.ID]; duplicate {
			return fmt.Errorf("duplicate record id %q", record.ID)
		}
		ids[record.ID] = struct{}{}
		count++
		return nil
	}); err != nil {
		return nil, fmt.Errorf("validate arena dataset: %w", err)
	}
	writer, err := dataset.NewWriter(canonical)
	if err != nil {
		return nil, err
	}
	return &DatasetSink{writer: writer, ids: ids, saved: make(map[string]SaveResponse), count: count}, nil
}

func (s *DatasetSink) Save(comparison Comparison, input SaveRequest) (SaveResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.saved[input.ArenaResultID]; ok {
		return previous, nil
	}
	var selected *ModelResult
	for index := range comparison.Results {
		if comparison.Results[index].Model == input.BaselineModel {
			selected = &comparison.Results[index]
			break
		}
	}
	if selected == nil || selected.Status != domain.StatusSuccess || selected.Response == nil {
		return SaveResponse{}, errors.New("baseline_model must select a successful arena result")
	}
	recordID, err := s.uniqueRecordID()
	if err != nil {
		return SaveResponse{}, err
	}
	var expected *domain.ExpectedEvaluation
	if input.UseResponseAsExpected || (input.IncludeSchema && len(comparison.Schema) > 0) {
		expected = &domain.ExpectedEvaluation{}
		if input.UseResponseAsExpected {
			expected.Expected = selected.Response.Content
		}
		if input.IncludeSchema {
			expected.Schema = comparison.Schema
		}
	}
	record := domain.Record{
		SchemaVersion: domain.SchemaVersion,
		ID:            recordID, Timestamp: time.Now().UTC(), Provider: selected.Provider, Model: selected.Model,
		Request:  comparison.Request,
		Response: domain.Response{Content: selected.Response.Content, FinishReason: selected.Response.FinishReason},
		Metrics:  selected.Metrics, Evaluation: expected, Status: domain.StatusSuccess,
	}
	if err := s.writer.Append(record); err != nil {
		return SaveResponse{}, err
	}
	if err := s.writer.Flush(); err != nil {
		return SaveResponse{}, err
	}
	s.ids[recordID] = struct{}{}
	s.count++
	response := SaveResponse{Status: "saved", RecordID: recordID, DatasetRecordCount: s.count}
	s.saved[input.ArenaResultID] = response
	return response, nil
}

func (s *DatasetSink) uniqueRecordID() (string, error) {
	for range 10 {
		id, err := randomID("rec_")
		if err != nil {
			return "", err
		}
		if _, exists := s.ids[id]; !exists {
			return id, nil
		}
	}
	return "", errors.New("could not generate a unique record id")
}

func (s *DatasetSink) Close() error {
	return s.writer.Close()
}
