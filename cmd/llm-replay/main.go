package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ardao/llm-replay/internal/cli"
	"github.com/ardao/llm-replay/internal/config"
)

var version = "dev"

func main() {
	cfg := config.Load()
	cmd := cli.NewRootCommand(cfg, version, os.Stdout, os.Stderr)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
