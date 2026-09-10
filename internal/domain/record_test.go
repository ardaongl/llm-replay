package domain

import (
	"strings"
	"testing"
	"time"
)

func TestRecordValidate(t *testing.T) {
	record := validRecord()
	if err := record.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
}

func TestRecordValidateRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Record)
		wantErr string
	}{
		{name: "schema version", mutate: func(r *Record) { r.SchemaVersion = "" }, wantErr: "schema_version"},
		{name: "id", mutate: func(r *Record) { r.ID = "" }, wantErr: "id is required"},
		{name: "messages", mutate: func(r *Record) { r.Request.Messages = nil }, wantErr: "at least one"},
		{name: "role", mutate: func(r *Record) { r.Request.Messages[0].Role = "tool" }, wantErr: "role"},
		{name: "status", mutate: func(r *Record) { r.Status = "unknown" }, wantErr: "status"},
		{name: "metrics", mutate: func(r *Record) { r.Metrics.InputTokens = -1 }, wantErr: "negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := validRecord()
			tt.mutate(&record)
			err := record.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func validRecord() Record {
	return Record{
		SchemaVersion: SchemaVersion,
		ID:            "rec-1",
		Timestamp:     time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC),
		Provider:      "openai",
		Model:         "gpt-test",
		Request: Request{Messages: []Message{
			{Role: "user", Content: "hello"},
		}},
		Response: Response{Content: "hi", FinishReason: "stop"},
		Metrics:  RecordMetrics{LatencyMs: 100, InputTokens: 2, OutputTokens: 1},
		Status:   StatusSuccess,
	}
}
