package cli

import (
	"io"

	"github.com/ardao/llm-replay/internal/config"
	"github.com/spf13/cobra"
)

// NewRootCommand constructs the root CLI command without global state.
func NewRootCommand(cfg *config.Config, version string, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "llm-replay",
		Short:         "Replay real LLM workloads before shipping changes",
		Long:          "LLM Replay captures and replays real LLM workloads so models and configurations can be compared before production changes are shipped.",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.PersistentFlags().BoolVarP(&cfg.Verbose, "verbose", "v", cfg.Verbose, "enable verbose logging")
	cmd.AddCommand(
		newCaptureCommand(),
		newInspectCommand(),
		newReplayCommand(cfg),
		newCompareCommand(),
		newReportCommand(),
		newUICommand(),
		newArenaCommand(cfg),
	)

	return cmd
}
