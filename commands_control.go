package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/spf13/cobra"
)

type projectListResponse struct {
	Projects []generated.Project `json:"projects"`
}

type projectResponse struct {
	Project generated.Project `json:"project"`
}

type apiKeyPayload struct {
	ID          uuid.UUID  `json:"id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	Label       string     `json:"label"`
	Kind        string     `json:"kind"`
	CreatedAt   time.Time  `json:"created_at"`
	RedactedKey string     `json:"redacted_key"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	SecretKey   string     `json:"secret_key,omitempty"`
}

type apiKeyListResponse struct {
	APIKeys []apiKeyPayload `json:"api_keys"`
}

type apiKeyResponse struct {
	APIKey apiKeyPayload `json:"api_key"`
}

type customerListResponse struct {
	Customers []generated.CustomerWithSubscription `json:"customers"`
}

type customerUsageSummary struct {
	TotalRequests       int64  `json:"total_requests"`
	TotalInputTokens    int64  `json:"total_input_tokens"`
	TotalOutputTokens   int64  `json:"total_output_tokens"`
	TotalImages         int64  `json:"total_images"`
	TotalCostCents      int64  `json:"total_cost_cents"`
	WalletBalanceCents  *int64 `json:"wallet_balance_cents,omitempty"`
	WalletReservedCents *int64 `json:"wallet_reserved_cents,omitempty"`
	OverageEnabled      *bool  `json:"overage_enabled,omitempty"`
}

type customerUsagePoint struct {
	Day              time.Time `json:"day"`
	Requests         int64     `json:"requests"`
	Tokens           int64     `json:"tokens"`
	Images           int64     `json:"images"`
	CreditsUsedCents int64     `json:"credits_used_cents"`
}

type tierLimitInfo struct {
	SpendLimitCents        int64     `json:"spend_limit_cents"`
	CurrentPeriodCostCents int64     `json:"current_period_cost_cents"`
	PercentageUsed         float64   `json:"percentage_used"`
	IsNearLimit            bool      `json:"is_near_limit"`
	WindowStart            time.Time `json:"window_start"`
	WindowEnd              time.Time `json:"window_end"`
}

type customerUsageResponse struct {
	Summary    customerUsageSummary `json:"summary"`
	DailyUsage []customerUsagePoint `json:"daily_usage"`
	TierLimit  *tierLimitInfo       `json:"tier_limit,omitempty"`
}

type usageSummary struct {
	PlanType    string    `json:"plan_type,omitempty"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Limit       int64     `json:"limit"`
	Used        int64     `json:"used"`
	Remaining   int64     `json:"remaining"`
	State       string    `json:"state"`
	Images      int64     `json:"images"`
}

type usageSummaryResponse struct {
	Summary usageSummary `json:"summary"`
}

type tierListResponse struct {
	Tiers []generated.Tier `json:"tiers"`
}

type tierResponse struct {
	Tier generated.Tier `json:"tier"`
}

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}
	cmd.AddCommand(newProjectListCmd(), newProjectGetCmd(), newProjectCreateCmd())
	return cmd
}

func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" {
				return errors.New("access token required")
			}
			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp projectListResponse
			if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, "/projects", nil, &resp); err != nil {
				return err
			}
			return outputProjects(cfg, resp.Projects)
		},
	}
}

func newProjectGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <project-id>",
		Short: "Get a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" {
				return errors.New("access token required")
			}
			projectID := strings.TrimSpace(args[0])
			if _, err := uuid.Parse(projectID); err != nil {
				return errors.New("invalid project id")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp projectResponse
			path := fmt.Sprintf("/projects/%s", projectID)
			if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, path, nil, &resp); err != nil {
				return err
			}
			return outputProject(cfg, resp.Project)
		},
	}
}

func newProjectCreateCmd() *cobra.Command {
	var name string
	var description string
	var markup float64

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" {
				return errors.New("access token required")
			}
			cleanName := strings.TrimSpace(name)
			if cleanName == "" {
				return errors.New("project name is required")
			}

			payload := map[string]any{"name": cleanName}
			if strings.TrimSpace(description) != "" {
				payload["description"] = strings.TrimSpace(description)
			}
			if markup >= 0 {
				payload["markup_percentage"] = markup
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp projectResponse
			if err := doJSON(ctx, cfg, authModeToken, http.MethodPost, "/projects", payload, &resp); err != nil {
				return err
			}
			return outputProject(cfg, resp.Project)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Project name")
	cmd.Flags().StringVar(&description, "description", "", "Project description")
	cmd.Flags().Float64Var(&markup, "markup-percentage", -1, "Markup percentage (0-100)")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage API keys",
	}
	cmd.AddCommand(newKeyListCmd(), newKeyCreateCmd(), newKeyRevokeCmd())
	return cmd
}

func newKeyListCmd() *cobra.Command {
	var listAll bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" {
				return errors.New("access token required")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp apiKeyListResponse
			if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, "/api-keys", nil, &resp); err != nil {
				return err
			}

			keys := resp.APIKeys
			if !listAll && strings.TrimSpace(cfg.ProjectID) != "" {
				projectID, err := uuid.Parse(cfg.ProjectID)
				if err != nil {
					return errors.New("invalid project id")
				}
				filtered := make([]apiKeyPayload, 0, len(keys))
				for _, key := range keys {
					if key.ProjectID == projectID {
						filtered = append(filtered, key)
					}
				}
				keys = filtered
			}

			if cfg.Output == outputFormatJSON {
				printJSON(apiKeyListResponse{APIKeys: keys})
				return nil
			}
			printKeysTable(keys)
			return nil
		},
	}
	cmd.Flags().BoolVar(&listAll, "all", false, "List all keys (ignore project filter)")
	return cmd
}

func newKeyCreateCmd() *cobra.Command {
	var name string
	var kind string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" {
				return errors.New("access token required")
			}
			if strings.TrimSpace(cfg.ProjectID) == "" {
				return errors.New("project id required")
			}
			if _, err := uuid.Parse(cfg.ProjectID); err != nil {
				return errors.New("invalid project id")
			}

			cleanName := strings.TrimSpace(name)
			if cleanName == "" {
				return errors.New("key name is required")
			}

			payload := map[string]any{
				"label":      cleanName,
				"project_id": cfg.ProjectID,
				"kind":       strings.TrimSpace(kind),
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp apiKeyResponse
			if err := doJSON(ctx, cfg, authModeToken, http.MethodPost, "/api-keys", payload, &resp); err != nil {
				return err
			}

			if cfg.Output == outputFormatJSON {
				printJSON(resp)
				return nil
			}
			printKeyDetails(resp.APIKey)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Key label")
	cmd.Flags().StringVar(&kind, "kind", "secret", "Key kind (secret, publishable)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newKeyRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <key-id>",
		Short: "Revoke an API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" {
				return errors.New("access token required")
			}
			keyID := strings.TrimSpace(args[0])
			if _, err := uuid.Parse(keyID); err != nil {
				return errors.New("invalid api key id")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			path := fmt.Sprintf("/api-keys/%s", keyID)
			if err := doJSON(ctx, cfg, authModeToken, http.MethodDelete, path, nil, nil); err != nil {
				return err
			}

			if cfg.Output == outputFormatJSON {
				printJSON(map[string]string{"revoked": keyID})
				return nil
			}
			fmt.Printf("revoked %s\n", keyID)
			return nil
		},
	}
}

func newCustomerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "customer",
		Short: "Manage customers",
	}
	cmd.AddCommand(newCustomerListCmd(), newCustomerGetCmd(), newCustomerCreateCmd())
	return cmd
}

func newCustomerListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List customers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp customerListResponse
			if strings.TrimSpace(cfg.Token) != "" {
				if strings.TrimSpace(cfg.ProjectID) == "" {
					return errors.New("project id required when using access token")
				}
				if _, err := uuid.Parse(cfg.ProjectID); err != nil {
					return errors.New("invalid project id")
				}
				path := fmt.Sprintf("/projects/%s/customers", cfg.ProjectID)
				if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, path, nil, &resp); err != nil {
					return err
				}
			} else if strings.TrimSpace(cfg.APIKey) != "" {
				if err := doJSON(ctx, cfg, authModeAPIKey, http.MethodGet, "/customers", nil, &resp); err != nil {
					return err
				}
			} else {
				return errors.New("auth required")
			}

			if cfg.Output == outputFormatJSON {
				printJSON(resp)
				return nil
			}
			printCustomersTable(resp.Customers)
			return nil
		},
	}
}

func newCustomerGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <customer-id>",
		Short: "Get a customer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			customerID := strings.TrimSpace(args[0])
			if _, err := uuid.Parse(customerID); err != nil {
				return errors.New("invalid customer id")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var customer generated.CustomerWithSubscription
			if strings.TrimSpace(cfg.APIKey) != "" {
				path := fmt.Sprintf("/customers/%s", customerID)
				body, err := doJSONRaw(ctx, cfg, authModeAPIKey, http.MethodGet, path, nil)
				if err != nil {
					return err
				}
				customer, err = decodeCustomer(body)
				if err != nil {
					return err
				}
			} else if strings.TrimSpace(cfg.Token) != "" {
				if strings.TrimSpace(cfg.ProjectID) == "" {
					return errors.New("project id required when using access token")
				}
				if _, err := uuid.Parse(cfg.ProjectID); err != nil {
					return errors.New("invalid project id")
				}
				path := fmt.Sprintf("/projects/%s/customers", cfg.ProjectID)
				var resp customerListResponse
				if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, path, nil, &resp); err != nil {
					return err
				}
				found := false
				for _, c := range resp.Customers {
					if c.Customer.Id != nil && formatUUIDPtr(c.Customer.Id) == customerID {
						customer = c
						found = true
						break
					}
				}
				if !found {
					return errors.New("customer not found")
				}
			} else {
				return errors.New("auth required")
			}

			if cfg.Output == outputFormatJSON {
				printJSON(customer)
				return nil
			}
			printCustomerDetails(customer)
			return nil
		},
	}
}

func newCustomerCreateCmd() *cobra.Command {
	var externalID string
	var email string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a customer",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}

			cleanExternalID := strings.TrimSpace(externalID)
			cleanEmail := strings.TrimSpace(email)
			if cleanExternalID == "" || cleanEmail == "" {
				return errors.New("external-id and email are required")
			}

			payload := map[string]any{
				"external_id": cleanExternalID,
				"email":       cleanEmail,
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var customer generated.CustomerWithSubscription
			if strings.TrimSpace(cfg.Token) != "" {
				if strings.TrimSpace(cfg.ProjectID) == "" {
					return errors.New("project id required when using access token")
				}
				if _, err := uuid.Parse(cfg.ProjectID); err != nil {
					return errors.New("invalid project id")
				}
				path := fmt.Sprintf("/projects/%s/customers", cfg.ProjectID)
				body, err := doJSONRaw(ctx, cfg, authModeToken, http.MethodPost, path, payload)
				if err != nil {
					return err
				}
				customer, err = decodeCustomer(body)
				if err != nil {
					return err
				}
			} else if strings.TrimSpace(cfg.APIKey) != "" {
				body, err := doJSONRaw(ctx, cfg, authModeAPIKey, http.MethodPost, "/customers", payload)
				if err != nil {
					return err
				}
				customer, err = decodeCustomer(body)
				if err != nil {
					return err
				}
			} else {
				return errors.New("auth required")
			}

			if cfg.Output == outputFormatJSON {
				printJSON(customer)
				return nil
			}
			printCustomerDetails(customer)
			return nil
		},
	}
	cmd.Flags().StringVar(&externalID, "external-id", "", "External customer identifier")
	cmd.Flags().StringVar(&email, "email", "", "Customer email")
	_ = cmd.MarkFlagRequired("external-id")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func newUsageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Usage reporting",
	}
	cmd.AddCommand(newUsageAccountCmd(), newUsageCustomerCmd())
	return cmd
}

func newUsageAccountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "account",
		Short: "Show account usage summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.APIKey) == "" {
				return errors.New("auth required")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp usageSummaryResponse
			if err := doJSON(ctx, cfg, authModeTokenOrAPIKey, http.MethodGet, "/llm/usage", nil, &resp); err != nil {
				return err
			}

			if cfg.Output == outputFormatJSON {
				printJSON(resp)
				return nil
			}
			printUsageSummaryDetails(resp.Summary)
			return nil
		},
	}
}

func newUsageCustomerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "customer <customer-id>",
		Short: "Show customer usage within a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Token) == "" {
				return errors.New("access token required")
			}
			if strings.TrimSpace(cfg.ProjectID) == "" {
				return errors.New("project id required")
			}
			if _, err := uuid.Parse(cfg.ProjectID); err != nil {
				return errors.New("invalid project id")
			}
			customerID := strings.TrimSpace(args[0])
			if _, err := uuid.Parse(customerID); err != nil {
				return errors.New("invalid customer id")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			path := fmt.Sprintf("/projects/%s/customers/%s/usage", cfg.ProjectID, customerID)
			var resp customerUsageResponse
			if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, path, nil, &resp); err != nil {
				return err
			}

			if cfg.Output == outputFormatJSON {
				printJSON(resp)
				return nil
			}
			printCustomerUsageDetails(resp)
			return nil
		},
	}
}

func newTierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tier",
		Short: "Manage tiers",
	}
	cmd.AddCommand(newTierListCmd(), newTierGetCmd())
	return cmd
}

func newTierListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tiers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp tierListResponse
			if strings.TrimSpace(cfg.Token) != "" {
				if strings.TrimSpace(cfg.ProjectID) == "" {
					return errors.New("project id required when using access token")
				}
				if _, err := uuid.Parse(cfg.ProjectID); err != nil {
					return errors.New("invalid project id")
				}
				path := fmt.Sprintf("/projects/%s/tiers", cfg.ProjectID)
				if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, path, nil, &resp); err != nil {
					return err
				}
			} else if strings.TrimSpace(cfg.APIKey) != "" {
				if err := doJSON(ctx, cfg, authModeAPIKey, http.MethodGet, "/tiers", nil, &resp); err != nil {
					return err
				}
			} else {
				return errors.New("auth required")
			}

			if cfg.Output == outputFormatJSON {
				printJSON(resp)
				return nil
			}
			printTiersTable(resp.Tiers)
			return nil
		},
	}
}

func newTierGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <tier-id>",
		Short: "Get a tier",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigFrom(cmd)
			if err != nil {
				return err
			}
			tierID := strings.TrimSpace(args[0])
			if _, err := uuid.Parse(tierID); err != nil {
				return errors.New("invalid tier id")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp tierResponse
			if strings.TrimSpace(cfg.Token) != "" {
				if strings.TrimSpace(cfg.ProjectID) == "" {
					return errors.New("project id required when using access token")
				}
				if _, err := uuid.Parse(cfg.ProjectID); err != nil {
					return errors.New("invalid project id")
				}
				path := fmt.Sprintf("/projects/%s/tiers/%s", cfg.ProjectID, tierID)
				if err := doJSON(ctx, cfg, authModeToken, http.MethodGet, path, nil, &resp); err != nil {
					return err
				}
			} else if strings.TrimSpace(cfg.APIKey) != "" {
				path := fmt.Sprintf("/tiers/%s", tierID)
				if err := doJSON(ctx, cfg, authModeAPIKey, http.MethodGet, path, nil, &resp); err != nil {
					return err
				}
			} else {
				return errors.New("auth required")
			}

			if cfg.Output == outputFormatJSON {
				printJSON(resp)
				return nil
			}
			printTierDetails(resp.Tier)
			return nil
		},
	}
}

func decodeCustomer(body []byte) (generated.CustomerWithSubscription, error) {
	var direct generated.CustomerWithSubscription
	if err := json.Unmarshal(body, &direct); err == nil {
		if customerHasData(direct.Customer) {
			return direct, nil
		}
	}
	var envelope struct {
		Customer generated.CustomerWithSubscription `json:"customer"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return generated.CustomerWithSubscription{}, err
	}
	if !customerHasData(envelope.Customer.Customer) {
		return generated.CustomerWithSubscription{}, errors.New("customer response missing data")
	}
	return envelope.Customer, nil
}

func customerHasData(customer generated.Customer) bool {
	return customer.Id != nil || customer.ExternalId != nil || customer.Email != nil
}

func outputProjects(cfg runtimeConfig, projects []generated.Project) error {
	if cfg.Output == outputFormatJSON {
		printJSON(projectListResponse{Projects: projects})
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tBILLING_MODE\tMARKUP\tCREATED_AT")
	for _, project := range projects {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			formatUUIDPtr(project.Id),
			stringOrEmpty(project.Name),
			stringOrEmpty(project.BillingMode),
			formatFloat32(project.MarkupPercentage),
			formatTime(project.CreatedAt),
		)
	}
	_ = w.Flush()
	return nil
}

func outputProject(cfg runtimeConfig, project generated.Project) error {
	if cfg.Output == outputFormatJSON {
		printJSON(projectResponse{Project: project})
		return nil
	}
	pairs := []kvPair{
		{Key: "id", Value: formatUUIDPtr(project.Id)},
		{Key: "name", Value: stringOrEmpty(project.Name)},
		{Key: "billing_mode", Value: stringOrEmpty(project.BillingMode)},
		{Key: "markup_percentage", Value: formatFloat32(project.MarkupPercentage)},
		{Key: "created_at", Value: formatTime(project.CreatedAt)},
		{Key: "updated_at", Value: formatTime(project.UpdatedAt)},
	}
	printKeyValueTable(pairs)
	return nil
}

func printKeysTable(keys []apiKeyPayload) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tLABEL\tKIND\tPROJECT_ID\tCREATED_AT\tLAST_USED")
	for _, key := range keys {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			key.ID.String(),
			key.Label,
			key.Kind,
			key.ProjectID.String(),
			formatTime(&key.CreatedAt),
			formatTime(key.LastUsedAt),
		)
	}
	_ = w.Flush()
}

func printKeyDetails(key apiKeyPayload) {
	pairs := []kvPair{
		{Key: "id", Value: key.ID.String()},
		{Key: "label", Value: key.Label},
		{Key: "kind", Value: key.Kind},
		{Key: "project_id", Value: key.ProjectID.String()},
		{Key: "created_at", Value: formatTime(&key.CreatedAt)},
		{Key: "last_used_at", Value: formatTime(key.LastUsedAt)},
		{Key: "redacted_key", Value: key.RedactedKey},
		{Key: "secret_key", Value: key.SecretKey},
	}
	printKeyValueTable(pairs)
}

func printCustomersTable(customers []generated.CustomerWithSubscription) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEXTERNAL_ID\tEMAIL\tTIER\tSTATUS\tCREATED_AT")
	for _, item := range customers {
		tierCode := ""
		status := ""
		if item.Subscription != nil {
			if item.Subscription.TierCode != nil {
				tierCode = string(*item.Subscription.TierCode)
			}
			if item.Subscription.SubscriptionStatus != nil {
				status = string(*item.Subscription.SubscriptionStatus)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			formatUUIDPtr(item.Customer.Id),
			stringOrEmpty(item.Customer.ExternalId),
			stringOrEmpty(item.Customer.Email),
			tierCode,
			status,
			formatTime(item.Customer.CreatedAt),
		)
	}
	_ = w.Flush()
}

func printCustomerDetails(customer generated.CustomerWithSubscription) {
	pairs := []kvPair{
		{Key: "id", Value: formatUUIDPtr(customer.Customer.Id)},
		{Key: "external_id", Value: stringOrEmpty(customer.Customer.ExternalId)},
		{Key: "email", Value: stringOrEmpty(customer.Customer.Email)},
		{Key: "project_id", Value: formatUUIDPtr(customer.Customer.ProjectId)},
		{Key: "created_at", Value: formatTime(customer.Customer.CreatedAt)},
		{Key: "updated_at", Value: formatTime(customer.Customer.UpdatedAt)},
	}
	if customer.Subscription != nil {
		if customer.Subscription.TierCode != nil {
			pairs = append(pairs, kvPair{Key: "tier_code", Value: string(*customer.Subscription.TierCode)})
		}
		if customer.Subscription.SubscriptionStatus != nil {
			pairs = append(pairs, kvPair{Key: "subscription_status", Value: string(*customer.Subscription.SubscriptionStatus)})
		}
	}
	printKeyValueTable(pairs)
}

func printCustomerUsageDetails(resp customerUsageResponse) {
	pairs := []kvPair{
		{Key: "total_requests", Value: fmt.Sprintf("%d", resp.Summary.TotalRequests)},
		{Key: "total_input_tokens", Value: fmt.Sprintf("%d", resp.Summary.TotalInputTokens)},
		{Key: "total_output_tokens", Value: fmt.Sprintf("%d", resp.Summary.TotalOutputTokens)},
		{Key: "total_images", Value: fmt.Sprintf("%d", resp.Summary.TotalImages)},
		{Key: "total_cost_cents", Value: fmt.Sprintf("%d", resp.Summary.TotalCostCents)},
	}
	if resp.Summary.WalletBalanceCents != nil {
		pairs = append(pairs, kvPair{Key: "wallet_balance_cents", Value: fmt.Sprintf("%d", *resp.Summary.WalletBalanceCents)})
	}
	if resp.Summary.WalletReservedCents != nil {
		pairs = append(pairs, kvPair{Key: "wallet_reserved_cents", Value: fmt.Sprintf("%d", *resp.Summary.WalletReservedCents)})
	}
	if resp.Summary.OverageEnabled != nil {
		pairs = append(pairs, kvPair{Key: "overage_enabled", Value: fmt.Sprintf("%t", *resp.Summary.OverageEnabled)})
	}
	if resp.TierLimit != nil {
		pairs = append(pairs,
			kvPair{Key: "tier_spend_limit_cents", Value: fmt.Sprintf("%d", resp.TierLimit.SpendLimitCents)},
			kvPair{Key: "tier_current_period_cost_cents", Value: fmt.Sprintf("%d", resp.TierLimit.CurrentPeriodCostCents)},
			kvPair{Key: "tier_percentage_used", Value: fmt.Sprintf("%.2f", resp.TierLimit.PercentageUsed)},
			kvPair{Key: "tier_is_near_limit", Value: fmt.Sprintf("%t", resp.TierLimit.IsNearLimit)},
		)
	}
	printKeyValueTable(pairs)
}

func printUsageSummaryDetails(summary usageSummary) {
	pairs := []kvPair{
		{Key: "plan_type", Value: summary.PlanType},
		{Key: "window_start", Value: summary.WindowStart.Format(time.RFC3339)},
		{Key: "window_end", Value: summary.WindowEnd.Format(time.RFC3339)},
		{Key: "limit", Value: fmt.Sprintf("%d", summary.Limit)},
		{Key: "used", Value: fmt.Sprintf("%d", summary.Used)},
		{Key: "remaining", Value: fmt.Sprintf("%d", summary.Remaining)},
		{Key: "state", Value: summary.State},
		{Key: "images", Value: fmt.Sprintf("%d", summary.Images)},
	}
	printKeyValueTable(pairs)
}

func printTiersTable(tiers []generated.Tier) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCODE\tDISPLAY_NAME\tSPEND_LIMIT_CENTS\tPRICE_CENTS\tINTERVAL")
	for _, tier := range tiers {
		spend := uint64(0)
		if tier.SpendLimitCents != nil {
			spend = *tier.SpendLimitCents
		}
		price := uint64(0)
		if tier.PriceAmountCents != nil {
			price = *tier.PriceAmountCents
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n",
			formatUUIDPtr(tier.Id),
			stringOrEmpty(tier.TierCode),
			stringOrEmpty(tier.DisplayName),
			spend,
			price,
			stringOrEmpty(tier.PriceInterval),
		)
	}
	_ = w.Flush()
}

func printTierDetails(tier generated.Tier) {
	spend := uint64(0)
	if tier.SpendLimitCents != nil {
		spend = *tier.SpendLimitCents
	}
	price := uint64(0)
	if tier.PriceAmountCents != nil {
		price = *tier.PriceAmountCents
	}
	trial := uint32(0)
	if tier.TrialDays != nil {
		trial = *tier.TrialDays
	}
	pairs := []kvPair{
		{Key: "id", Value: formatUUIDPtr(tier.Id)},
		{Key: "project_id", Value: formatUUIDPtr(tier.ProjectId)},
		{Key: "tier_code", Value: stringOrEmpty(tier.TierCode)},
		{Key: "display_name", Value: stringOrEmpty(tier.DisplayName)},
		{Key: "spend_limit_cents", Value: fmt.Sprintf("%d", spend)},
		{Key: "price_amount_cents", Value: fmt.Sprintf("%d", price)},
		{Key: "price_currency", Value: stringOrEmpty(tier.PriceCurrency)},
		{Key: "price_interval", Value: stringOrEmpty(tier.PriceInterval)},
		{Key: "trial_days", Value: fmt.Sprintf("%d", trial)},
		{Key: "created_at", Value: formatTime(tier.CreatedAt)},
		{Key: "updated_at", Value: formatTime(tier.UpdatedAt)},
	}
	printKeyValueTable(pairs)
}
