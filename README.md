# ModelRelay CLI (mrl)

A lightweight CLI that uses the Go SDK to run and test agents.

## Setup

```bash
export MODELRELAY_API_KEY=mr_sk_...
export MODELRELAY_PROJECT_ID=... # UUID
```

## Run an agent

```bash
go run ./cmd/mrl agent run researcher --input "Analyze Q4 sales"
```

## Test an agent with mocked tools

```bash
go run ./cmd/mrl agent test researcher \
  --input "Analyze Q4 sales" \
  --mock-tools ./mocks.json \
  --trace
```

## JSON input file

```bash
go run ./cmd/mrl agent test researcher \
  --input-file ./inputs.json \
  --output ./trace.json \
  --json
```

## List models

```bash
go run ./cmd/mrl models
```

Filter by provider/capability and include deprecated:

```bash
go run ./cmd/mrl models --provider openai --capability text_generation
go run ./cmd/mrl models --include-deprecated --json
```

## Lint a JSON schema

```bash
go run ./cmd/mrl schema lint ./schema.json
```

Validate provider compatibility:

```bash
go run ./cmd/mrl schema lint ./schema.json --provider openai
go run ./cmd/mrl schema lint ./tool-schema.json --provider openai --tool-schema
```
