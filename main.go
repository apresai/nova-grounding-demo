package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// Global flags
var (
	showThinking bool
	verbose      bool
)

func main() {
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
  all      Run all available models in parallel (default)

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

	query := flag.String("q", "", "Question to ask (required)")
	model := flag.String("model", "all", "Model to use: nova, claude, gemini, grok, or all")
	thinking := flag.Bool("thinking", false, "Show model's thinking/reasoning traces")
	verboseFlag := flag.Bool("v", false, "Enable verbose output with timing details")
	flag.Parse()

	showThinking = *thinking || *verboseFlag
	verbose = *verboseFlag

	if *query == "" {
		fmt.Fprintln(os.Stderr, "Error: -q flag is required. Use -h for help.")
		os.Exit(1)
	}

	printHeader()
	fmt.Printf("📝 Query: %s\n\n", *query)

	ctx := context.Background()

	if *model == "all" {
		runAllModels(ctx, *query)
	} else {
		runSingleModel(ctx, *model, *query)
	}
}

func runAllModels(ctx context.Context, query string) {
	// Pre-flight auth check
	var available []Provider
	var skipped []string

	for _, name := range All() {
		p, _ := Get(name)
		if err := p.CheckAuth(); err != nil {
			skipped = append(skipped, fmt.Sprintf("%s %s: %s", p.Emoji(), p.DisplayName(), err.Error()))
		} else {
			available = append(available, p)
		}
	}

	printSkippedProviders(skipped)

	if len(available) == 0 {
		fmt.Println("❌ No providers available. Set at least one API key.")
		os.Exit(1)
	}

	fmt.Printf("🚀 Running query against %d models in parallel...\n", len(available))
	fmt.Println(strings.Repeat("═", 65))
	fmt.Println()

	var wg sync.WaitGroup
	results := make(chan ModelResult, len(available))

	for _, p := range available {
		wg.Add(1)
		go func(provider Provider) {
			defer wg.Done()
			r := provider.Query(ctx, query, verbose)
			results <- ModelResult{
				Provider: provider,
				Result:   r,
				Score:    calculateScore(r),
			}
		}(p)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var modelResults []ModelResult
	for mr := range results {
		modelResults = append(modelResults, mr)
	}

	// Sort by score (highest first)
	sort.Slice(modelResults, func(i, j int) bool {
		return modelResults[i].Score > modelResults[j].Score
	})

	// Print each response
	for i, mr := range modelResults {
		rank := i + 1
		printModelResultWithRank(mr, rank)
		fmt.Println()
	}

	printComparisonSummary(modelResults)
	printCombinedSummary(modelResults, query)
}

func runSingleModel(ctx context.Context, modelName, query string) {
	p, ok := Get(modelName)
	if !ok {
		fmt.Fprintf(os.Stderr, "❌ Unknown model: %s\n", modelName)
		fmt.Printf("Available models: %s\n", strings.Join(All(), ", "))
		os.Exit(1)
	}

	if err := p.CheckAuth(); err != nil {
		fmt.Printf("❌ %s %s: %s\n", p.Emoji(), p.DisplayName(), err.Error())
		os.Exit(1)
	}

	fmt.Printf("🔍 Running with %s...\n", p.DisplayName())
	fmt.Println(strings.Repeat("─", 60))

	r := p.Query(ctx, query, verbose)
	mr := ModelResult{
		Provider: p,
		Result:   r,
		Score:    calculateScore(r),
	}
	printModelResult(mr)
}
