package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

const defaultAPIBaseURL = "https://api.modelrelay.ai/api/v1"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "mrl",
		Short:         "ModelRelay CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

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
