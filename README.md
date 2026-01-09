# ModelRelay CLI (mrl)

A lightweight CLI for running and testing ModelRelay agents.

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

```bash
export MODELRELAY_API_KEY=mr_sk_...
export MODELRELAY_PROJECT_ID=... # UUID
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

### List models

```bash
mrl models
```

Filter by provider/capability and include deprecated:

```bash
mrl models --provider openai --capability text_generation
mrl models --include-deprecated --json
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
mrl --version
```

## Releasing

To release a new version:

```bash
git tag mrl-v0.2.0 && git push origin mrl-v0.2.0
```

The workflow automatically builds binaries, uploads to R2, and updates the Homebrew tap.
