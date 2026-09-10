package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/domain"
)

func TestReaderValidJSONL(t *testing.T) {
	first := sampleRecord("rec-1")
	second := sampleRecord("rec-2")
	input := mustJSON(t, first) + "\n" + mustJSON(t, second) + "\n"
	reader := NewReader(strings.NewReader(input))

	gotFirst, err := reader.Next()
	if err != nil {
		t.Fatalf("read first record: %v", err)
	}
	gotSecond, err := reader.Next()
	if err != nil {
		t.Fatalf("read second record: %v", err)
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("final Next() error = %v, want io.EOF", err)
	}
	if !reflect.DeepEqual(gotFirst, first) || !reflect.DeepEqual(gotSecond, second) {
		t.Fatal("decoded records differ from input")
	}
}

func TestReaderCorruptedLineReportsLineNumber(t *testing.T) {
	input := mustJSON(t, sampleRecord("rec-1")) + "\n{broken-json}\n"
	reader := NewReader(strings.NewReader(input))
	if _, err := reader.Next(); err != nil {
		t.Fatalf("read first record: %v", err)
	}
	_, err := reader.Next()
	var lineErr *LineError
	if !errors.As(err, &lineErr) || lineErr.Line != 2 {
		t.Fatalf("Next() error = %v, want a line 2 error", err)
	}
}

func TestReaderRejectsInvalidRecord(t *testing.T) {
	record := sampleRecord("rec-invalid")
	record.SchemaVersion = "99"
	reader := NewReader(strings.NewReader(mustJSON(t, record) + "\n"))

	_, err := reader.Next()
	if err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("Next() error = %v, want schema validation error", err)
	}
}

func TestReadFileRequiresVisitor(t *testing.T) {
	err := ReadFile(filepath.Join(t.TempDir(), "unused.jsonl"), nil)
	if err == nil || !strings.Contains(err.Error(), "visitor") {
		t.Fatalf("ReadFile() error = %v, want visitor error", err)
	}
}

func TestWriterAppendAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dataset.jsonl")
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	want := []domain.Record{sampleRecord("rec-1"), sampleRecord("rec-2")}
	for _, record := range want {
		if err := writer.Append(record); err != nil {
			t.Fatalf("append record: %v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush writer: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var got []domain.Record
	if err := ReadFile(path, func(record domain.Record) error {
		got = append(got, record)
		return nil
	}); err != nil {
		t.Fatalf("read dataset: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip records differ\ngot:  %#v\nwant: %#v", got, want)
	}
	if err := writer.Append(sampleRecord("rec-3")); err == nil {
		t.Fatal("Append() succeeded after Close()")
	}
}

func TestWriterConcurrentAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dataset.jsonl")
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}

	const total = 50
	errCh := make(chan error, total)
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errCh <- writer.Append(sampleRecord(fmt.Sprintf("rec-%d", index)))
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent append: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	count := 0
	if err := ReadFile(path, func(domain.Record) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("read dataset: %v", err)
	}
	if count != total {
		t.Fatalf("record count = %d, want %d", count, total)
	}
}

func TestReaderStreamsThousandRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.jsonl")
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	for i := 0; i < 1000; i++ {
		if err := writer.Append(sampleRecord(fmt.Sprintf("rec-%d", i))); err != nil {
			t.Fatalf("append record %d: %v", i, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	count := 0
	if err := ReadFile(path, func(domain.Record) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("stream dataset: %v", err)
	}
	if count != 1000 {
		t.Fatalf("record count = %d, want 1000", count)
	}
}

func TestHashFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dataset.jsonl")
	if err := os.WriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := HashFile(path)
	if err != nil {
		t.Fatalf("hash file: %v", err)
	}
	const want = "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	if got != want {
		t.Fatalf("HashFile() = %q, want %q", got, want)
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dataset.meta.json")
	want := Metadata{
		SchemaVersion: "1",
		Name:          "support-production",
		CreatedAt:     time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		RecordCount:   1000,
		Source:        "capture",
		SHA256:        strings.Repeat("a", 64),
		Tags:          []string{"support", "production"},
	}
	if err := WriteMetadata(path, want); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	got, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata differs\ngot:  %#v\nwant: %#v", got, want)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(data)
}

func sampleRecord(id string) domain.Record {
	temperature := 0.2
	maxTokens := 128
	return domain.Record{
		SchemaVersion: domain.SchemaVersion,
		ID:            id,
		Timestamp:     time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		Provider:      "openai",
		Model:         "gpt-test",
		Request: domain.Request{
			Messages: []domain.Message{
				{Role: "system", Content: "Answer briefly."},
				{Role: "user", Content: "Hello"},
			},
			Temperature: &temperature,
			MaxTokens:   &maxTokens,
			Metadata:    map[string]any{"tenant": "test"},
		},
		Response: domain.Response{Content: "Hi", FinishReason: "stop"},
		Metrics:  domain.RecordMetrics{LatencyMs: 120, InputTokens: 8, OutputTokens: 2},
		Evaluation: &domain.ExpectedEvaluation{
			Expected: "Hi",
		},
		Status: domain.StatusSuccess,
	}
}
