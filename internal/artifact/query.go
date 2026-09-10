package artifact

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/ardao/llm-replay/internal/domain"
)

func (c *Comparison) Page(query Query, cursor string, limit int) (Page, error) {
	if limit <= 0 || limit > 200 {
		return Page{}, errorsNewLimit(limit)
	}
	start, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	matching := make([]Record, 0)
	for _, record := range c.Records {
		if matches(record, query) {
			matching = append(matching, record)
		}
	}
	if start > len(matching) {
		return Page{}, fmt.Errorf("cursor is outside result set")
	}
	end := start + limit
	if end > len(matching) {
		end = len(matching)
	}
	page := Page{Total: len(matching), Records: make([]RecordView, 0, end-start)}
	for _, record := range matching[start:end] {
		page.Records = append(page.Records, recordView(record))
	}
	if end < len(matching) {
		page.NextCursor = encodeCursor(end)
	}
	return page, nil
}

func (c *Comparison) Record(id string) (RecordView, bool) {
	index := sortSearch(c.Records, id)
	if index >= len(c.Records) || c.Records[index].ID != id {
		return RecordView{}, false
	}
	return recordView(c.Records[index]), true
}

func matches(record Record, query Query) bool {
	if query.Search != "" {
		needle := strings.ToLower(query.Search)
		if !strings.Contains(strings.ToLower(record.ID), needle) && !requestContains(record, needle) {
			return false
		}
	}
	matchedModel := false
	for _, model := range record.Models {
		if query.Model != "" && model.Name != query.Model {
			continue
		}
		matchedModel = true
		if query.Status != "" && string(model.Result.Status) != query.Status {
			continue
		}
		if query.MinLatencyMS > 0 && model.Result.Metrics.LatencyMs < query.MinLatencyMS {
			continue
		}
		if query.Evaluator != "" {
			found := false
			for _, evaluation := range model.Result.Evaluations {
				if !evaluatorMatches(evaluation.Name, query.Evaluator) {
					continue
				}
				if query.Passed == nil || evaluation.Passed == *query.Passed {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		return true
	}
	return matchedModel && query.Status == "" && query.MinLatencyMS == 0 && query.Evaluator == ""
}

func requestContains(record Record, needle string) bool {
	for _, message := range record.Request.Messages {
		if strings.Contains(strings.ToLower(message.Content), needle) {
			return true
		}
	}
	return false
}

func encodeCursor(index int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(index)))
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor")
	}
	index, err := strconv.Atoi(string(decoded))
	if err != nil || index < 0 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return index, nil
}

func errorsNewLimit(limit int) error {
	return fmt.Errorf("limit must be between 1 and 200, got %d", limit)
}

func sortSearch(records []Record, id string) int {
	low, high := 0, len(records)
	for low < high {
		middle := int(uint(low+high) >> 1)
		if records[middle].ID < id {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low
}

func ValidStatus(value string) bool {
	switch value {
	case "", string(domain.StatusSuccess), string(domain.StatusError), string(domain.StatusTimeout):
		return true
	default:
		return false
	}
}

func ValidEvaluator(value string) bool {
	if value == "" {
		return true
	}
	for _, candidate := range []string{"json", "schema", "match", "json_valid", "schema_adherence", "exact_match"} {
		if value == candidate {
			return true
		}
	}
	return false
}

func (c *Comparison) HasModel(name string) bool {
	if name == "" {
		return true
	}
	for _, run := range c.Runs {
		if run.Name == name {
			return true
		}
	}
	return false
}
