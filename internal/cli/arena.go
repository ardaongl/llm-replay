package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ardao/llm-replay/internal/arena"
	"github.com/ardao/llm-replay/internal/config"
	"github.com/ardao/llm-replay/internal/pricing"
	"github.com/ardao/llm-replay/internal/providerfactory"
	"github.com/ardao/llm-replay/internal/ui"
	"github.com/spf13/cobra"
)

func newArenaCommand(globalConfig *config.Config) *cobra.Command {
	var host string
	var port int
	var timeout time.Duration
	var pricingPath string
	var datasetPath string
	var noBrowser bool
	var openAIBaseURL string
	var anthropicBaseURL string

	command := &cobra.Command{
		Use:   "arena",
		Short: "Compare two live models in a secure local web UI",
		Args:  cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if err := ui.ValidateHost(host); err != nil {
				return err
			}
			if port < 0 || port > 65535 {
				return errors.New("--port must be between 0 and 65535")
			}
			if timeout <= 0 || timeout > time.Minute {
				return errors.New("--timeout must be greater than zero and no more than 60s")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			registry, err := pricing.LoadWithDefaults(pricingPath)
			if err != nil {
				return err
			}
			providerConfig := providerfactory.Config{
				OpenAIAPIKey: globalConfig.OpenAIAPIKey, AnthropicAPIKey: globalConfig.AnthropicAPIKey,
				OpenAIBaseURL: openAIBaseURL, AnthropicBaseURL: anthropicBaseURL, Timeout: timeout,
			}
			specs := make([]arena.ModelSpec, 0, len(defaultArenaModels))
			for _, catalogModel := range defaultArenaModels {
				modelID := catalogModel.provider + "/" + catalogModel.model
				providerName := catalogModel.provider
				spec := arena.ModelSpec{ID: modelID, Provider: providerName, Available: providerfactory.Available(providerName, providerConfig)}
				if spec.Available {
					spec.Adapter, err = providerfactory.New(providerName, catalogModel.model, providerConfig)
					if err != nil {
						return err
					}
				}
				specs = append(specs, spec)
			}
			runner, err := arena.NewRunner(specs, registry, timeout)
			if err != nil {
				return err
			}

			var sink *arena.DatasetSink
			if datasetPath != "" {
				sink, err = arena.OpenDatasetSink(datasetPath)
				if err != nil {
					return err
				}
				defer sink.Close()
			}

			listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err != nil {
				return fmt.Errorf("start live arena: %w", err)
			}
			defer listener.Close()
			targetURL := "http://" + listener.Addr().String()
			handler, err := arena.NewServer(arena.ServerConfig{Origin: targetURL, Runner: runner, Sink: sink})
			if err != nil {
				return err
			}

			server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			shutdownDone := make(chan struct{})
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "arena shutdown warning: %v\n", shutdownErr)
				}
				close(shutdownDone)
			}()

			fmt.Fprintf(cmd.OutOrStdout(), "LLM Replay Live Arena: %s/\n", targetURL)
			if runner.AvailableCount() < 2 {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: fewer than two catalog models are available; comparison is disabled")
			}
			if !noBrowser {
				if browserErr := ui.OpenBrowser(targetURL + "/"); browserErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "browser warning: %v\n", browserErr)
				}
			}
			err = server.Serve(listener)
			stop()
			<-shutdownDone
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("run live arena: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&host, "host", "127.0.0.1", "loopback host to bind")
	command.Flags().IntVar(&port, "port", 0, "local port; 0 selects a free port")
	command.Flags().DurationVar(&timeout, "timeout", globalConfig.Timeout, "timeout for each provider request (maximum 60s)")
	command.Flags().StringVar(&pricingPath, "pricing", "", "custom YAML pricing registry")
	command.Flags().StringVar(&datasetPath, "dataset", "", "existing JSONL dataset that may receive confirmed results")
	command.Flags().BoolVar(&noBrowser, "no-browser", false, "do not open the default browser")
	command.Flags().StringVar(&openAIBaseURL, "openai-base-url", "", "override the OpenAI API base URL")
	command.Flags().StringVar(&anthropicBaseURL, "anthropic-base-url", "", "override the Anthropic API base URL")
	return command
}

type arenaCatalogModel struct {
	provider string
	model    string
}

var defaultArenaModels = []arenaCatalogModel{
	{provider: "openai", model: "gpt-4.1"},
	{provider: "openai", model: "gpt-4.1-mini"},
	{provider: "openai", model: "gpt-4.1-nano"},
	{provider: "anthropic", model: "claude-sonnet-5"},
	{provider: "anthropic", model: "claude-sonnet-4-6"},
	{provider: "anthropic", model: "claude-haiku-4-5-20251001"},
}
