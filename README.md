# 🔍 Web Search CLI

**Compare AI models with real-time web search grounding**

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![AWS](https://img.shields.io/badge/AWS-Bedrock-FF9900?logo=amazonaws)](https://aws.amazon.com/bedrock/)
[![Anthropic](https://img.shields.io/badge/Anthropic-Claude-7C3AED)](https://anthropic.com)
[![Google](https://img.shields.io/badge/Google-Gemini-4285F4?logo=google)](https://ai.google.dev)
[![xAI](https://img.shields.io/badge/xAI-Grok-000000)](https://x.ai)

A CLI tool that sends the same query to multiple AI providers and compares their web-grounded responses. See which model gives the best citations, most comprehensive answers, and best value for cost.

## ✨ Features

- **🚀 Parallel Execution** — Query all 4 providers simultaneously
- **📊 Smart Ranking** — Score responses by citations + comprehensiveness
- **💰 Cost Tracking** — Token usage + estimated search fees per provider
- **🔗 Citation Extraction** — Unified source list across all models
- **⚡ Graceful Degradation** — Missing API keys skip gracefully, others continue

## 🎬 Demo

```
$ ./web-search -q "What is happening at Davos 2025?"

╔══════════════════════════════════════════════════════════════╗
║                    WEB SEARCH CLI                            ║
║     Compare AI models with real-time web search              ║
╚══════════════════════════════════════════════════════════════╝

📝 Query: What is happening at Davos 2025?

🚀 Running query against 4 models in parallel...

┌─ 🥇 #1 🔵 Gemini 3 Pro (32.7s)
│ 📊 523 words | 7 citations | score: 120
│ 💰 ~$0.0462 est. (tokens: $0.0112 + search: ~$0.0350)
│
│ The World Economic Forum Annual Meeting 2025 took place from
│ January 20-24, 2025 in Davos, Switzerland...
│
│ 📎 Sources:
│   [1] World Economic Forum - weforum.org
│   [2] BNP Paribas Analysis - cib.bnpparibas
│   ...
└────────────────────────────────────────────────────────────

╔══════════════════════════════════════════════════════════════════════╗
║                        RANKING & PERFORMANCE                         ║
╠══════════════════════════════════════════════════════════════════════╣
║ 🥇 🔵 Gemini 3 Pro           ✅ │  523 words │  7 cites │ ~$0.0462  ║
║ 🥈 ⚫ Grok 4 (xAI)           ✅ │  522 words │  4 cites │ ~$0.0638  ║
║ 🥉 🟠 Nova Premier (AWS)     ✅ │  300 words │  5 cites │ ~$0.0182  ║
║    🟣 Claude 4.5 Sonnet      ✅ │  223 words │  5 cites │ ~$0.0544  ║
╠══════════════════════════════════════════════════════════════════════╣
║ 💰 TOTAL EST. COST: ~$0.1825                                         ║
║ 🏆 WINNER: Gemini 3 Pro                                              ║
╠══════════════════════════════════════════════════════════════════════╣
║ ⚠️  Costs are estimates. Search/grounding fees vary by provider.     ║
╚══════════════════════════════════════════════════════════════════════╝
```

## 🏗️ Supported Providers

| Provider | Model | Grounding Method | Search Cost |
|----------|-------|------------------|-------------|
| 🟠 **Nova** | Nova Premier | AWS Bedrock `nova_grounding` | ~$0.01/query |
| 🟣 **Claude** | Claude 4.5 Sonnet | `web_search_20250305` tool | $0.01/search |
| 🔵 **Gemini** | Gemini 3 Pro | Google Search grounding | $0.035/query |
| ⚫ **Grok** | Grok 4 | xAI `web_search` | Included |

## 📦 Installation

```bash
# Clone the repository
git clone https://github.com/apresai/nova-grounding-demo.git
cd nova-grounding-demo

# Build
make build

# Or with Go directly
go build -o web-search .
```

## ⚙️ Configuration

Set your API keys as environment variables:

```bash
# Claude (Anthropic)
export ANTHROPIC_API_KEY="sk-ant-..."

# Gemini (Google)
export GOOGLE_API_KEY="..."
# or
export GEMINI_API_KEY="..."

# Grok (xAI)
export XAI_API_KEY="..."

# Nova (AWS) - uses standard AWS credentials
# Via ~/.aws/credentials or:
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."
```

**Tip:** Add these to `~/.zshrc` or a secrets file that gets sourced.

## 🚀 Usage

```bash
# Compare all providers (default)
./web-search -q "Latest news on AI regulation"

# Single provider
./web-search -q "Bitcoin price today" -model claude

# Verbose mode (shows timing details)
./web-search -q "SpaceX launches" -v

# Show model thinking/reasoning
./web-search -q "Explain quantum computing" -thinking
```

### Available Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-q` | Query to search (required) | — |
| `-model` | Provider: `nova`, `claude`, `gemini`, `grok`, `all` | `all` |
| `-v` | Verbose output with debug info | `false` |
| `-thinking` | Show model reasoning traces | `false` |

### Make Targets

```bash
make build          # Build the binary
make run            # Build + run default query
make query Q="..."  # Build + run custom query
make nova Q="..."   # Run single provider
make clean          # Remove binary
make help           # Show CLI help
```

## 📐 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         main.go                             │
│  CLI parsing, parallel orchestration, runAllModels()        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       provider.go                           │
│  Provider interface, Result/Citation types, registry        │
│  Pricing maps, cost calculation                             │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   nova.go    │    │  claude.go   │    │  gemini.go   │  ...
│  Bedrock SDK │    │ Anthropic SDK│    │  Google SDK  │
│  init() reg  │    │  init() reg  │    │  init() reg  │
└──────────────┘    └──────────────┘    └──────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                       display.go                            │
│  Output formatting, rankings, cost display, key points      │
└─────────────────────────────────────────────────────────────┘
```

### Provider Interface

Each provider implements:

```go
type Provider interface {
    Name() string                                           // "claude"
    DisplayName() string                                    // "Claude 4.5 Sonnet"
    Emoji() string                                          // "🟣"
    CheckAuth() error                                       // Validate credentials
    Query(ctx context.Context, query string, verbose bool) Result
}
```

Providers self-register via `init()` — no manual wiring needed.

## ➕ Adding a New Provider

1. Create `newprovider.go`:

```go
package main

func init() {
    Register(&NewProvider{})
}

type NewProvider struct{}

func (p *NewProvider) Name() string        { return "newprovider" }
func (p *NewProvider) DisplayName() string { return "New Provider" }
func (p *NewProvider) Emoji() string       { return "🟢" }
func (p *NewProvider) CheckAuth() error    { /* check API key */ }
func (p *NewProvider) Query(ctx context.Context, query string, verbose bool) Result {
    // Implement API call + parse response
}
```

2. Add pricing to `provider.go`:

```go
var Pricing = map[string]struct{ Input, Output float64 }{
    // ...existing...
    "newprovider": {2.00, 8.00},  // per million tokens
}

var SearchCost = map[string]float64{
    // ...existing...
    "newprovider": 0.02,  // per query, or 0 if included
}
```

3. Build and test:

```bash
make build
./web-search -q "test" -model newprovider
```

See [PROVIDERS.md](PROVIDERS.md) for detailed documentation.

## 💰 Cost Breakdown

Costs shown include **token usage + estimated search fees**:

| Provider | Input Tokens | Output Tokens | Search Fee |
|----------|-------------|---------------|------------|
| Nova | $2.50/M | $12.50/M | ~$0.01 |
| Claude | $3.00/M | $15.00/M | $0.01 |
| Gemini | $2.00/M | $12.00/M | $0.035 |
| Grok | $3.00/M | $15.00/M | Included |

> ⚠️ Search costs are estimates. Check provider documentation for current pricing.

## 📁 Project Structure

```
nova-grounding-demo/
├── main.go           # CLI + orchestration (180 lines)
├── provider.go       # Interface + registry (115 lines)
├── display.go        # Output formatting (263 lines)
├── nova.go           # AWS Bedrock provider
├── claude.go         # Anthropic provider
├── gemini.go         # Google AI provider
├── grok.go           # xAI provider
├── PROVIDERS.md      # Guide for adding providers
├── CLAUDE.md         # AI assistant guidance
├── Makefile          # Build targets
└── go.mod            # Go module definition
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/new-provider`)
3. Add your provider following the [Adding a New Provider](#-adding-a-new-provider) guide
4. Test with all providers (`./web-search -q "test" -model all`)
5. Submit a pull request

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

<p align="center">
  <sub>Built with ❤️ to compare AI grounding capabilities</sub>
</p>
