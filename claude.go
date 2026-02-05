package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const claudeModelID = "claude-sonnet-4-5-20250929"

func init() {
	Register(&ClaudeProvider{})
}

// ClaudeProvider implements Provider for Claude via Anthropic API.
type ClaudeProvider struct{}

func (p *ClaudeProvider) Name() string        { return "claude" }
func (p *ClaudeProvider) DisplayName() string { return "Claude 4.5 Sonnet" }
func (p *ClaudeProvider) Emoji() string       { return "🟣" }

func (p *ClaudeProvider) CheckAuth() error {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	return nil
}

func (p *ClaudeProvider) Query(ctx context.Context, query string, verbose bool) Result {
	start := time.Now()
	result := Result{}

	client := anthropic.NewClient()

	if verbose {
		fmt.Printf("  [Claude] Sending request with web_search tool...\n")
	}

	message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     claudeModelID,
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(query)),
		},
		Tools: []anthropic.ToolUnionParam{
			{
				OfWebSearchTool20250305: &anthropic.WebSearchTool20250305Param{
					Name: "web_search",
					Type: "web_search_20250305",
				},
			},
		},
	})

	result.Duration = time.Since(start)

	if err != nil {
		result.Error = fmt.Errorf("API error: %w", err)
		return result
	}

	// Extract token usage
	result.Tokens.Input = int(message.Usage.InputTokens)
	result.Tokens.Output = int(message.Usage.OutputTokens)

	parseClaudeResponse(message, &result)
	return result
}

func parseClaudeResponse(message *anthropic.Message, result *Result) {
	var textBuilder strings.Builder
	seen := make(map[string]bool)

	for _, block := range message.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			textBuilder.WriteString(b.Text)
			for _, citation := range b.Citations {
				if citation.Type == "web_search_result_location" && citation.URL != "" {
					DeduplicateCitations(&result.Citations, seen, Citation{
						URL:   citation.URL,
						Title: citation.Title,
					})
				}
			}
		}
	}

	result.Text = textBuilder.String()
}
