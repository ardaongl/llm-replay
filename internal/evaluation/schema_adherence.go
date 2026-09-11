package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardao/llm-replay/internal/domain"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type SchemaAdherence struct{}

func (SchemaAdherence) Name() string {
	return SchemaAdherenceName
}

func (e SchemaAdherence) Evaluate(ctx context.Context, record *domain.Record, candidate *domain.Response) Result {
	if err := ctx.Err(); err != nil {
		return Result{Name: e.Name(), Error: err.Error()}
	}
	if record == nil || record.Evaluation == nil || len(record.Evaluation.Schema) == 0 {
		return skipped(e.Name(), "JSON schema is not configured")
	}
	if candidate == nil {
		return skipped(e.Name(), "candidate response is unavailable")
	}
	schema, err := CompileSchema(record.Evaluation.Schema)
	if err != nil {
		return Result{Name: e.Name(), Error: err.Error()}
	}
	instance, err := jsonschema.UnmarshalJSON(strings.NewReader(candidate.Content))
	if err != nil {
		return Result{
			Name:    e.Name(),
			Details: map[string]any{"reason": "candidate content is not valid JSON"},
		}
	}
	if err := schema.Validate(instance); err != nil {
		return Result{
			Name:    e.Name(),
			Details: map[string]any{"reason": err.Error()},
		}
	}
	return Result{Name: e.Name(), Passed: true, Score: 1}
}

// CompileSchema validates and compiles a local-only JSON Schema.
func CompileSchema(value map[string]any) (*jsonschema.Schema, error) {
	if err := rejectExternalReferences(value); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", value); err != nil {
		return nil, fmt.Errorf("load JSON schema: %v", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile JSON schema: %v", err)
	}
	return schema, nil
}

func rejectExternalReferences(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef" {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return fmt.Errorf("external JSON schema reference %q is not allowed", ref)
				}
			}
			if err := rejectExternalReferences(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectExternalReferences(child); err != nil {
				return err
			}
		}
	}
	return nil
}
