package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelrelay/modelrelay/sdk/go"
	"github.com/spf13/cobra"
)

// runPrompt is the default action when mrl is invoked with a prompt.
func runPrompt(cmd *cobra.Command, args []string, modelFlag, system string, stream, showUsage bool) error {
	cfg, err := runtimeConfigFrom(cmd)
	if err != nil {
		return err
	}

	model := resolveModel(modelFlag, cfg)
	if model == "" {
		return errors.New("model is required (set via --model, MODELRELAY_MODEL, or mrl config set --model)")
	}

	client, err := newPromptClient(cfg)
	if err != nil {
		return err
	}

	prompt := strings.Join(args, " ")

	ctx, cancel := contextWithTimeout(cfg.Timeout)
	defer cancel()

	if stream {
		return runStreamWithUsage(ctx, client, model, system, prompt, showUsage)
	}

	opts := &sdk.ChatOptions{System: system}
	start := time.Now()
	resp, err := client.Chat(ctx, model, prompt, opts)
	if err != nil {
		return err
	}
	latency := time.Since(start)

	fmt.Println(resp.AssistantText())
	if showUsage {
		fmt.Printf("\nModel: %s | Tokens: %d in / %d out | Latency: %s\n",
			resp.Model,
			resp.Usage.InputTokens,
			resp.Usage.OutputTokens,
			latency.Round(time.Millisecond),
		)
	}
	return nil
}

func runStreamWithUsage(ctx context.Context, client *sdk.Client, model, system, prompt string, showUsage bool) error {
	builder := client.Responses.New().Model(sdk.NewModelID(model)).User(prompt)
	if system != "" {
		builder = builder.System(system)
	}

	req, opts, err := builder.Build()
	if err != nil {
		return err
	}

	start := time.Now()
	stream, err := client.Responses.Stream(ctx, req, opts...)
	if err != nil {
		return err
	}
	defer stream.Close()

	var finalModel sdk.ModelID
	var finalUsage *sdk.Usage
	var ttft time.Duration
	var sawFirstToken bool

	for {
		ev, ok, err := stream.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		if ev.TextDelta != "" {
			if !sawFirstToken {
				ttft = time.Since(start)
				sawFirstToken = true
			}
			fmt.Print(ev.TextDelta)
		}
		if !ev.Model.IsEmpty() {
			finalModel = ev.Model
		}
		if ev.Usage != nil {
			finalUsage = ev.Usage
		}
	}
	totalDuration := time.Since(start)
	fmt.Println()

	if showUsage && finalUsage != nil {
		fmt.Printf("\nModel: %s | Tokens: %d in / %d out | TTFT: %s | Total: %s\n",
			finalModel,
			finalUsage.InputTokens,
			finalUsage.OutputTokens,
			ttft.Round(time.Millisecond),
			totalDuration.Round(time.Millisecond),
		)
	}
	return nil
}

func newPromptClient(cfg runtimeConfig) (*sdk.Client, error) {
	opts := []sdk.Option{sdk.WithClientHeader(clientHeader())}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		opts = append(opts, sdk.WithBaseURL(cfg.BaseURL))
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("api key required")
	}
	key, err := sdk.ParseAPIKeyAuth(cfg.APIKey)
	if err != nil {
		return nil, err
	}
	return sdk.NewClientWithKey(key, opts...)
}

func resolveModel(flagValue string, cfg runtimeConfig) string {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue)
	}
	return cfg.Model
}
