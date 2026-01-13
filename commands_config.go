package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
	}
	cmd.AddCommand(newConfigShowCmd(), newConfigSetCmd(), newConfigUseCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show config profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCLIConfig()
			if err != nil {
				return err
			}
			profileName := resolveProfileName(profile, cfg)
			profileCfg := profileFor(cfg, profileName)
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				printJSON(map[string]any{
					"profile":         profileName,
					"config":          profileCfg,
					"current_profile": cfg.CurrentProfile,
				})
				return nil
			}
			pairs := []kvPair{
				{Key: "profile", Value: profileName},
				{Key: "current_profile", Value: cfg.CurrentProfile},
				{Key: "api_key", Value: profileCfg.APIKey},
				{Key: "base_url", Value: profileCfg.BaseURL},
				{Key: "project_id", Value: profileCfg.ProjectID},
				{Key: "output", Value: profileCfg.Output},
			}
			printKeyValueTable(pairs)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	var profile string
	var apiKey string
	var baseURL string
	var projectID string
	var output string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set config values for a profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCLIConfig()
			if err != nil {
				return err
			}
			profileName := resolveProfileName(profile, cfg)
			profileCfg := profileFor(cfg, profileName)

			if cmd.Flags().Changed("api-key") {
				profileCfg.APIKey = strings.TrimSpace(apiKey)
			}
			if cmd.Flags().Changed("base-url") {
				profileCfg.BaseURL = strings.TrimSpace(baseURL)
			}
			if cmd.Flags().Changed("project") {
				profileCfg.ProjectID = strings.TrimSpace(projectID)
			}
			if cmd.Flags().Changed("output") {
				clean := strings.ToLower(strings.TrimSpace(output))
				switch clean {
				case "", string(outputFormatJSON), string(outputFormatTable):
					profileCfg.Output = clean
				default:
					return errors.New("output must be json or table")
				}
			}

			if cfg.Profiles == nil {
				cfg.Profiles = map[string]cliProfile{}
			}
			cfg.Profiles[profileName] = profileCfg
			if cfg.CurrentProfile == "" {
				cfg.CurrentProfile = profileName
			}
			if err := writeCLIConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("updated profile %s\n", profileName)
			return nil
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL")
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID")
	cmd.Flags().StringVar(&output, "output", "", "Output format (json|table)")
	return cmd
}

func newConfigUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <profile>",
		Short: "Set the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadCLIConfig()
			if err != nil {
				return err
			}
			profileName := strings.TrimSpace(args[0])
			if profileName == "" {
				return errors.New("profile name required")
			}
			if cfg.Profiles == nil {
				cfg.Profiles = map[string]cliProfile{}
			}
			cfg.CurrentProfile = profileName
			if err := writeCLIConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("current profile set to %s\n", profileName)
			return nil
		},
	}
}
