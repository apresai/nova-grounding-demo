package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"google.golang.org/genai"
)

const (
	// Model IDs
	novaModelID   = "us.amazon.nova-premier-v1:0"
	claudeModelID = "claude-sonnet-4-5-20250929"
	geminiModelID = "gemini-3-pro-preview"
	grokModelID   = "grok-4"

	// API endpoints
	grokAPIEndpoint = "https://api.x.ai/v1/responses"

	// System tool names
	novaGroundingTool = "nova_grounding"
)

// Citation represents a web source citation
type Citation struct {
	URL    string
	Domain string
	Title  string
}

// ModelResponse holds a model's response with metadata
type ModelResponse struct {
	ModelName string
	Text      string
	Citations []Citation
	Duration  time.Duration
	Error     error
	WordCount int
	Score     int // Calculated score for ranking
}

// Global flags
var (
	showThinking bool
	verbose      bool
)

func main() {
	// Custom usage function for --help
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `
╔══════════════════════════════════════════════════════════════╗
║                    WEB SEARCH CLI                            ║
║  Compare AI models with real-time web search capabilities    ║
╚══════════════════════════════════════════════════════════════╝

USAGE:
  web-search [flags] -q "your question"

FLAGS:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
MODELS:
  nova     Amazon Nova Premier with AWS Bedrock grounding
  claude   Claude 4.5 Sonnet with Anthropic web_search tool
  gemini   Gemini 3 Pro with Google Search grounding
  grok     Grok 4 with xAI web search

ENVIRONMENT VARIABLES:
  AWS credentials      Required for Nova (via ~/.aws/credentials or env vars)
  ANTHROPIC_API_KEY    Required for Claude
  GOOGLE_API_KEY       Required for Gemini
  XAI_API_KEY          Required for Grok

EXAMPLES:
  # Compare all models (default)
  web-search -q "What happened in tech news today?"

  # Run single model
  web-search -model claude -q "Current Bitcoin price"

  # Verbose output with timing details
  web-search -v -q "Latest SpaceX launches"

  # Show model thinking/reasoning traces
  web-search -thinking -q "Who won the Super Bowl?"

`)
	}

	// Parse command line flags
	query := flag.String("q", "", "Question to ask (required)")
	model := flag.String("model", "all", "Model to use: nova, claude, gemini, grok, or all")
	thinking := flag.Bool("thinking", false, "Show model's thinking/reasoning traces")
	verboseFlag := flag.Bool("v", false, "Enable verbose output with timing details")
	flag.Parse()

	showThinking = *thinking || *verboseFlag
	verbose = *verboseFlag

	// Require a query
	if *query == "" {
		fmt.Fprintln(os.Stderr, "Error: -q flag is required. Use -h for help.")
		os.Exit(1)
	}

	printHeader()
	fmt.Printf("📝 Query: %s\n\n", *query)

	ctx := context.Background()

	switch *model {
	case "all":
		runAllModels(ctx, *query)
	case "nova":
		runSingleModel(ctx, "nova", *query)
	case "claude":
		runSingleModel(ctx, "claude", *query)
	case "gemini":
		runSingleModel(ctx, "gemini", *query)
	case "grok":
		runSingleModel(ctx, "grok", *query)
	default:
		fmt.Fprintf(os.Stderr, "Unknown model: %s\n", *model)
		fmt.Println("Available models: nova, claude, gemini, grok, all")
		os.Exit(1)
	}
}

func printHeader() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    WEB SEARCH CLI                            ║")
	fmt.Println("║     Nova | Claude 4.5 | Gemini 3 Pro | Grok 4                ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func runAllModels(ctx context.Context, query string) {
	fmt.Println("🚀 Running query against all models in parallel...")
	fmt.Println(strings.Repeat("═", 65))
	fmt.Println()

	var wg sync.WaitGroup
	results := make(chan ModelResponse, 4)

	// Launch all four models in parallel
	models := []string{"nova", "claude", "gemini", "grok"}
	for _, m := range models {
		wg.Add(1)
		go func(modelName string) {
			defer wg.Done()
			response := invokeModel(ctx, modelName, query)
			response.WordCount = len(strings.Fields(response.Text))
			response.Score = calculateScore(response)
			results <- response
		}(m)
	}

	// Wait for all to complete and close channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	responses := make([]ModelResponse, 0, 4)
	for resp := range results {
		responses = append(responses, resp)
	}

	// Sort by score (highest first)
	sort.Slice(responses, func(i, j int) bool {
		return responses[i].Score > responses[j].Score
	})

	// Print each response
	for i, resp := range responses {
		rank := i + 1
		printModelResponseWithRank(resp, rank)
		fmt.Println()
	}

	// Print comparison summary
	printComparisonSummary(responses)

	// Print combined summary
	printCombinedSummary(responses, query)
}

func runSingleModel(ctx context.Context, modelName, query string) {
	fmt.Printf("🔍 Running with %s...\n", getModelDisplayName(modelName))
	fmt.Println(strings.Repeat("─", 60))

	response := invokeModel(ctx, modelName, query)
	response.WordCount = len(strings.Fields(response.Text))
	response.Score = calculateScore(response)
	printModelResponse(response)
}

func calculateScore(resp ModelResponse) int {
	if resp.Error != nil {
		return 0
	}
	// Score based on: citations (10 pts each) + words (0.1 pts each, max 50)
	citationScore := len(resp.Citations) * 10
	wordScore := min(resp.WordCount/10, 50)
	return citationScore + wordScore
}

func invokeModel(ctx context.Context, modelName, query string) ModelResponse {
	switch modelName {
	case "nova":
		return invokeNova(ctx, query)
	case "claude":
		return invokeClaude(ctx, query)
	case "gemini":
		return invokeGemini(ctx, query)
	case "grok":
		return invokeGrok(ctx, query)
	default:
		return ModelResponse{ModelName: modelName, Error: fmt.Errorf("unknown model")}
	}
}

func getModelDisplayName(modelName string) string {
	switch modelName {
	case "nova":
		return "Nova Premier (AWS)"
	case "claude":
		return "Claude 4.5 Sonnet"
	case "gemini":
		return "Gemini 3 Pro"
	case "grok":
		return "Grok 4 (xAI)"
	default:
		return modelName
	}
}

func getModelEmoji(modelName string) string {
	switch modelName {
	case "nova":
		return "🟠"
	case "claude":
		return "🟣"
	case "gemini":
		return "🔵"
	case "grok":
		return "⚫"
	default:
		return "⚪"
	}
}

// ============================================================================
// NOVA (with Web Grounding via Bedrock)
// ============================================================================

func invokeNova(ctx context.Context, query string) ModelResponse {
	start := time.Now()
	result := ModelResponse{ModelName: "nova"}

	client, err := createBedrockClient(ctx)
	if err != nil {
		result.Error = err
		return result
	}

	userMessage := types.Message{
		Role: types.ConversationRoleUser,
		Content: []types.ContentBlock{
			&types.ContentBlockMemberText{Value: query},
		},
	}

	toolConfig := &types.ToolConfiguration{
		Tools: []types.Tool{
			&types.ToolMemberSystemTool{
				Value: types.SystemTool{
					Name: aws.String(novaGroundingTool),
				},
			},
		},
	}

	input := &bedrockruntime.ConverseInput{
		ModelId:    aws.String(novaModelID),
		Messages:   []types.Message{userMessage},
		ToolConfig: toolConfig,
	}

	if verbose {
		fmt.Printf("  [Nova] Sending request with web grounding...\n")
	}

	output, err := client.Converse(ctx, input)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = fmt.Errorf("API error: %w", err)
		return result
	}

	parseBedrockResponse(output, &result)
	return result
}

// ============================================================================
// CLAUDE (via Anthropic API with Web Search tool)
// ============================================================================

func invokeClaude(ctx context.Context, query string) ModelResponse {
	start := time.Now()
	result := ModelResponse{ModelName: "claude"}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		result.Error = fmt.Errorf("ANTHROPIC_API_KEY not set")
		return result
	}

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

	parseClaudeResponse(message, &result)
	return result
}

func parseClaudeResponse(message *anthropic.Message, result *ModelResponse) {
	var textBuilder strings.Builder
	seenCitations := make(map[string]bool)

	for _, block := range message.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			textBuilder.WriteString(b.Text)
			for _, citation := range b.Citations {
				if citation.Type == "web_search_result_location" && citation.URL != "" {
					if !seenCitations[citation.URL] {
						seenCitations[citation.URL] = true
						result.Citations = append(result.Citations, Citation{
							URL:   citation.URL,
							Title: citation.Title,
						})
					}
				}
			}
		case anthropic.ToolUseBlock:
			if verbose {
				fmt.Printf("  [Claude] Used tool: %s\n", b.Name)
			}
		}
	}

	result.Text = textBuilder.String()
}

// ============================================================================
// GEMINI (via Google AI API with Google Search grounding)
// ============================================================================

func invokeGemini(ctx context.Context, query string) ModelResponse {
	start := time.Now()
	result := ModelResponse{ModelName: "gemini"}

	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		result.Error = fmt.Errorf("GOOGLE_API_KEY not set")
		return result
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

	parseGeminiResponse(resp, &result)
	return result
}

func parseGeminiResponse(resp *genai.GenerateContentResponse, result *ModelResponse) {
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
		seenCitations := make(map[string]bool)
		for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
			if chunk.Web != nil {
				url := chunk.Web.URI
				if url != "" && !seenCitations[url] {
					seenCitations[url] = true
					result.Citations = append(result.Citations, Citation{
						URL:   url,
						Title: chunk.Web.Title,
					})
				}
			}
		}
	}
}

// ============================================================================
// GROK (via xAI API with web search)
// ============================================================================

type GrokRequest struct {
	Model string        `json:"model"`
	Input []GrokMessage `json:"input"`
	Tools []GrokTool    `json:"tools,omitempty"`
}

type GrokMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GrokTool struct {
	Type string `json:"type"`
}

type GrokResponse struct {
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
}

func invokeGrok(ctx context.Context, query string) ModelResponse {
	start := time.Now()
	result := ModelResponse{ModelName: "grok"}

	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		result.Error = fmt.Errorf("XAI_API_KEY not set")
		return result
	}

	if verbose {
		fmt.Printf("  [Grok] Sending request with web search...\n")
	}

	reqBody := GrokRequest{
		Model: grokModelID,
		Input: []GrokMessage{
			{Role: "user", Content: query},
		},
		Tools: []GrokTool{
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

	var grokResp GrokResponse
	if err := json.Unmarshal(body, &grokResp); err != nil {
		result.Error = fmt.Errorf("parse error: %w", err)
		return result
	}

	parseGrokResponse(&grokResp, &result)
	return result
}

func parseGrokResponse(resp *GrokResponse, result *ModelResponse) {
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

	// Extract citations from markdown links in text [[n]](url) pattern
	seenCitations := make(map[string]bool)
	linkRegex := regexp.MustCompile(`\[\[(\d+)\]\]\((https?://[^\)]+)\)`)
	matches := linkRegex.FindAllStringSubmatch(result.Text, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			url := match[2]
			if !seenCitations[url] {
				seenCitations[url] = true
				result.Citations = append(result.Citations, Citation{
					URL: url,
				})
			}
		}
	}

	// Also extract from web_search_call action sources
	for _, out := range resp.Output {
		if out.Type == "web_search_call" && out.Action.Type == "search" {
			for _, src := range out.Action.Sources {
				if src.URL != "" && !seenCitations[src.URL] {
					seenCitations[src.URL] = true
					result.Citations = append(result.Citations, Citation{
						URL:   src.URL,
						Title: src.Title,
					})
				}
			}
		}
	}
}

// ============================================================================
// SHARED HELPERS
// ============================================================================

func createBedrockClient(ctx context.Context) (*bedrockruntime.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg, func(o *bedrockruntime.Options) {
		o.HTTPClient = &httpClientWithTimeout{timeout: 5 * time.Minute}
	})

	return client, nil
}

type httpClientWithTimeout struct {
	timeout time.Duration
}

func (c *httpClientWithTimeout) Do(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: c.timeout}
	return client.Do(req)
}

func parseBedrockResponse(output *bedrockruntime.ConverseOutput, result *ModelResponse) {
	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		result.Error = fmt.Errorf("unexpected output type")
		return
	}

	var textBuilder strings.Builder
	seenCitations := make(map[string]bool)

	for _, block := range msg.Value.Content {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			textBuilder.WriteString(b.Value)

		case *types.ContentBlockMemberCitationsContent:
			for _, content := range b.Value.Content {
				if textContent, ok := content.(*types.CitationGeneratedContentMemberText); ok {
					textBuilder.WriteString(textContent.Value)
				}
			}

			for _, citation := range b.Value.Citations {
				if citation.Location != nil {
					if webLoc, ok := citation.Location.(*types.CitationLocationMemberWeb); ok {
						url := ""
						domain := ""
						if webLoc.Value.Url != nil {
							url = *webLoc.Value.Url
						}
						if webLoc.Value.Domain != nil {
							domain = *webLoc.Value.Domain
						}
						if url != "" && !seenCitations[url] {
							seenCitations[url] = true
							result.Citations = append(result.Citations, Citation{
								URL:    url,
								Domain: domain,
							})
						}
					}
				}
			}
		}
	}

	result.Text = textBuilder.String()
}

func stripThinkingTags(text string) string {
	re := regexp.MustCompile(`<thinking>.*?</thinking>\s*`)
	return strings.TrimSpace(re.ReplaceAllString(text, ""))
}

func printModelResponse(resp ModelResponse) {
	printModelResponseWithRank(resp, 0)
}

func printModelResponseWithRank(resp ModelResponse, rank int) {
	emoji := getModelEmoji(resp.ModelName)
	name := getModelDisplayName(resp.ModelName)

	// Build header
	header := fmt.Sprintf("%s %s", emoji, name)
	if rank > 0 {
		medals := []string{"🥇", "🥈", "🥉", "4th"}
		medal := medals[min(rank-1, 3)]
		header = fmt.Sprintf("%s #%d %s", medal, rank, header)
	}
	if resp.Duration > 0 {
		header += fmt.Sprintf(" (%v)", resp.Duration.Round(time.Millisecond))
	}

	fmt.Printf("┌─ %s\n", header)

	if resp.Error != nil {
		fmt.Printf("│ ❌ Error: %v\n", resp.Error)
		fmt.Println("└" + strings.Repeat("─", 60))
		return
	}

	// Stats line
	fmt.Printf("│ 📊 %d words | %d citations | score: %d\n", resp.WordCount, len(resp.Citations), resp.Score)
	fmt.Println("│")

	// Print response text
	text := resp.Text
	if !showThinking {
		text = stripThinkingTags(text)
	}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		fmt.Printf("│ %s\n", line)
	}

	// Print citations if any
	if len(resp.Citations) > 0 {
		fmt.Println("│")
		fmt.Println("│ 📎 Sources:")
		for i, citation := range resp.Citations {
			if citation.Title != "" {
				fmt.Printf("│   [%d] %s\n", i+1, citation.Title)
				fmt.Printf("│       %s\n", citation.URL)
			} else {
				fmt.Printf("│   [%d] %s\n", i+1, citation.URL)
			}
		}
	}

	fmt.Println("└" + strings.Repeat("─", 60))
}

func printComparisonSummary(responses []ModelResponse) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                        RANKING & SCORES                              ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════════╣")

	for i, resp := range responses {
		emoji := getModelEmoji(resp.ModelName)
		name := getModelDisplayName(resp.ModelName)
		status := "✅"
		if resp.Error != nil {
			status = "❌"
		}

		medals := []string{"🥇", "🥈", "🥉", "  "}
		medal := medals[min(i, 3)]

		fmt.Printf("║ %s %s %-22s %s │ %4d words │ %2d cites │ score: %3d ║\n",
			medal, emoji, name, status, resp.WordCount, len(resp.Citations), resp.Score)
	}

	fmt.Println("╠══════════════════════════════════════════════════════════════════════╣")

	// Find winner
	if len(responses) > 0 && responses[0].Error == nil {
		winner := getModelDisplayName(responses[0].ModelName)
		fmt.Printf("║ 🏆 WINNER: %-58s ║\n", winner)
	}

	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printCombinedSummary(responses []ModelResponse, query string) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     COMBINED INTELLIGENCE                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Collect all unique citations
	allCitations := make(map[string]Citation)
	for _, resp := range responses {
		for _, c := range resp.Citations {
			if c.URL != "" {
				allCitations[c.URL] = c
			}
		}
	}

	// Show which models found what
	fmt.Println("📊 Coverage Analysis:")
	fmt.Println(strings.Repeat("─", 70))

	for _, resp := range responses {
		if resp.Error != nil {
			continue
		}
		emoji := getModelEmoji(resp.ModelName)
		name := getModelDisplayName(resp.ModelName)

		// Extract key points (first 3 bullet points or sentences)
		keyPoints := extractKeyPoints(resp.Text, 3)
		fmt.Printf("\n%s %s found:\n", emoji, name)
		for _, point := range keyPoints {
			fmt.Printf("   • %s\n", point)
		}
	}

	// Show all unique sources
	if len(allCitations) > 0 {
		fmt.Println()
		fmt.Printf("🌐 All Sources (%d unique across all models):\n", len(allCitations))
		fmt.Println(strings.Repeat("─", 70))

		i := 1
		for _, c := range allCitations {
			title := c.Title
			if title == "" {
				title = c.Domain
			}
			if title == "" {
				title = "(no title)"
			}
			fmt.Printf("   [%d] %s\n       %s\n", i, title, c.URL)
			i++
			if i > 10 {
				fmt.Printf("   ... and %d more sources\n", len(allCitations)-10)
				break
			}
		}
	}

	fmt.Println()
}

func extractKeyPoints(text string, maxPoints int) []string {
	// Remove thinking tags
	text = stripThinkingTags(text)

	var points []string

	// Look for bullet points first
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "• ") {
			point := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "), "• ")
			// Truncate long points
			if len(point) > 100 {
				point = point[:97] + "..."
			}
			points = append(points, point)
			if len(points) >= maxPoints {
				break
			}
		}
	}

	// If no bullets found, extract first sentences
	if len(points) == 0 {
		sentences := strings.Split(text, ". ")
		for i, s := range sentences {
			s = strings.TrimSpace(s)
			if len(s) > 20 && len(s) < 150 {
				points = append(points, s)
				if len(points) >= maxPoints {
					break
				}
			}
			if i > 10 {
				break
			}
		}
	}

	return points
}
