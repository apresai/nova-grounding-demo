# Adding a New Provider

This guide explains how to add a new AI provider to the web-search CLI.

## Quick Start

1. Create a new file: `myprovider.go`
2. Implement the `Provider` interface (5 methods)
3. Register with `init()`
4. Add pricing to `provider.go`
5. Build and test

## Provider Interface

```go
type Provider interface {
    Name() string        // Short identifier for -model flag
    DisplayName() string // Human-readable name for output
    Emoji() string       // Visual indicator in results
    CheckAuth() error    // Validate credentials, return nil if ready
    Query(ctx context.Context, query string, verbose bool) Result
}
```

## Step-by-Step Example

### 1. Create the Provider File

```go
// openai.go
package main

import (
    "context"
    "fmt"
    "os"
    "time"
)

func init() {
    Register(&OpenAIProvider{})
}

type OpenAIProvider struct{}

func (p *OpenAIProvider) Name() string        { return "openai" }
func (p *OpenAIProvider) DisplayName() string { return "GPT-4o" }
func (p *OpenAIProvider) Emoji() string       { return "🟢" }

func (p *OpenAIProvider) CheckAuth() error {
    if os.Getenv("OPENAI_API_KEY") == "" {
        return fmt.Errorf("OPENAI_API_KEY not set")
    }
    return nil
}

func (p *OpenAIProvider) Query(ctx context.Context, query string, verbose bool) Result {
    start := time.Now()
    result := Result{}

    // 1. Create API client
    // 2. Build request with web search tool
    // 3. Send request
    // 4. Parse response into result.Text
    // 5. Extract citations into result.Citations
    // 6. Set token counts: result.Tokens.Input, result.Tokens.Output

    result.Duration = time.Since(start)
    return result
}
```

### 2. Add Pricing

In `provider.go`, add your provider's pricing (per million tokens):

```go
var Pricing = map[string]struct{ Input, Output float64 }{
    "nova":    {2.50, 12.50},
    "claude":  {3.00, 15.00},
    "gemini":  {2.00, 12.00},
    "grok":    {3.00, 15.00},
    "openai":  {2.50, 10.00},  // Add your provider here
}
```

### 3. Build and Test

```bash
make build
./web-search -q "test query" -model openai
./web-search -q "test query" -model all
```

## Result Struct

```go
type Result struct {
    Text      string        // Response text from the model
    Citations []Citation    // Web sources used (URL, Title, Domain)
    Duration  time.Duration // Total API call time
    Tokens    TokenUsage    // Input/output token counts for cost
    Error     error         // nil on success
}

type TokenUsage struct {
    Input  int
    Output int
}

type Citation struct {
    URL    string
    Domain string
    Title  string
}
```

## Helper Functions

### Deduplicate Citations

Use the shared helper to avoid duplicate URLs:

```go
seen := make(map[string]bool)
for _, source := range apiResponse.Sources {
    DeduplicateCitations(&result.Citations, seen, Citation{
        URL:   source.URL,
        Title: source.Title,
    })
}
```

### Verbose Logging

Add debug output when verbose mode is enabled:

```go
if verbose {
    fmt.Printf("  [MyProvider] Sending request with web search...\n")
}
```

## Checklist

- [ ] Create `myprovider.go` with all 5 interface methods
- [ ] Add `func init() { Register(&MyProvider{}) }`
- [ ] Implement `CheckAuth()` to validate API key/credentials
- [ ] Extract token usage from API response for cost tracking
- [ ] Use `DeduplicateCitations()` helper for citations
- [ ] Add pricing to `provider.go`
- [ ] Test with `-model myprovider` and `-model all`

## File Structure

```
nova-grounding-demo/
├── main.go           # CLI + orchestration
├── provider.go       # Interface, types, registry
├── display.go        # Output formatting
├── nova.go           # Amazon Nova provider
├── claude.go         # Anthropic Claude provider
├── gemini.go         # Google Gemini provider
├── grok.go           # xAI Grok provider
├── myprovider.go     # Your new provider
└── PROVIDERS.md      # This documentation
```
