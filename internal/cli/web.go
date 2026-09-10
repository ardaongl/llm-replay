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

	"github.com/ardao/llm-replay/internal/report"
	"github.com/ardao/llm-replay/internal/ui"
	"github.com/spf13/cobra"
)

func newReportCommand() *cobra.Command {
	var outputPath string
	var maxResults int
	var failuresOnly bool
	command := &cobra.Command{
		Use:   "report <run-directory> [run-directory...]",
		Short: "Export replay runs as a self-contained HTML report",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := report.GenerateHTML(report.HTMLOptions{
				RunDirs: args, OutputPath: outputPath, MaxResults: maxResults, FailuresOnly: failuresOnly,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "HTML report: %s\nRecords: %d/%d\n", result.OutputPath, result.Included, result.Total)
			if result.Truncated {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: report was truncated; increase --max-results or use the local UI")
			}
			return nil
		},
	}
	command.Flags().StringVarP(&outputPath, "output", "o", "", "self-contained .html output path")
	command.Flags().IntVar(&maxResults, "max-results", 5000, "maximum records embedded in the HTML report")
	command.Flags().BoolVar(&failuresOnly, "failures-only", false, "include only records with request or evaluation failures")
	_ = command.MarkFlagRequired("output")
	return command
}

func newUICommand() *cobra.Command {
	var host string
	var port int
	var noBrowser bool
	command := &cobra.Command{
		Use:   "ui <run-directory> [run-directory...]",
		Short: "Explore replay runs in a secure local web UI",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if err := ui.ValidateHost(host); err != nil {
				return err
			}
			if port < 0 || port > 65535 {
				return errors.New("--port must be between 0 and 65535")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			handler, err := ui.NewServer(args)
			if err != nil {
				return err
			}
			listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err != nil {
				return fmt.Errorf("start local UI: %w", err)
			}
			defer listener.Close()

			server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			shutdownDone := make(chan struct{})
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "UI shutdown warning: %v\n", shutdownErr)
				}
				close(shutdownDone)
			}()

			address := listener.Addr().String()
			targetURL := "http://" + address + "/"
			fmt.Fprintf(cmd.OutOrStdout(), "LLM Replay UI: %s\n", targetURL)
			if !noBrowser {
				if browserErr := ui.OpenBrowser(targetURL); browserErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "browser warning: %v\n", browserErr)
				}
			}
			err = server.Serve(listener)
			stop()
			<-shutdownDone
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("run local UI: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&host, "host", "127.0.0.1", "loopback host to bind")
	command.Flags().IntVar(&port, "port", 0, "local port; 0 selects a free port")
	command.Flags().BoolVar(&noBrowser, "no-browser", false, "do not open the default browser")
	return command
}
