package evaluation

import (
	"context"
	"testing"

	"github.com/ardao/llm-replay/internal/domain"
)

func TestJSONValidity(t *testing.T) {
	evaluator := JSONValidity{}
	tests := []struct {
		name    string
		content string
		passed  bool
	}{
		{name: "object", content: `{"category":"billing"}`, passed: true},
		{name: "array", content: `[1,2,3]`, passed: true},
		{name: "truncated", content: `{"category":`, passed: false},
		{name: "empty", content: ``, passed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.Evaluate(context.Background(), nil, &domain.Response{Content: tt.content})
			if result.Passed != tt.passed || result.Skipped {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
	if result := evaluator.Evaluate(context.Background(), nil, nil); !result.Skipped {
		t.Fatalf("missing response was not skipped: %#v", result)
	}
}

func TestExactMatch(t *testing.T) {
	record := &domain.Record{Evaluation: &domain.ExpectedEvaluation{Expected: " Billing "}}
	strict := ExactMatch{TrimSpace: true}
	if result := strict.Evaluate(context.Background(), record, &domain.Response{Content: "Billing"}); !result.Passed {
		t.Fatalf("trimmed exact match failed: %#v", result)
	}
	if result := strict.Evaluate(context.Background(), record, &domain.Response{Content: "billing"}); result.Passed {
		t.Fatalf("case-sensitive exact match passed: %#v", result)
	}
	insensitive := ExactMatch{TrimSpace: true, CaseInsensitive: true}
	if result := insensitive.Evaluate(context.Background(), record, &domain.Response{Content: "billing"}); !result.Passed {
		t.Fatalf("case-insensitive exact match failed: %#v", result)
	}
	if result := strict.Evaluate(context.Background(), &domain.Record{}, &domain.Response{}); !result.Skipped {
		t.Fatalf("unconfigured exact match was not skipped: %#v", result)
	}
}

func TestSchemaAdherence(t *testing.T) {
	evaluator := SchemaAdherence{}
	record := &domain.Record{Evaluation: &domain.ExpectedEvaluation{Schema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{"type": "string"},
			"urgency":  map[string]any{"type": "string", "enum": []any{"low", "high"}},
		},
		"required":             []any{"category", "urgency"},
		"additionalProperties": false,
	}}}
	tests := []struct {
		name    string
		content string
		passed  bool
	}{
		{name: "valid", content: `{"category":"billing","urgency":"high"}`, passed: true},
		{name: "missing required", content: `{"category":"billing"}`, passed: false},
		{name: "wrong type", content: `{"category":42,"urgency":"high"}`, passed: false},
		{name: "invalid enum", content: `{"category":"billing","urgency":"medium"}`, passed: false},
		{name: "invalid JSON", content: `{"category":`, passed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.Evaluate(context.Background(), record, &domain.Response{Content: tt.content})
			if result.Passed != tt.passed || result.Skipped || result.Error != "" {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
}

func TestSchemaAdherenceSkipsMissingSchemaAndRejectsExternalRef(t *testing.T) {
	evaluator := SchemaAdherence{}
	if result := evaluator.Evaluate(context.Background(), &domain.Record{}, &domain.Response{Content: `{}`}); !result.Skipped {
		t.Fatalf("missing schema was not skipped: %#v", result)
	}
	record := &domain.Record{Evaluation: &domain.ExpectedEvaluation{Schema: map[string]any{"$ref": "https://example.com/schema.json"}}}
	result := evaluator.Evaluate(context.Background(), record, &domain.Response{Content: `{}`})
	if result.Error == "" {
		t.Fatalf("external schema reference was accepted: %#v", result)
	}
	record.Evaluation.Schema = map[string]any{"$dynamicRef": "other.json#item"}
	result = evaluator.Evaluate(context.Background(), record, &domain.Response{Content: `{}`})
	if result.Error == "" {
		t.Fatalf("external dynamic schema reference was accepted: %#v", result)
	}
}

func TestAggregatorExcludesSkippedResults(t *testing.T) {
	aggregator := NewAggregator()
	aggregator.Add([]Result{
		{Name: JSONValidName, Passed: true},
		{Name: JSONValidName, Passed: false},
		{Name: JSONValidName, Skipped: true},
		{Name: ExactMatchName, Skipped: true},
	})
	if rate, ok := aggregator.Rate(JSONValidName); !ok || rate != 0.5 {
		t.Fatalf("JSON valid rate = %v, %v; want 0.5, true", rate, ok)
	}
	if _, ok := aggregator.Rate(ExactMatchName); ok {
		t.Fatal("skipped exact match produced a rate")
	}
}
