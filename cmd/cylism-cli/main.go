package main

import (
	"context"
	"os"

	runtimecli "github.com/cylism/cylism-manager/internal/runtime/cli"
)

func main() {
	code := runtimecli.Run(context.Background(), os.Args[1:], runtimecli.Config{
		BaseURL:   os.Getenv("CYLISM_AGENT_API_URL"),
		TokenFile: os.Getenv("CYLISM_AGENT_TOKEN_FILE"),
	}, os.Stdout)
	os.Exit(code)
}
