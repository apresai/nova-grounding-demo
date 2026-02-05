package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"
)

const (
	grokModelID     = "grok-4"
	grokAPIEndpoint = "https://api.x.ai/v1/responses"
)

func init() {
	Register(&GrokProvider{})
}

// GrokProvider implements Provider for Grok via xAI API.
type GrokProvider struct{}

func (p *GrokProvider) Name() string        { return "grok" }
func (p *GrokProvider) DisplayName() string { return "Grok 4 (xAI)" }
func (p *GrokProvider) Emoji() string       { return "⚫" }

func (p *GrokProvider) CheckAuth() error {
	if os.Getenv("XAI_API_KEY") == "" {
		return fmt.Errorf("XAI_API_KEY not set")
	}
	return nil
}

func (p *GrokProvider) Query(ctx context.Context, query string, verbose bool) Result {
	start := time.Now()
	result := Result{}

	apiKey := os.Getenv("XAI_API_KEY")

	if verbose {
		fmt.Printf("  [Grok] Sending request with web search...\n")
	}

	reqBody := grokRequest{
		Model: grokModelID,
		Input: []grokMessage{
			{Role: "user", Content: query},
		},
		Tools: []grokTool{
			{Type: "web_search"},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		result.Error = fmt.Errorf("marshal error: %w", err)
		return result
	}

	req, err := http.NewRequestWithContext(ctx, "POST", grokAPIEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		result.Error = fmt.Errorf("request error: %w", err)
		return result
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = fmt.Errorf("API error: %w", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("read error: %w", err)
		return result
	}

	var grokResp grokResponse
	if err := json.Unmarshal(body, &grokResp); err != nil {
		result.Error = fmt.Errorf("parse error: %w", err)
		return result
	}

	// Extract token usage
	if grokResp.Usage != nil {
		result.Tokens.Input = grokResp.Usage.InputTokens
		result.Tokens.Output = grokResp.Usage.OutputTokens
	}

	parseGrokResponse(&grokResp, &result)
	return result
}

// --- Grok API Types ---

type grokRequest struct {
	Model string        `json:"model"`
	Input []grokMessage `json:"input"`
	Tools []grokTool    `json:"tools,omitempty"`
}

type grokMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type grokTool struct {
	Type string `json:"type"`
}

type grokResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content,omitempty"`
		Action struct {
			Type    string `json:"type"`
			Query   string `json:"query"`
			URL     string `json:"url"`
			Sources []struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"sources"`
		} `json:"action,omitempty"`
	} `json:"output"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

func parseGrokResponse(resp *grokResponse, result *Result) {
	// Get the main text response
	result.Text = resp.OutputText

	// Extract text from Output array content blocks
	if result.Text == "" {
		for _, out := range resp.Output {
			for _, content := range out.Content {
				if content.Type == "output_text" && content.Text != "" {
					result.Text = content.Text
					break
				}
			}
			if result.Text != "" {
				break
			}
		}
	}

	seen := make(map[string]bool)

	// Extract citations from markdown links in text [[n]](url) pattern
	linkRegex := regexp.MustCompile(`\[\[(\d+)\]\]\((https?://[^\)]+)\)`)
	matches := linkRegex.FindAllStringSubmatch(result.Text, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			DeduplicateCitations(&result.Citations, seen, Citation{
				URL: match[2],
			})
		}
	}

	// Also extract from web_search_call action sources
	for _, out := range resp.Output {
		if out.Type == "web_search_call" && out.Action.Type == "search" {
			for _, src := range out.Action.Sources {
				DeduplicateCitations(&result.Citations, seen, Citation{
					URL:   src.URL,
					Title: src.Title,
				})
			}
		}
	}
}
