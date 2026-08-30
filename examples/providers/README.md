# Provider Reference Configs

One file per provider showing the correct configuration format. Copy into your project's `settings.providers` block.

## Files

| File | Provider | Notes |
|------|----------|-------|
| `openai.yaml` | OpenAI | Direct API access |
| `anthropic.yaml` | Anthropic | Claude models |
| `gemini.yaml` | Google Gemini | Gemini models |
| `ollama.yaml` | Ollama | Local models, no API key needed |
| `litellm.yaml` | LiteLLM | Proxy to any provider via `base_url` |
| `nvidia.yaml` | NVIDIA NIM | OpenAI-compatible, free tier via build.nvidia.com (DeepSeek, Llama, Nemotron) |
| `multi-provider.yaml` | Mixed | Fallback chain across multiple providers |

## Usage

```bash
# These are reference configs, not runnable on their own.
# Copy the provider block into any example:

rakitsu run examples/single/01-chat/config.yaml "Hello"
```

## API Keys

All providers use env var interpolation:

```yaml
api_key: ${OPENAI_API_KEY}
api_key: ${ANTHROPIC_API_KEY}
api_key: ${GEMINI_API_KEY}
api_key: ${NVIDIA_API_KEY}
```

Ollama and LiteLLM don't require API keys when running locally.
