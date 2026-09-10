package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ardao/llm-replay/internal/capture"
	"github.com/spf13/cobra"
)

func newCaptureCommand() *cobra.Command {
	var listenAddress string
	var upstreamURL string
	var outputPath string

	command := &cobra.Command{
		Use:   "capture",
		Short: "Capture OpenAI-compatible traffic into a local JSONL dataset",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			proxy, err := capture.New(capture.Config{
				ListenAddr:  listenAddress,
				UpstreamURL: upstreamURL,
				OutputPath:  outputPath,
				OnCaptureError: func(err error) {
					fmt.Fprintf(cmd.ErrOrStderr(), "capture warning: %v\n", err)
				},
			})
			if err != nil {
				return err
			}
			defer proxy.Close()

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			server := &http.Server{
				Addr:              proxy.ListenAddr(),
				Handler:           proxy,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       90 * time.Second,
			}
			shutdownDone := make(chan struct{})
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := server.Shutdown(shutdownCtx); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "capture shutdown warning: %v\n", err)
				}
				close(shutdownDone)
			}()

			fmt.Fprintf(cmd.OutOrStdout(), "Capture proxy listening on http://%s\n", proxy.ListenAddr())
			fmt.Fprintf(cmd.OutOrStdout(), "Upstream: %s\nDataset: %s\n", upstreamURL, outputPath)
			err = server.ListenAndServe()
			stop()
			<-shutdownDone
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("run capture proxy: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&listenAddress, "listen", "127.0.0.1:8787", "loopback address to listen on")
	command.Flags().StringVar(&upstreamURL, "upstream", "https://api.openai.com", "OpenAI-compatible upstream base URL")
	command.Flags().StringVarP(&outputPath, "output", "o", "", "JSONL dataset output path")
	_ = command.MarkFlagRequired("output")
	return command
}
