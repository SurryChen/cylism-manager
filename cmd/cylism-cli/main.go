package main

import (
	"context"
	"os"

	"github.com/cylism/cylism-manager/internal/agentcli"
)

func main() {
	code := agentcli.Run(context.Background(), os.Args[1:], agentcli.Config{
		BaseURL:   os.Getenv("CYLISM_AGENT_API_URL"),
		TokenFile: os.Getenv("CYLISM_AGENT_TOKEN_FILE"),
	}, os.Stdout)
	os.Exit(code)
}
