package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go"
	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run and test agents",
	}
	cmd.AddCommand(newAgentRunCmd(), newAgentTestCmd(), newAgentLoopCmd())
	return cmd
}

type agentFlags struct {
	inputText         string
	inputFile         string
	mockToolsPath     string
	outputPath        string
	trace             bool
	model             string
	maxSteps          int
	maxDuration       int
	stepTimeout       int
	toolFailurePolicy string
	toolRetryCount    int
	captureToolIO     bool
	dryRun            bool
	customerID        string
}

func newAgentRunCmd() *cobra.Command {
	flags := &agentFlags{}
	cmd := &cobra.Command{
		Use:   "run <slug>",
		Short: "Run an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(cmd, args[0], flags, false)
		},
	}
	bindAgentFlags(cmd, flags, false)
	cmd.Flags().StringVar(&flags.customerID, "customer", "", "Customer ID")
	return cmd
}

func newAgentTestCmd() *cobra.Command {
	flags := &agentFlags{}
	cmd := &cobra.Command{
		Use:   "test <slug>",
		Short: "Test an agent with mocked tools",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(cmd, args[0], flags, true)
		},
	}
	bindAgentFlags(cmd, flags, true)
	return cmd
}

func bindAgentFlags(cmd *cobra.Command, flags *agentFlags, includeMock bool) {
	cmd.Flags().StringVar(&flags.inputText, "input", "", "Inline user input")
	cmd.Flags().StringVar(&flags.inputFile, "input-file", "", "Path to JSON array of input items")
	if includeMock {
		cmd.Flags().StringVar(&flags.mockToolsPath, "mock-tools", "", "Path to JSON object of mock tool outputs")
	}
	cmd.Flags().StringVar(&flags.outputPath, "output", "", "Write JSON response to file")
	cmd.Flags().BoolVar(&flags.trace, "trace", false, "Print step summaries")
	cmd.Flags().StringVar(&flags.model, "model", "", "Override model")
	cmd.Flags().IntVar(&flags.maxSteps, "max-steps", -1, "Override max tool steps")
	cmd.Flags().IntVar(&flags.maxDuration, "max-duration-sec", -1, "Override max run duration")
	cmd.Flags().IntVar(&flags.stepTimeout, "step-timeout-sec", -1, "Override per-step timeout")
	cmd.Flags().StringVar(&flags.toolFailurePolicy, "tool-failure-policy", "", "stop | continue | retry")
	cmd.Flags().IntVar(&flags.toolRetryCount, "tool-retry-count", -1, "Retry count when policy is retry")
	cmd.Flags().BoolVar(&flags.captureToolIO, "capture-tool-io", false, "Include step logs in response")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Error if a tool is not mocked")
}

func runAgent(cmd *cobra.Command, slug string, flags *agentFlags, useTestEndpoint bool) error {
	cfg, err := runtimeConfigFrom(cmd)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return errors.New("project id required")
	}
	projectID, err := uuid.Parse(cfg.ProjectID)
	if err != nil {
		return errors.New("invalid project id")
	}

	client, err := newAgentClient(cfg)
	if err != nil {
		return err
	}

	input, err := resolveInput(flags.inputText, flags.inputFile, cmd.Flags().Args()[1:])
	if err != nil {
		return err
	}

	options := buildAgentOptions(flags, cmd.Flags())

	ctx, cancel := contextWithTimeout(cfg.Timeout)
	defer cancel()

	if useTestEndpoint {
		mockTools, err := readMockTools(flags.mockToolsPath)
		if err != nil {
			return err
		}
		req := sdk.AgentTestRequest{
			Input:     input,
			MockTools: mockTools,
			Options:   options,
		}
		resp, err := client.Agents.Test(ctx, projectID, slug, req)
		if err != nil {
			return err
		}
		return handleAgentResponse(cfg, resp, flags)
	}

	req := sdk.AgentRunRequest{
		Input:      input,
		Options:    options,
		CustomerID: optionalString(flags.customerID),
	}
	resp, err := client.Agents.Run(ctx, projectID, slug, req)
	if err != nil {
		return err
	}
	return handleAgentResponse(cfg, resp, flags)
}

func newAgentClient(cfg runtimeConfig) (*sdk.Client, error) {
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

func buildAgentOptions(flags *agentFlags, flagset *pflag.FlagSet) *sdk.AgentRunOptions {
	opts := &sdk.AgentRunOptions{}
	if strings.TrimSpace(flags.model) != "" {
		clean := strings.TrimSpace(flags.model)
		opts.Model = &clean
	}
	if flags.maxSteps >= 0 {
		val := flags.maxSteps
		opts.MaxSteps = &val
	}
	if flags.maxDuration >= 0 {
		val := flags.maxDuration
		opts.MaxDurationSec = &val
	}
	if flags.stepTimeout >= 0 {
		val := flags.stepTimeout
		opts.StepTimeoutSec = &val
	}
	if strings.TrimSpace(flags.toolFailurePolicy) != "" {
		clean := sdk.AgentRunOptionsToolFailurePolicy(strings.TrimSpace(flags.toolFailurePolicy))
		opts.ToolFailurePolicy = &clean
	}
	if flags.toolRetryCount >= 0 {
		val := flags.toolRetryCount
		opts.ToolRetryCount = &val
	}
	if flagset != nil {
		if flagset.Changed("capture-tool-io") {
			val := flags.captureToolIO
			opts.CaptureToolIo = &val
		}
		if flagset.Changed("dry-run") {
			val := flags.dryRun
			opts.DryRun = &val
		}
	}

	if opts.Model == nil && opts.MaxSteps == nil && opts.MaxDurationSec == nil && opts.StepTimeoutSec == nil &&
		opts.ToolFailurePolicy == nil && opts.ToolRetryCount == nil && opts.CaptureToolIo == nil && opts.DryRun == nil {
		return nil
	}
	return opts
}

func resolveInput(inputText, inputFile string, tail []string) ([]llm.InputItem, error) {
	if inputFile != "" {
		return readInputFile(inputFile)
	}
	text := strings.TrimSpace(inputText)
	if text == "" && len(tail) > 0 {
		text = strings.TrimSpace(strings.Join(tail, " "))
	}
	if text == "" {
		return nil, errors.New("input is required (use --input, --input-file, or trailing args)")
	}
	return []llm.InputItem{llm.NewUserText(text)}, nil
}

func readInputFile(path string) ([]llm.InputItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("input file is empty")
	}

	var items []llm.InputItem
	if err := json.Unmarshal(raw, &items); err == nil && len(items) > 0 {
		return items, nil
	}

	var wrapper struct {
		Input []llm.InputItem `json:"input"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Input) > 0 {
		return wrapper.Input, nil
	}

	return nil, errors.New("input file must be a JSON array of input items or an object with {input: [...]} ")
}

func readMockTools(path string) (map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("mock-tools must be a JSON object: %w", err)
	}
	return payload, nil
}

func handleAgentResponse(cfg runtimeConfig, resp sdk.AgentRunResponse, flags *agentFlags) error {
	jsonPayload, _ := json.MarshalIndent(resp, "", "  ")
	if flags.outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(flags.outputPath), 0o755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		if err := os.WriteFile(flags.outputPath, jsonPayload, 0o600); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}

	if cfg.Output == outputFormatJSON {
		fmt.Println(string(jsonPayload))
		return nil
	}

	fmt.Printf("Run ID: %s\n", resp.RunId)
	fmt.Printf("Steps: %d | LLM calls: %d | Tool calls: %d | Cost: %0.2f\n",
		resp.Usage.TotalSteps,
		resp.Usage.TotalLlmCalls,
		resp.Usage.TotalToolCalls,
		float64(resp.Usage.TotalCostCents)/100.0,
	)

	if outputText := renderOutputText(resp.Output); outputText != "" {
		fmt.Println("\nOutput:\n" + outputText)
	}

	if flags.trace && resp.Steps != nil {
		fmt.Println("\nTrace:")
		for _, step := range *resp.Steps {
			toolCalls := 0
			toolResults := 0
			if step.ToolCalls != nil {
				toolCalls = len(*step.ToolCalls)
			}
			if step.ToolResults != nil {
				toolResults = len(*step.ToolResults)
			}
			fmt.Printf("- node=%s step=%d tool_calls=%d tool_results=%d\n", step.NodeId, step.Step, toolCalls, toolResults)
		}
	}

	return nil
}

func renderOutputText(output *[]generated.OutputItem) string {
	if output == nil {
		return ""
	}
	var builder strings.Builder
	for _, item := range *output {
		if item.Content == nil {
			continue
		}
		for _, part := range *item.Content {
			textPart, err := part.AsContentPartText()
			if err != nil {
				continue
			}
			builder.WriteString(textPart.Text)
		}
	}
	return strings.TrimSpace(builder.String())
}

func optionalString(val string) *string {
	if strings.TrimSpace(val) == "" {
		return nil
	}
	clean := strings.TrimSpace(val)
	return &clean
}
