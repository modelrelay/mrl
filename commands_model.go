package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/spf13/cobra"
)

type modelsResponse struct {
	Models []generated.Model `json:"models"`
}

func newModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "List models",
	}
	cmd.AddCommand(newModelListCmd())
	return cmd
}

func newModelListCmd() *cobra.Command {
	var provider string
	var capability string
	var includeDeprecated bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available models",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			path := "/models"
			query := []string{}
			if strings.TrimSpace(provider) != "" {
				query = append(query, "provider="+strings.TrimSpace(provider))
			}
			if strings.TrimSpace(capability) != "" {
				query = append(query, "capability="+strings.TrimSpace(capability))
			}
			if len(query) > 0 {
				path = path + "?" + strings.Join(query, "&")
			}

			var resp modelsResponse
			if err := doJSON(ctx, cfg, authModeNone, http.MethodGet, path, nil, &resp); err != nil {
				return err
			}

			models := resp.Models
			if !includeDeprecated {
				filtered := make([]generated.Model, 0, len(models))
				for _, model := range models {
					if model.Deprecated {
						continue
					}
					filtered = append(filtered, model)
				}
				models = filtered
			}

			if cfg.Output == outputFormatJSON {
				printJSON(modelsResponse{Models: models})
				return nil
			}
			printModelsTable(models)
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider")
	cmd.Flags().StringVar(&capability, "capability", "text_generation", "Filter by capability")
	cmd.Flags().BoolVar(&includeDeprecated, "include-deprecated", false, "Include deprecated models")
	return cmd
}

func printModelsTable(models []generated.Model) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tMODEL\tDISPLAY_NAME\tCTX\tMAX_OUT\tDEPRECATED")
	for _, model := range models {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%d\t%d\t%t\n",
			model.Provider,
			model.ModelId,
			model.DisplayName,
			model.ContextWindow,
			model.MaxOutputTokens,
			model.Deprecated,
		)
	}
	_ = w.Flush()
}
