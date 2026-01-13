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

type customerListResponse struct {
	Customers []generated.CustomerWithSubscription `json:"customers"`
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
			if strings.TrimSpace(cfg.APIKey) == "" {
				return errors.New("api key required")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp customerListResponse
			if err := doJSON(ctx, cfg, authModeAPIKey, http.MethodGet, "/customers", nil, &resp); err != nil {
				return err
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
			if strings.TrimSpace(cfg.APIKey) == "" {
				return errors.New("api key required")
			}
			customerID := strings.TrimSpace(args[0])
			if _, err := uuid.Parse(customerID); err != nil {
				return errors.New("invalid customer id")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			path := fmt.Sprintf("/customers/%s", customerID)
			body, err := doJSONRaw(ctx, cfg, authModeAPIKey, http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			customer, err := decodeCustomer(body)
			if err != nil {
				return err
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
			if strings.TrimSpace(cfg.APIKey) == "" {
				return errors.New("api key required")
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

			body, err := doJSONRaw(ctx, cfg, authModeAPIKey, http.MethodPost, "/customers", payload)
			if err != nil {
				return err
			}
			customer, err := decodeCustomer(body)
			if err != nil {
				return err
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
	cmd.AddCommand(newUsageAccountCmd())
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
			if strings.TrimSpace(cfg.APIKey) == "" {
				return errors.New("api key required")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp usageSummaryResponse
			if err := doJSON(ctx, cfg, authModeAPIKey, http.MethodGet, "/llm/usage", nil, &resp); err != nil {
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
			if strings.TrimSpace(cfg.APIKey) == "" {
				return errors.New("api key required")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp tierListResponse
			if err := doJSON(ctx, cfg, authModeAPIKey, http.MethodGet, "/tiers", nil, &resp); err != nil {
				return err
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
			if strings.TrimSpace(cfg.APIKey) == "" {
				return errors.New("api key required")
			}
			tierID := strings.TrimSpace(args[0])
			if _, err := uuid.Parse(tierID); err != nil {
				return errors.New("invalid tier id")
			}

			ctx, cancel := contextWithTimeout(cfg.Timeout)
			defer cancel()

			var resp tierResponse
			path := fmt.Sprintf("/tiers/%s", tierID)
			if err := doJSON(ctx, cfg, authModeAPIKey, http.MethodGet, path, nil, &resp); err != nil {
				return err
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
