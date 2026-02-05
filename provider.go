package main

import (
	"context"
	"sort"
	"time"
)

// Provider defines the interface for AI model providers with web search.
type Provider interface {
	// Name returns the provider identifier (e.g., "nova", "claude")
	Name() string

	// DisplayName returns the human-friendly name (e.g., "Nova Premier (AWS)")
	DisplayName() string

	// Emoji returns the model's emoji indicator
	Emoji() string

	// CheckAuth returns nil if credentials are configured, or an error describing what's missing
	CheckAuth() error

	// Query performs a web-grounded search and returns the result
	Query(ctx context.Context, query string, verbose bool) Result
}

// Citation represents a web source citation.
type Citation struct {
	URL    string
	Domain string
	Title  string
}

// TokenUsage tracks token counts for cost calculation.
type TokenUsage struct {
	Input  int
	Output int
}

// Result holds a provider's response with performance metrics.
type Result struct {
	Text      string
	Citations []Citation
	Duration  time.Duration
	Tokens    TokenUsage
	Error     error
}

// Pricing per million tokens (USD).
var Pricing = map[string]struct{ Input, Output float64 }{
	"nova":   {2.50, 12.50},  // Nova Premier
	"claude": {3.00, 15.00},  // Claude 4.5 Sonnet
	"gemini": {2.00, 12.00},  // Gemini 3 Pro
	"grok":   {3.00, 15.00},  // Grok 4
}

// Cost calculates USD cost from token usage.
func (r Result) Cost(provider string) float64 {
	p, ok := Pricing[provider]
	if !ok {
		return 0
	}
	return (float64(r.Tokens.Input)*p.Input + float64(r.Tokens.Output)*p.Output) / 1_000_000
}

// --- Provider Registry ---

var providers = make(map[string]Provider)

// Register adds a provider to the registry.
func Register(p Provider) {
	providers[p.Name()] = p
}

// Get returns a provider by name.
func Get(name string) (Provider, bool) {
	p, ok := providers[name]
	return p, ok
}

// All returns all registered provider names (sorted).
func All() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// --- Shared Helpers ---

// DeduplicateCitations adds a citation if the URL hasn't been seen.
func DeduplicateCitations(citations *[]Citation, seen map[string]bool, c Citation) {
	if c.URL != "" && !seen[c.URL] {
		seen[c.URL] = true
		*citations = append(*citations, c)
	}
}
