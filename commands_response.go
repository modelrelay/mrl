package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
	"github.com/spf13/cobra"
)

func newResponseCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "response",
		Short: "Inspect direct Responses configuration",
	}
	command.AddCommand(newResponseResolveCmd())
	return command
}

func newResponseResolveCmd() *cobra.Command {
	var model string
	var provider string
	command := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve a direct Responses route without executing it",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			model = strings.TrimSpace(model)
			provider = strings.TrimSpace(provider)
			if model == "" {
				return errors.New("--model is required")
			}
			config, err := runtimeConfigFrom(command)
			if err != nil {
				return err
			}
			ctx, cancel := contextWithTimeout(config.Timeout)
			defer cancel()
			resolved, err := requestResponseResolution(ctx, config, model, provider)
			if err != nil {
				return err
			}
			if config.Output == outputFormatJSON {
				printJSON(resolved)
				return nil
			}
			printResponseResolution(resolved)
			return nil
		},
	}
	command.Flags().StringVar(&model, "model", "", "Concrete model ID (required)")
	command.Flags().StringVar(&provider, "provider", "", "Optional concrete provider ID")
	return command
}

func requestResponseResolution(ctx context.Context, config runtimeConfig, model, provider string) (generated.ResponseResolveResponse, error) {
	auth, err := dataPlaneAuthMode(config)
	if err != nil {
		return generated.ResponseResolveResponse{}, err
	}
	request := generated.ResponseResolveRequest{Model: model}
	if provider != "" {
		request.Provider = &provider
	}
	var response generated.ResponseResolveResponse
	if err = doJSON(ctx, config, auth, http.MethodPost, routes.ResponsesResolve, request, &response); err != nil {
		return generated.ResponseResolveResponse{}, err
	}
	return response, nil
}

func dataPlaneAuthMode(config runtimeConfig) (authMode, error) {
	if strings.TrimSpace(config.APIKey) != "" {
		return authModeAPIKey, nil
	}
	if strings.TrimSpace(config.Token) != "" {
		return authModeBearer, nil
	}
	return authModeNone, errors.New("api key or account token required")
}

func printResponseResolution(response generated.ResponseResolveResponse) {
	pricing := response.Pricing
	pairs := []kvPair{
		{Key: "resolved_model", Value: response.ResolvedModel},
		{Key: "provider", Value: response.Provider},
		{Key: "pricing_availability", Value: string(pricing.Availability)},
		{Key: "pricing_content_hash", Value: pricing.ContentHash},
		{Key: "pricing_source", Value: stringOrEmpty(pricing.Source)},
		{Key: "pricing_model", Value: stringOrEmpty(pricing.Model)},
		{Key: "pricing_provider", Value: stringOrEmpty(pricing.Provider)},
	}
	if pricing.InputCostPerMillionCents != nil {
		pairs = append(pairs, kvPair{Key: "input_cost_per_million_cents", Value: fmt.Sprint(*pricing.InputCostPerMillionCents)})
	}
	if pricing.OutputCostPerMillionCents != nil {
		pairs = append(pairs, kvPair{Key: "output_cost_per_million_cents", Value: fmt.Sprint(*pricing.OutputCostPerMillionCents)})
	}
	if pricing.PlatformFeePercent != nil {
		pairs = append(pairs, kvPair{Key: "platform_fee_percent", Value: fmt.Sprint(*pricing.PlatformFeePercent)})
	}
	printKeyValueTable(pairs)
}
