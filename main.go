package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"

	schema "github.com/modelrelay/modelrelay/providers/schema"
	sdk "github.com/modelrelay/modelrelay/sdk/go"
	"github.com/modelrelay/modelrelay/sdk/go/generated"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// version is set by -ldflags at build time
var version = "dev"

const defaultAPIBaseURL = "https://api.modelrelay.ai/api/v1"

func clientHeader() string {
	return "mrl/" + version
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "agent":
		runAgent(os.Args[2:])
	case "models":
		runModels(os.Args[2:])
	case "schema":
		runSchema(os.Args[2:])
	case "-v", "--version", "version":
		fmt.Printf("mrl %s\n", version)
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("ModelRelay CLI (mrl)")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  mrl agent test <slug> [flags]")
	fmt.Println("  mrl agent run <slug> [flags]")
	fmt.Println("  mrl models [flags]")
	fmt.Println("  mrl schema lint <path> [flags]")
	fmt.Println("")
	fmt.Println("Global flags:")
	fmt.Println("  --api-key        Secret API key (mr_sk_*) [env: MODELRELAY_API_KEY]")
	fmt.Println("  --base-url       API base URL override [env: MODELRELAY_API_BASE_URL]")
	fmt.Println("  --project        Project UUID [env: MODELRELAY_PROJECT_ID]")
}

func runSchema(args []string) {
	if len(args) == 0 {
		printSchemaUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "lint":
		runSchemaLint(args[1:])
	case "-h", "--help", "help":
		printSchemaUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown schema command: %s\n", args[0])
		printSchemaUsage()
		os.Exit(1)
	}
}

func printSchemaUsage() {
	fmt.Println("Usage:")
	fmt.Println("  mrl schema lint <path|-> [flags]")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  --provider     openai | anthropic | googleai | xai (optional)")
	fmt.Println("  --tool-schema  Validate as tool parameters (OpenAI only)")
}

func runAgent(args []string) {
	if len(args) == 0 {
		printAgentUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "test":
		runAgentTest(args[1:])
	case "run":
		runAgentRun(args[1:])
	case "-h", "--help", "help":
		printAgentUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown agent command: %s\n", args[0])
		printAgentUsage()
		os.Exit(1)
	}
}

func printAgentUsage() {
	fmt.Println("Usage:")
	fmt.Println("  mrl agent test <slug> [flags]")
	fmt.Println("  mrl agent run <slug> [flags]")
	fmt.Println("")
	fmt.Println("Input flags:")
	fmt.Println("  --input         Inline user input (defaults to remaining args)")
	fmt.Println("  --input-file    Path to JSON array of input items")
	fmt.Println("  --mock-tools    Path to JSON object of mock tool outputs (test/replay)")
	fmt.Println("")
	fmt.Println("Execution flags:")
	fmt.Println("  --model                 Override model")
	fmt.Println("  --max-steps             Override max tool steps")
	fmt.Println("  --max-duration-sec      Override max run duration")
	fmt.Println("  --step-timeout-sec      Override per-step timeout")
	fmt.Println("  --tool-failure-policy   stop | continue | retry")
	fmt.Println("  --tool-retry-count      Retry count when policy is retry")
	fmt.Println("  --capture-tool-io       Include step logs in response")
	fmt.Println("  --dry-run               Error if a tool is not mocked")
	fmt.Println("  --json                  Print full JSON response")
	fmt.Println("  --output                Write JSON response to file")
	fmt.Println("  --trace                 Print step summaries")
}

func runModels(args []string) {
	cfg, err := parseModelsFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, cancel := contextWithTimeout(cfg.timeout)
	defer cancel()

	models, err := fetchModels(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if cfg.jsonOutput {
		payload, _ := json.MarshalIndent(models, "", "  ")
		fmt.Println(string(payload))
		return
	}

	printModelsTable(models)
}

type schemaLintConfig struct {
	provider   string
	toolSchema bool
}

func runSchemaLint(args []string) {
	fs := flag.NewFlagSet("schema lint", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	var cfg schemaLintConfig
	fs.StringVar(&cfg.provider, "provider", "", "")
	fs.BoolVar(&cfg.toolSchema, "tool-schema", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	path := strings.TrimSpace(fs.Arg(0))
	if path == "" {
		fmt.Fprintln(os.Stderr, "schema path is required")
		os.Exit(1)
	}

	raw, err := readSchemaInput(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	normalized, err := schema.NormalizeJSON(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if strings.TrimSpace(cfg.provider) != "" {
		if err := validateSchemaForProvider(normalized, cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	fmt.Println("OK")
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

func runAgentTest(args []string) {
	cfg, slug, input, err := parseAgentFlags("agent test", args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client, err := newClient(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	mockTools, err := readMockTools(cfg.mockToolsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	req := sdk.AgentTestRequest{
		Input:     input,
		MockTools: mockTools,
		Options:   cfg.options,
	}

	ctx, cancel := contextWithTimeout(cfg.timeout)
	defer cancel()

	resp, err := client.Agents.Test(ctx, cfg.projectID, slug, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	handleAgentResponse(resp, cfg)
}

func runAgentRun(args []string) {
	cfg, slug, input, err := parseAgentFlags("agent run", args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client, err := newClient(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	req := sdk.AgentRunRequest{
		Input:      input,
		Options:    cfg.options,
		CustomerID: cfg.customerID,
	}

	ctx, cancel := contextWithTimeout(cfg.timeout)
	defer cancel()

	resp, err := client.Agents.Run(ctx, cfg.projectID, slug, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	handleAgentResponse(resp, cfg)
}

type agentCLIConfig struct {
	apiKey        string
	baseURL       string
	projectID     uuid.UUID
	customerID    *string
	inputText     string
	inputFile     string
	mockToolsPath string
	options       *sdk.AgentRunOptions
	outputPath    string
	jsonOutput    bool
	trace         bool
	timeout       time.Duration
}

type modelsCLIConfig struct {
	apiKey            string
	baseURL           string
	provider          string
	capability        string
	includeDeprecated bool
	jsonOutput        bool
	timeout           time.Duration
}

func parseModelsFlags(args []string) (modelsCLIConfig, error) {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	var cfg modelsCLIConfig
	fs.StringVar(&cfg.apiKey, "api-key", "", "")
	fs.StringVar(&cfg.baseURL, "base-url", "", "")
	fs.StringVar(&cfg.provider, "provider", "", "")
	fs.StringVar(&cfg.capability, "capability", "text_generation", "")
	fs.BoolVar(&cfg.includeDeprecated, "include-deprecated", false, "")
	fs.BoolVar(&cfg.jsonOutput, "json", false, "")
	timeout := fs.Duration("timeout", 30*time.Second, "")

	if err := fs.Parse(args); err != nil {
		return modelsCLIConfig{}, err
	}

	cfg.apiKey = firstNonEmpty(cfg.apiKey, os.Getenv("MODELRELAY_API_KEY"), os.Getenv("MODELRELAY_SECRET_KEY"))
	cfg.baseURL = firstNonEmpty(cfg.baseURL, os.Getenv("MODELRELAY_API_BASE_URL"), defaultAPIBaseURL)
	cfg.timeout = *timeout

	if strings.TrimSpace(cfg.apiKey) == "" {
		return modelsCLIConfig{}, errors.New("api key is required (use --api-key or MODELRELAY_API_KEY)")
	}
	if strings.TrimSpace(cfg.baseURL) == "" {
		return modelsCLIConfig{}, errors.New("base URL is required")
	}
	return cfg, nil
}

func parseAgentFlags(cmd string, args []string) (agentCLIConfig, string, []llm.InputItem, error) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	var cfg agentCLIConfig
	fs.StringVar(&cfg.apiKey, "api-key", "", "")
	fs.StringVar(&cfg.baseURL, "base-url", "", "")
	project := fs.String("project", "", "")
	customer := fs.String("customer", "", "")
	fs.StringVar(&cfg.inputText, "input", "", "")
	fs.StringVar(&cfg.inputFile, "input-file", "", "")
	fs.StringVar(&cfg.mockToolsPath, "mock-tools", "", "")
	fs.StringVar(&cfg.outputPath, "output", "", "")
	fs.BoolVar(&cfg.jsonOutput, "json", false, "")
	fs.BoolVar(&cfg.trace, "trace", false, "")
	model := fs.String("model", "", "")
	maxSteps := fs.Int("max-steps", -1, "")
	maxDuration := fs.Int("max-duration-sec", -1, "")
	stepTimeout := fs.Int("step-timeout-sec", -1, "")
	toolFailurePolicy := fs.String("tool-failure-policy", "", "")
	toolRetryCount := fs.Int("tool-retry-count", -1, "")
	captureToolIO := fs.Bool("capture-tool-io", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	timeout := fs.Duration("timeout", 2*time.Minute, "")

	if err := fs.Parse(args); err != nil {
		return agentCLIConfig{}, "", nil, err
	}

	slug := strings.TrimSpace(fs.Arg(0))
	if slug == "" {
		return agentCLIConfig{}, "", nil, errors.New("agent slug is required")
	}

	cfg.apiKey = firstNonEmpty(cfg.apiKey, os.Getenv("MODELRELAY_API_KEY"), os.Getenv("MODELRELAY_SECRET_KEY"))
	cfg.baseURL = firstNonEmpty(cfg.baseURL, os.Getenv("MODELRELAY_API_BASE_URL"))
	cfg.timeout = *timeout

	projectRaw := firstNonEmpty(*project, os.Getenv("MODELRELAY_PROJECT_ID"))
	if strings.TrimSpace(projectRaw) == "" {
		return agentCLIConfig{}, "", nil, errors.New("project ID is required (use --project or MODELRELAY_PROJECT_ID)")
	}
	projectID, err := uuid.Parse(strings.TrimSpace(projectRaw))
	if err != nil {
		return agentCLIConfig{}, "", nil, fmt.Errorf("invalid project ID: %w", err)
	}
	cfg.projectID = projectID

	if strings.TrimSpace(*customer) != "" {
		clean := strings.TrimSpace(*customer)
		cfg.customerID = &clean
	}

	opts := &sdk.AgentRunOptions{
		CaptureToolIo: captureToolIO,
		DryRun:        dryRun,
	}
	if strings.TrimSpace(*model) != "" {
		clean := strings.TrimSpace(*model)
		opts.Model = &clean
	}
	if *maxSteps >= 0 {
		val := *maxSteps
		opts.MaxSteps = &val
	}
	if *maxDuration >= 0 {
		val := *maxDuration
		opts.MaxDurationSec = &val
	}
	if *stepTimeout >= 0 {
		val := *stepTimeout
		opts.StepTimeoutSec = &val
	}
	if strings.TrimSpace(*toolFailurePolicy) != "" {
		clean := sdk.AgentRunOptionsToolFailurePolicy(strings.TrimSpace(*toolFailurePolicy))
		opts.ToolFailurePolicy = &clean
	}
	if *toolRetryCount >= 0 {
		val := *toolRetryCount
		opts.ToolRetryCount = &val
	}
	if opts.Model == nil && opts.MaxSteps == nil && opts.MaxDurationSec == nil && opts.StepTimeoutSec == nil &&
		opts.ToolFailurePolicy == nil && opts.ToolRetryCount == nil && opts.CaptureToolIo == nil && opts.DryRun == nil {
		opts = nil
	}
	cfg.options = opts

	input, err := resolveInput(cfg, fs.Args()[1:])
	if err != nil {
		return agentCLIConfig{}, "", nil, err
	}

	return cfg, slug, input, nil
}

func resolveInput(cfg agentCLIConfig, tail []string) ([]llm.InputItem, error) {
	if cfg.inputFile != "" {
		return readInputFile(cfg.inputFile)
	}
	text := strings.TrimSpace(cfg.inputText)
	if text == "" && len(tail) > 0 {
		text = strings.TrimSpace(strings.Join(tail, " "))
	}
	if text == "" {
		return nil, errors.New("input is required (use --input, --input-file, or provide trailing args)")
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

func newClient(cfg agentCLIConfig) (*sdk.Client, error) {
	if strings.TrimSpace(cfg.apiKey) == "" {
		return nil, errors.New("api key is required (use --api-key or MODELRELAY_API_KEY)")
	}
	key, err := sdk.ParseAPIKeyAuth(cfg.apiKey)
	if err != nil {
		return nil, err
	}
	opts := []sdk.Option{sdk.WithClientHeader(clientHeader())}
	if strings.TrimSpace(cfg.baseURL) != "" {
		opts = append(opts, sdk.WithBaseURL(cfg.baseURL))
	}
	return sdk.NewClientWithKey(key, opts...)
}

func fetchModels(ctx context.Context, cfg modelsCLIConfig) ([]generated.Model, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.baseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/models"
	q := u.Query()
	if strings.TrimSpace(cfg.provider) != "" {
		q.Set("provider", strings.TrimSpace(cfg.provider))
	}
	if strings.TrimSpace(cfg.capability) != "" {
		q.Set("capability", strings.TrimSpace(cfg.capability))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-ModelRelay-Api-Key", cfg.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload generated.ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	if cfg.includeDeprecated {
		return payload.Models, nil
	}

	models := make([]generated.Model, 0, len(payload.Models))
	for _, model := range payload.Models {
		if model.Deprecated {
			continue
		}
		models = append(models, model)
	}
	return models, nil
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

func handleAgentResponse(resp sdk.AgentRunResponse, cfg agentCLIConfig) {
	jsonPayload, _ := json.MarshalIndent(resp, "", "  ")
	if cfg.outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.outputPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(cfg.outputPath, jsonPayload, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
			os.Exit(1)
		}
	}

	if cfg.jsonOutput {
		fmt.Println(string(jsonPayload))
		return
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

	if cfg.trace && resp.Steps != nil {
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
			if part.Text == nil {
				continue
			}
			builder.WriteString(*part.Text)
		}
	}
	return strings.TrimSpace(builder.String())
}

func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), timeout)
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}
