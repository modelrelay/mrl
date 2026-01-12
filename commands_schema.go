package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	schema "github.com/modelrelay/modelrelay/providers/schema"
	"github.com/spf13/cobra"
)

type schemaLintConfig struct {
	provider   string
	toolSchema bool
}

func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Schema utilities",
	}
	cmd.AddCommand(newSchemaLintCmd())
	return cmd
}

func newSchemaLintCmd() *cobra.Command {
	cfg := schemaLintConfig{}
	cmd := &cobra.Command{
		Use:   "lint <path|->",
		Short: "Validate a JSON schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := strings.TrimSpace(args[0])
			if path == "" {
				return errors.New("schema path is required")
			}

			raw, err := readSchemaInput(path)
			if err != nil {
				return err
			}

			normalized, err := schema.NormalizeJSON(raw)
			if err != nil {
				return err
			}

			if strings.TrimSpace(cfg.provider) != "" {
				if err := validateSchemaForProvider(normalized, cfg); err != nil {
					return err
				}
			}

			fmt.Println("OK")
			return nil
		},
	}

	cmd.Flags().StringVar(&cfg.provider, "provider", "", "openai | anthropic | googleai | xai (optional)")
	cmd.Flags().BoolVar(&cfg.toolSchema, "tool-schema", false, "Validate as tool parameters (OpenAI only)")
	return cmd
}

func readSchemaInput(path string) (json.RawMessage, error) {
	if path == "-" {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, errors.New("schema input is empty")
		}
		return json.RawMessage(raw), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("schema file is empty")
	}
	return json.RawMessage(raw), nil
}

func validateSchemaForProvider(normalized *schema.Schema, cfg schemaLintConfig) error {
	switch strings.ToLower(strings.TrimSpace(cfg.provider)) {
	case "openai":
		if cfg.toolSchema {
			_, err := schema.AdaptOpenAIToolSchema(normalized)
			return err
		}
		_, err := schema.AdaptOpenAI(normalized)
		return err
	case "anthropic":
		if cfg.toolSchema {
			return errors.New("tool-schema flag is not supported for anthropic")
		}
		_, err := schema.AdaptAnthropic(normalized)
		return err
	case "googleai":
		if cfg.toolSchema {
			return errors.New("tool-schema flag is not supported for googleai")
		}
		_, err := schema.AdaptGoogleAI(normalized)
		return err
	case "xai":
		if cfg.toolSchema {
			return errors.New("tool-schema flag is not supported for xai")
		}
		_, err := schema.AdaptXAI(normalized)
		return err
	default:
		return fmt.Errorf("unknown provider: %s", cfg.provider)
	}
}
