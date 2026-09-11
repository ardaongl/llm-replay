package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ardao/llm-replay/internal/config"
	"github.com/ardao/llm-replay/internal/evaluation"
	"github.com/ardao/llm-replay/internal/pricing"
	"github.com/ardao/llm-replay/internal/provider"
	"github.com/ardao/llm-replay/internal/providerfactory"
	"github.com/ardao/llm-replay/internal/replay"
	"github.com/ardao/llm-replay/internal/report"
	"github.com/spf13/cobra"
)

func newInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <dataset.jsonl>",
		Short: "Inspect a dataset without replaying it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := report.InspectDataset(args[0])
			if err != nil {
				return err
			}
			report.PrintDatasetSummary(cmd.OutOrStdout(), summary)
			return nil
		},
	}
}

func newReplayCommand(globalConfig *config.Config) *cobra.Command {
	var models []string
	var concurrency int
	var timeout time.Duration
	var retries int
	var outputDir string
	var pricingPath string
	var evaluatorNames []string
	var openAIBaseURL string
	var anthropicBaseURL string

	command := &cobra.Command{
		Use:   "replay <dataset.jsonl>",
		Short: "Replay a dataset against one or more candidate models",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if len(models) == 0 {
				return errors.New("at least one --model is required")
			}
			if concurrency <= 0 {
				return errors.New("--concurrency must be greater than zero")
			}
			if timeout <= 0 {
				return errors.New("--timeout must be greater than zero")
			}
			if retries < 0 {
				return errors.New("--retries cannot be negative")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := pricing.LoadWithDefaults(pricingPath)
			if err != nil {
				return err
			}
			evaluators, err := selectEvaluators(evaluatorNames)
			if err != nil {
				return err
			}
			datasetSummary, err := report.InspectDataset(args[0])
			if err != nil {
				return err
			}

			specs := make([]replayCandidateSpec, 0, len(models))
			for _, modelID := range models {
				providerName, modelName, ok := strings.Cut(modelID, "/")
				if !ok || providerName == "" || modelName == "" {
					return fmt.Errorf("model %q must use provider/model format", modelID)
				}
				adapter, err := newProviderAdapter(providerName, modelName, globalConfig, openAIBaseURL, anthropicBaseURL, timeout)
				if err != nil {
					return err
				}
				specs = append(specs, replayCandidateSpec{id: modelID, adapter: adapter})
			}

			results := make(chan candidateReplayResult, len(specs))
			for index, spec := range specs {
				go func() {
					results <- runReplayCandidate(cmd.Context(), index, spec, args[0], datasetSummary.TotalRecords, replay.Config{
						Concurrency:    concurrency,
						RequestTimeout: timeout,
						MaxRetries:     retries,
						DisableRetries: retries == 0,
						OutputDir:      outputDir,
						Pricing:        &registry,
						Evaluators:     evaluators,
					})
				}()
			}

			orderedResults := make([]candidateReplayResult, len(specs))
			for range specs {
				result := <-results
				orderedResults[result.index] = result
			}
			var candidates []report.Candidate
			for _, result := range orderedResults {
				fmt.Fprint(cmd.OutOrStdout(), result.output)
				if result.err != nil {
					return result.err
				}
				candidates = append(candidates, result.candidates...)
			}

			fmt.Fprintln(cmd.OutOrStdout())
			report.PrintComparison(cmd.OutOrStdout(), candidates)
			return nil
		},
	}

	command.Flags().StringSliceVarP(&models, "model", "m", nil, "candidate model in provider/model format (repeatable)")
	command.Flags().IntVarP(&concurrency, "concurrency", "c", globalConfig.Concurrency, "number of concurrent requests")
	command.Flags().DurationVar(&timeout, "timeout", globalConfig.Timeout, "timeout per replay record")
	command.Flags().IntVar(&retries, "retries", 3, "maximum retries for transient errors")
	command.Flags().StringVarP(&outputDir, "output", "o", "runs", "directory for run artifacts")
	command.Flags().StringVar(&pricingPath, "pricing", "", "custom YAML pricing registry")
	command.Flags().StringSliceVar(&evaluatorNames, "eval", []string{"json", "schema", "match"}, "evaluators: json,schema,match or none")
	command.Flags().StringVar(&openAIBaseURL, "openai-base-url", "", "override the OpenAI API base URL")
	command.Flags().StringVar(&anthropicBaseURL, "anthropic-base-url", "", "override the Anthropic API base URL")
	return command
}

type replayCandidateSpec struct {
	id      string
	adapter provider.Provider
}

type candidateReplayResult struct {
	index      int
	output     string
	candidates []report.Candidate
	err        error
}

func newProviderAdapter(providerName, modelName string, globalConfig *config.Config, openAIBaseURL, anthropicBaseURL string, timeout time.Duration) (provider.Provider, error) {
	return providerfactory.New(providerName, modelName, providerfactory.Config{
		OpenAIAPIKey: globalConfig.OpenAIAPIKey, AnthropicAPIKey: globalConfig.AnthropicAPIKey,
		OpenAIBaseURL: openAIBaseURL, AnthropicBaseURL: anthropicBaseURL, Timeout: timeout,
	})
}

func runReplayCandidate(ctx context.Context, index int, spec replayCandidateSpec, datasetPath string, totalRecords int, replayConfig replay.Config) candidateReplayResult {
	var output bytes.Buffer
	fmt.Fprintf(&output, "\nModel: %s\n", spec.id)
	progress := report.NewProgress(&output, "Running replay", totalRecords)
	completed := 0
	if totalRecords == 0 {
		progress.Update(0)
	}
	replayConfig.OnResult = func(replay.Result) {
		completed++
		progress.Update(completed)
	}
	engine, err := replay.New(spec.adapter, spec.id, replayConfig)
	if err != nil {
		return candidateReplayResult{index: index, output: output.String(), err: err}
	}
	outcome, err := engine.Run(ctx, datasetPath)
	if err != nil {
		return candidateReplayResult{index: index, output: output.String(), err: fmt.Errorf("replay %s: %w", spec.id, err)}
	}
	candidates, err := report.LoadCandidates(outcome.RunDir)
	return candidateReplayResult{index: index, output: output.String(), candidates: candidates, err: err}
}

func newCompareCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <run-directory> [run-directory...]",
		Short: "Compare one or more completed replay runs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var candidates []report.Candidate
			for _, path := range args {
				loaded, err := report.LoadCandidates(path)
				if err != nil {
					return err
				}
				candidates = append(candidates, loaded...)
			}
			report.PrintComparison(cmd.OutOrStdout(), candidates)
			return nil
		},
	}
}

func selectEvaluators(names []string) ([]evaluation.Evaluator, error) {
	if len(names) == 1 && strings.EqualFold(strings.TrimSpace(names[0]), "none") {
		return []evaluation.Evaluator{}, nil
	}
	selected := make([]evaluation.Evaluator, 0, len(names))
	seen := make(map[string]bool)
	for _, name := range names {
		switch normalized := strings.ToLower(strings.TrimSpace(name)); normalized {
		case "json", evaluation.JSONValidName:
			if !seen[evaluation.JSONValidName] {
				selected = append(selected, evaluation.JSONValidity{})
				seen[evaluation.JSONValidName] = true
			}
		case "schema", evaluation.SchemaAdherenceName:
			if !seen[evaluation.SchemaAdherenceName] {
				selected = append(selected, evaluation.SchemaAdherence{})
				seen[evaluation.SchemaAdherenceName] = true
			}
		case "match", evaluation.ExactMatchName:
			if !seen[evaluation.ExactMatchName] {
				selected = append(selected, evaluation.ExactMatch{TrimSpace: true})
				seen[evaluation.ExactMatchName] = true
			}
		case "none":
			return nil, errors.New("--eval none cannot be combined with other evaluators")
		default:
			return nil, fmt.Errorf("unknown evaluator %q; use json, schema, match, or none", name)
		}
	}
	return selected, nil
}
