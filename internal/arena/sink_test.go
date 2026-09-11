package arena

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ardao/llm-replay/internal/dataset"
	"github.com/ardao/llm-replay/internal/domain"
)

func TestDatasetSinkSavesOnceAndUsesOnlyStoredComparison(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arena.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := OpenDatasetSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	comparison := Comparison{
		ID: "ar_test", CreatedAt: time.Now(),
		Request: domain.Request{Messages: []domain.Message{{Role: "user", Content: "original prompt"}}},
		Schema:  map[string]any{"type": "string"},
		Results: []ModelResult{{Model: "openai/left", Provider: "openai", Status: domain.StatusSuccess, Response: &SafeResponse{Content: "baseline"}}},
	}
	input := SaveRequest{ArenaResultID: comparison.ID, BaselineModel: "openai/left", UseResponseAsExpected: true, IncludeSchema: true}
	first, err := sink.Save(comparison, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sink.Save(comparison, input)
	if err != nil || second.RecordID != first.RecordID || second.DatasetRecordCount != 1 {
		t.Fatalf("save was not idempotent: first=%#v second=%#v err=%v", first, second, err)
	}
	var records []domain.Record
	if err := dataset.ReadFile(path, func(record domain.Record) error { records = append(records, record); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Request.Messages[0].Content != "original prompt" || records[0].Evaluation.Expected != "baseline" || records[0].Response.Raw != "" {
		t.Fatalf("unexpected saved record: %#v", records)
	}
}

func TestDatasetSinkRequiresExistingRegularJSONL(t *testing.T) {
	temp := t.TempDir()
	for _, path := range []string{filepath.Join(temp, "missing.jsonl"), filepath.Join(temp, "wrong.txt")} {
		if _, err := OpenDatasetSink(path); err == nil {
			t.Fatalf("unsafe dataset path accepted: %s", path)
		}
	}
}

func TestDatasetSinkRejectsBrokenDatasetAndSymlink(t *testing.T) {
	temp := t.TempDir()
	broken := filepath.Join(temp, "broken.jsonl")
	if err := os.WriteFile(broken, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDatasetSink(broken); err == nil {
		t.Fatal("broken dataset was accepted")
	}
	target := filepath.Join(temp, "target.jsonl")
	link := filepath.Join(temp, "link.jsonl")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if _, err := OpenDatasetSink(link); err == nil {
		t.Fatal("symlink dataset was accepted")
	}
}

func TestDatasetSinkSerializesConcurrentAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := OpenDatasetSink(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	const total = 12
	var wait sync.WaitGroup
	errorsSeen := make(chan error, total)
	for index := range total {
		wait.Add(1)
		go func() {
			defer wait.Done()
			id := fmt.Sprintf("ar_%d", index)
			comparison := Comparison{ID: id, CreatedAt: time.Now(), Request: domain.Request{Messages: []domain.Message{{Role: "user", Content: id}}}, Results: []ModelResult{{Model: "openai/left", Provider: "openai", Status: domain.StatusSuccess, Response: &SafeResponse{Content: id}}}}
			_, saveErr := sink.Save(comparison, SaveRequest{ArenaResultID: id, BaselineModel: "openai/left"})
			errorsSeen <- saveErr
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for saveErr := range errorsSeen {
		if saveErr != nil {
			t.Fatal(saveErr)
		}
	}
	count := 0
	if err := dataset.ReadFile(path, func(domain.Record) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	if count != total {
		t.Fatalf("record count = %d, want %d", count, total)
	}
}

func TestStoreExpiresAndBoundsResults(t *testing.T) {
	now := time.Now()
	store := NewStore()
	store.limit = 2
	store.ttl = time.Minute
	store.now = func() time.Time { return now }
	for _, id := range []string{"one", "two", "three"} {
		store.Put(Comparison{ID: id, CreatedAt: now})
	}
	if _, ok := store.Get("one"); ok {
		t.Fatal("oldest result was not evicted")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := store.Get("three"); ok {
		t.Fatal("expired result was not evicted")
	}
}
