# aiflow

Multi-AI prompt workflow CLI. DeepSeek clarifies your task, then routes it to the right model automatically.

```
text task       → Gemini
routine code    → Qwen Coder
architecture    → Claude Sonnet
```

## Install

```fish
git clone https://github.com/user/aiflow
cd aiflow
go build -ldflags="-s -w" -o aiflow ./cmd/aiflow
sudo mv aiflow /usr/local/bin/
```

Or install to `~/.local/bin` (make sure it's in `$PATH`):
```fish
go build -ldflags="-s -w" -o ~/.local/bin/aiflow ./cmd/aiflow
```

## Setup

```fish
aiflow init
# writes ~/.config/aiflow/config.json

# Add to ~/.config/fish/config.fish:
set -x OPENROUTER_API_KEY "sk-or-..."
set -x ANTHROPIC_API_KEY  "sk-ant-..."
```

## Usage

### New task
```fish
aiflow new "build a REST API for user auth in Go"
```
- Calls DeepSeek to generate 2–5 clarifying questions
- Creates `build-a-rest-api.ai` with questions
- Opens your editor (`$EDITOR` or nvim)
- Fill in `A1:`, `A2:` etc, save and quit

### Run
```fish
aiflow run build-a-rest-api.ai
```
- DeepSeek reads answers, builds final prompt, picks route
- Routes to Gemini / Qwen / Claude
- Streams response to terminal
- Saves output to `build-a-rest-api.output.md`

### Continue the conversation
```fish
aiflow chat build-a-rest-api.ai "now add refresh token support"
```
- Full history is preserved in the `.ai` file
- Appends to the `.output.md` file

### Inspect a file
```fish
aiflow show build-a-rest-api.ai
```

## .ai file format

```
[TASK]
Build a REST API for user auth in Go

[QUESTIONS]
Q1: Should it use JWT or sessions?
A1: JWT, stateless
Q2: Which database?
A2: PostgreSQL with sqlx

[PROMPT]
<<FINAL>>
Build a production-ready REST API in Go for user authentication...

[META]
route: quality_code
provider: claude
reason: Architecture decision with complex security requirements

[HISTORY]
role: user
Build a production-ready REST API...
---
role: assistant
Here's the implementation...
```

## Config

`~/.config/aiflow/config.json` — fully customisable:

```json
{
  "providers": [
    {
      "name": "deepseek",
      "type": "openrouter",
      "api_key": "$OPENROUTER_API_KEY",
      "model": "deepseek/deepseek-chat",
      "base_url": "https://openrouter.ai/api/v1"
    },
    {
      "name": "my-local-llama",
      "type": "ollama",
      "model": "llama3.1:8b",
      "base_url": "http://localhost:11434"
    },
    {
      "name": "claude",
      "type": "anthropic",
      "api_key": "$ANTHROPIC_API_KEY",
      "model": "claude-sonnet-4-20250514",
      "base_url": "https://api.anthropic.com"
    }
  ],
  "router": {
    "clarify_provider": "deepseek",
    "routes": {
      "text":         "gemini",
      "code":         "qwen",
      "quality_code": "claude"
    }
  },
  "editor": "nvim"
}
```

### Adding a local model (Ollama)

1. Add to `providers` array with `"type": "ollama"`
2. Change any route to point to it:
   ```json
   "routes": { "code": "my-local-llama" }
   ```

### Using a custom OpenAI-compatible API

Use `"type": "openai_compat"` with any `base_url`.

## Provider types

| type | Works with |
|------|-----------|
| `openrouter` | OpenRouter (Gemini, Qwen, DeepSeek, Mistral, ...) |
| `anthropic` | Claude API (streaming native) |
| `ollama` | Local Ollama models |
| `openai_compat` | Any OpenAI-compatible endpoint |
