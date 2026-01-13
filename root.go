package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

const defaultAPIBaseURL = "https://api.modelrelay.ai/api/v1"

func newRootCmd() *cobra.Command {
	var model string
	var system string
	var stream bool
	var usage bool

	root := &cobra.Command{
		Use:   "mrl [prompt]",
		Short: "ModelRelay CLI",
		Long: `ModelRelay CLI - chat with AI models.

Examples:
  mrl "What is 2 + 2?"
  mrl "Write a haiku" --stream
  mrl "Explain recursion" --model gpt-5.2 --usage
  mrl config set --model claude-sonnet-4-5`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runPrompt(cmd, args, model, system, stream, usage)
		},
	}

	// Prompt flags (on root command)
	root.Flags().StringVar(&model, "model", "", "Model ID (overrides profile default)")
	root.Flags().StringVar(&system, "system", "", "System prompt")
	root.Flags().BoolVar(&stream, "stream", false, "Stream output as it's generated")
	root.Flags().BoolVar(&usage, "usage", false, "Show token usage after response")

	// Global flags
	root.PersistentFlags().String("profile", "", "Config profile")
	root.PersistentFlags().String("base-url", "", "API base URL override")
	root.PersistentFlags().String("project", "", "Project UUID")
	root.PersistentFlags().String("api-key", "", "Secret API key (mr_sk_*)")
	root.PersistentFlags().Bool("json", false, "Output JSON")
	root.PersistentFlags().Duration("timeout", 30*time.Second, "Request timeout")

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cfgFile, err := loadCLIConfig()
		if err != nil {
			return err
		}
		runtime, err := resolveRuntimeConfig(cmd, cfgFile)
		if err != nil {
			return err
		}
		cmd.SetContext(withRuntimeConfig(cmd.Context(), runtime))
		return nil
	}

	root.AddCommand(
		newConfigCmd(),
		newCustomerCmd(),
		newUsageCmd(),
		newTierCmd(),
		newAgentCmd(),
		newModelCmd(),
		newSchemaCmd(),
		newVersionCmd(),
		newDoCmd(),
	)

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("mrl %s\n", version)
		},
	}
}
