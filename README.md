# ModelRelay CLI (mrl)

A lightweight CLI for running and testing ModelRelay agents and managing resources.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install modelrelay/tap/mrl
```

To upgrade:

```bash
brew upgrade mrl
```

### Manual Download

Download the latest release from [releases.modelrelay.ai](https://releases.modelrelay.ai/mrl/) and add to your PATH.

### From Source

```bash
go install github.com/modelrelay/modelrelay/cmd/mrl@latest
```

Or build locally:

```bash
cd cmd/mrl && go build -o mrl
```

## Setup

Environment variables:

```bash
export MODELRELAY_API_KEY=mr_sk_...
export MODELRELAY_ACCESS_TOKEN=... # bearer token for control-plane commands
export MODELRELAY_PROJECT_ID=...    # UUID (optional default)
export MODELRELAY_API_BASE_URL=...  # optional
```

Config file (`~/.config/modelrelay/config.yaml`):

```yaml
current_profile: default
profiles:
  default:
    api_key: mr_sk_...
    token: <bearer token>
    base_url: https://api.modelrelay.ai/api/v1
    project_id: <uuid>
    output: table # or json
```

Manage config with:

```bash
mrl config set --profile dev --token <bearer> --api-key mr_sk_...
mrl config use dev
mrl config show
```

## Commands

### Run an agent

```bash
mrl agent run researcher --input "Analyze Q4 sales"
```

### Test an agent with mocked tools

```bash
mrl agent test researcher \
  --input "Analyze Q4 sales" \
  --mock-tools ./mocks.json \
  --trace
```

### JSON input file

```bash
mrl agent test researcher \
  --input-file ./inputs.json \
  --output ./trace.json \
  --json
```

### Run a local agentic tool loop

Enable the local `bash` tool (deny-by-default) and run a loop:

```bash
mrl agent loop \
  --model claude-sonnet-4-5 \
  --tool bash \
  --bash-allow "git " \
  --input "List recent commits and summarize them"
```

Include `tasks.write` for progress tracking (state handle optional):

```bash
mrl agent loop \
  --model claude-sonnet-4-5 \
  --tool bash \
  --tool tasks.write \
  --state-ttl-sec 86400 \
  --tasks-output ./tasks.json \
  --input "Audit this repo and track your progress"
```

Enable local filesystem tools (`fs.*`):

```bash
mrl agent loop \
  --model claude-sonnet-4-5 \
  --tool fs \
  --input "Search for TODOs in this repo"
```

### Tool manifest (TOML/JSON)

You can load tools from a manifest file. The format is chosen by file extension (`.toml` or `.json`). CLI flags override manifest values.

`tools.toml`:

```toml
tool_root = "."
tools = ["bash", "tasks.write"]
state_ttl_sec = 86400

[bash]
allow = ["git ", "rg "]
timeout = "15s"
max_output_bytes = 64000

[tasks_write]
output = "tasks.json"
print = true

[fs]
ignore_dirs = ["node_modules", ".git"]
search_timeout = "3s"

[[custom]]
name = "custom.echo"
description = "Echo input as JSON"
command = ["cat"]
schema = { type = "object", properties = { message = { type = "string" } }, required = ["message"] }
```

Run with:

```bash
mrl agent loop --model claude-sonnet-4-5 --tools-file ./tools.toml --input "Audit this repo"
```

### List models

```bash
mrl model list
```

Filter by provider/capability and include deprecated:

```bash
mrl model list --provider openai --capability text_generation
mrl model list --include-deprecated --json
```

### Lint a JSON schema

```bash
mrl schema lint ./schema.json
```

Validate provider compatibility:

```bash
mrl schema lint ./schema.json --provider openai
mrl schema lint ./tool-schema.json --provider openai --tool-schema
```

### Version

```bash
mrl version
```

## Control-Plane Commands

### Projects

```bash
mrl project list --token $MODELRELAY_ACCESS_TOKEN
mrl project get <project_id> --token $MODELRELAY_ACCESS_TOKEN
mrl project create --name "My Project" --token $MODELRELAY_ACCESS_TOKEN
```

### API Keys

```bash
mrl key list --token $MODELRELAY_ACCESS_TOKEN
mrl key create --name "ci-key" --project <project_id> --token $MODELRELAY_ACCESS_TOKEN
mrl key revoke <key_id> --token $MODELRELAY_ACCESS_TOKEN
```

### Customers

```bash
mrl customer list --project <project_id> --token $MODELRELAY_ACCESS_TOKEN
mrl customer get <customer_id> --api-key $MODELRELAY_API_KEY
mrl customer create --project <project_id> --external-id user_123 --email user@example.com --token $MODELRELAY_ACCESS_TOKEN
```

### Usage

```bash
mrl usage account --api-key $MODELRELAY_API_KEY
mrl usage customer <customer_id> --project <project_id> --token $MODELRELAY_ACCESS_TOKEN
```

### Tiers

```bash
mrl tier list --project <project_id> --token $MODELRELAY_ACCESS_TOKEN
mrl tier get <tier_id> --project <project_id> --token $MODELRELAY_ACCESS_TOKEN
```

## Output

Table output is the default. Use `--json` for machine-readable output.

## Releasing

To release a new version:

```bash
git tag mrl-v0.2.0 && git push origin mrl-v0.2.0
```

The workflow automatically builds binaries, uploads to R2, and updates the Homebrew tap.
