# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
make build                           # Build ./web-search binary
make run                             # Build + run with default query
make query Q="your question"         # Build + run custom query
make nova Q="question"               # Run single provider
./web-search -q "question" -model all   # Run all providers in parallel
./web-search -q "question" -model claude -v  # Single provider with verbose
```

## Environment Variables

API keys must be set (typically in `~/.secrets/shell.zsh`):
- `ANTHROPIC_API_KEY` - Claude
- `GOOGLE_API_KEY` or `GEMINI_API_KEY` - Gemini
- `XAI_API_KEY` - Grok
- AWS credentials via `~/.aws/credentials` - Nova

## Architecture

CLI tool comparing web search grounding across 4 AI providers. Uses a **Provider interface pattern** with auto-registration via `init()`.

### Key Files

| File | Purpose |
|------|---------|
| `provider.go` | `Provider` interface, `Result`/`Citation` types, registry (`Register`, `Get`, `All`), pricing maps |
| `main.go` | CLI flags, `runAllModels()` parallel execution, `runSingleModel()` |
| `display.go` | All output formatting, scoring (`calculateScore`), cost display |
| `{nova,claude,gemini,grok}.go` | Provider implementations |

### Provider Interface

```go
type Provider interface {
    Name() string        // "claude" - used for -model flag
    DisplayName() string // "Claude 4.5 Sonnet"
    Emoji() string       // "🟣"
    CheckAuth() error    // Validate credentials before query
    Query(ctx, query, verbose) Result
}
```

### Adding a New Provider

1. Create `newprovider.go` implementing `Provider`
2. Add `func init() { Register(&NewProvider{}) }`
3. Add pricing to `Pricing` and `SearchCost` maps in `provider.go`

See `PROVIDERS.md` for detailed guide.

### Cost Tracking

Two maps in `provider.go`:
- `Pricing` - token costs per million (input/output)
- `SearchCost` - estimated per-query grounding fees

`Result.EstimatedCost()` combines both for display.
