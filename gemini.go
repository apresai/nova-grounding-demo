package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

const geminiModelID = "gemini-3-pro-preview"

func init() {
	Register(&GeminiProvider{})
}

// GeminiProvider implements Provider for Gemini via Google AI API.
type GeminiProvider struct{}

func (p *GeminiProvider) Name() string        { return "gemini" }
func (p *GeminiProvider) DisplayName() string { return "Gemini 3 Pro" }
func (p *GeminiProvider) Emoji() string       { return "🔵" }

func (p *GeminiProvider) CheckAuth() error {
	if os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("GEMINI_API_KEY") == "" {
		return fmt.Errorf("GOOGLE_API_KEY not set")
	}
	return nil
}

func (p *GeminiProvider) Query(ctx context.Context, query string, verbose bool) Result {
	start := time.Now()
	result := Result{}

	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		result.Error = fmt.Errorf("client error: %w", err)
		return result
	}

	if verbose {
		fmt.Printf("  [Gemini] Sending request with Google Search grounding...\n")
	}

	googleSearchTool := &genai.Tool{
		GoogleSearch: &genai.GoogleSearch{},
	}

	resp, err := client.Models.GenerateContent(ctx, geminiModelID, genai.Text(query), &genai.GenerateContentConfig{
		Tools: []*genai.Tool{googleSearchTool},
	})
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = fmt.Errorf("API error: %w", err)
		return result
	}

	// Extract token usage
	if resp.UsageMetadata != nil {
		result.Tokens.Input = int(resp.UsageMetadata.PromptTokenCount)
		result.Tokens.Output = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	parseGeminiResponse(resp, &result)
	return result
}

func parseGeminiResponse(resp *genai.GenerateContentResponse, result *Result) {
	if resp == nil || len(resp.Candidates) == 0 {
		return
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil {
		return
	}

	var textBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			textBuilder.WriteString(part.Text)
		}
	}
	result.Text = textBuilder.String()

	if candidate.GroundingMetadata != nil {
		seen := make(map[string]bool)
		for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
			if chunk.Web != nil {
				DeduplicateCitations(&result.Citations, seen, Citation{
					URL:   chunk.Web.URI,
					Title: chunk.Web.Title,
				})
			}
		}
	}
}
