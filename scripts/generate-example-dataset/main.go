package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ardao/llm-replay/internal/domain"
)

type scenario struct {
	message  string
	category string
	urgency  string
}

var scenarios = []scenario{
	{"My card was charged twice for this month's plan.", "billing", "high"},
	{"The invoice has the wrong company tax number.", "billing", "medium"},
	{"I do not recognize a charge that appeared this morning.", "billing", "high"},
	{"Where can I download invoices for the last quarter?", "billing", "low"},
	{"Our annual payment failed although the card is valid.", "billing", "high"},
	{"The dashboard stays blank after I sign in.", "technical_support", "high"},
	{"CSV export stops at 80 percent every time.", "technical_support", "medium"},
	{"Webhook deliveries have returned 500 errors since noon.", "technical_support", "high"},
	{"The mobile app does not show my latest projects.", "technical_support", "medium"},
	{"Dark mode flickers when opening the settings page.", "technical_support", "low"},
	{"Please cancel the workspace before the next renewal.", "cancellation", "medium"},
	{"I accidentally created a second subscription and need it closed.", "cancellation", "high"},
	{"We are shutting down this project and no longer need the plan.", "cancellation", "low"},
	{"Cancel my trial; I do not want it to convert to paid.", "cancellation", "medium"},
	{"I need to stop renewal today because the budget was frozen.", "cancellation", "high"},
	{"I forgot my password and the reset email never arrives.", "account_access", "high"},
	{"My authenticator phone was lost and I cannot pass two-factor login.", "account_access", "high"},
	{"A new teammate's invitation link says it has expired.", "account_access", "medium"},
	{"How do I change the owner of our workspace?", "account_access", "low"},
	{"My account is locked after several failed sign-in attempts.", "account_access", "medium"},
	{"Please refund the duplicate payment from yesterday.", "refund", "high"},
	{"I cancelled during the trial but was still billed; I need a refund.", "refund", "high"},
	{"Can the unused portion of our annual plan be refunded?", "refund", "medium"},
	{"The add-on did not work and I would like the purchase reversed.", "refund", "medium"},
	{"What is the status of the refund approved last week?", "refund", "low"},
	{"Can I move from monthly billing to an annual subscription?", "subscription", "low"},
	{"We need to add ten seats before tomorrow's onboarding.", "subscription", "high"},
	{"Please explain the difference between the Team and Business plans.", "subscription", "low"},
	{"Our seat count decreased but the subscription total did not change.", "subscription", "medium"},
	{"Can our plan be paused for two months?", "subscription", "medium"},
	{"Please add an audit-log export endpoint to the API.", "feature_request", "low"},
	{"We need SAML group mapping before our enterprise rollout.", "feature_request", "medium"},
	{"Could saved reports be scheduled for email delivery?", "feature_request", "low"},
	{"A bulk user import would save our operations team hours.", "feature_request", "medium"},
	{"Please support regional data residency for our compliance program.", "feature_request", "high"},
}

func main() {
	output := flag.String("output", "examples/datasets/support.jsonl", "dataset output path")
	flag.Parse()
	if err := generate(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create example directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create example dataset: %w", err)
	}
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	schema := supportSchema()
	startedAt := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)
	channels := []string{"email", "in-app chat", "support form", "partner portal"}
	for index := 0; index < 120; index++ {
		scenario := scenarios[index%len(scenarios)]
		content := fmt.Sprintf("Channel: %s. %s", channels[index%len(channels)], scenario.message)
		expected := fmt.Sprintf(`{"category":%q,"urgency":%q}`, scenario.category, scenario.urgency)
		record := domain.Record{
			SchemaVersion: domain.SchemaVersion,
			ID:            fmt.Sprintf("support-%03d", index+1),
			Timestamp:     startedAt.Add(time.Duration(index) * time.Minute),
			Provider:      "openai",
			Model:         "gpt-4o-mini",
			Request: domain.Request{Messages: []domain.Message{
				{Role: "system", Content: "Classify the support ticket. Return only JSON with category and urgency."},
				{Role: "user", Content: content},
			}},
			Response: domain.Response{Content: expected, FinishReason: "stop"},
			Metrics: domain.RecordMetrics{
				LatencyMs:    int64(180 + (index*37)%620),
				InputTokens:  32 + index%19,
				OutputTokens: 12 + index%5,
			},
			Evaluation: &domain.ExpectedEvaluation{Schema: schema, Expected: expected},
			Status:     domain.StatusSuccess,
		}
		if err := record.Validate(); err != nil {
			_ = file.Close()
			return fmt.Errorf("validate record %d: %w", index+1, err)
		}
		if err := encoder.Encode(record); err != nil {
			_ = file.Close()
			return fmt.Errorf("encode record %d: %w", index+1, err)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		return fmt.Errorf("flush example dataset: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close example dataset: %w", err)
	}
	return nil
}

func supportSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{
				"type": "string",
				"enum": []string{"account_access", "billing", "cancellation", "feature_request", "refund", "subscription", "technical_support"},
			},
			"urgency": map[string]any{
				"type": "string",
				"enum": []string{"low", "medium", "high"},
			},
		},
		"required":             []string{"category", "urgency"},
		"additionalProperties": false,
	}
}
