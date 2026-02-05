package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ModelResult wraps Result with provider info for display.
type ModelResult struct {
	Provider Provider
	Result   Result
	Score    int
}

func printHeader() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    WEB SEARCH CLI                            ║")
	fmt.Println("║     Compare AI models with real-time web search              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printSkippedProviders(skipped []string) {
	if len(skipped) == 0 {
		return
	}
	fmt.Println("⏭️  Skipping providers (missing credentials):")
	for _, msg := range skipped {
		fmt.Printf("   %s\n", msg)
	}
	fmt.Println()
}

func printModelResult(mr ModelResult) {
	printModelResultWithRank(mr, 0)
}

func printModelResultWithRank(mr ModelResult, rank int) {
	p := mr.Provider
	r := mr.Result

	// Build header
	header := fmt.Sprintf("%s %s", p.Emoji(), p.DisplayName())
	if rank > 0 {
		medals := []string{"🥇", "🥈", "🥉", "  "}
		medal := medals[min(rank-1, 3)]
		header = fmt.Sprintf("%s #%d %s", medal, rank, header)
	}
	if r.Duration > 0 {
		header += fmt.Sprintf(" (%v)", r.Duration.Round(time.Millisecond))
	}

	fmt.Printf("┌─ %s\n", header)

	if r.Error != nil {
		fmt.Printf("│ ❌ Error: %v\n", r.Error)
		fmt.Println("└" + strings.Repeat("─", 60))
		return
	}

	// Stats line with cost
	wordCount := len(strings.Fields(r.Text))
	cost := r.Cost(p.Name())
	fmt.Printf("│ 📊 %d words | %d citations | score: %d\n", wordCount, len(r.Citations), mr.Score)
	if r.Tokens.Input > 0 || r.Tokens.Output > 0 {
		fmt.Printf("│ 💰 $%.4f (%d in / %d out tokens)\n", cost, r.Tokens.Input, r.Tokens.Output)
	}
	fmt.Println("│")

	// Print response text
	text := r.Text
	if !showThinking {
		text = stripThinkingTags(text)
	}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		fmt.Printf("│ %s\n", line)
	}

	// Print citations if any
	if len(r.Citations) > 0 {
		fmt.Println("│")
		fmt.Println("│ 📎 Sources:")
		for i, citation := range r.Citations {
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

func printComparisonSummary(results []ModelResult) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                        RANKING & PERFORMANCE                         ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════════╣")

	var totalCost float64
	for i, mr := range results {
		p := mr.Provider
		r := mr.Result

		status := "✅"
		if r.Error != nil {
			status = "❌"
		}

		medals := []string{"🥇", "🥈", "🥉", "  "}
		medal := medals[min(i, 3)]

		wordCount := len(strings.Fields(r.Text))
		cost := r.Cost(p.Name())
		totalCost += cost

		fmt.Printf("║ %s %s %-22s %s │ %4d words │ %2d cites │ $%.4f   ║\n",
			medal, p.Emoji(), p.DisplayName(), status, wordCount, len(r.Citations), cost)
	}

	fmt.Println("╠══════════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║ 💰 TOTAL COST: $%.4f                                                ║\n", totalCost)

	// Find winner
	if len(results) > 0 && results[0].Result.Error == nil {
		winner := results[0].Provider.DisplayName()
		fmt.Printf("║ 🏆 WINNER: %-58s ║\n", winner)
	}

	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printCombinedSummary(results []ModelResult, query string) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     COMBINED INTELLIGENCE                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Collect all unique citations
	allCitations := make(map[string]Citation)
	for _, mr := range results {
		for _, c := range mr.Result.Citations {
			if c.URL != "" {
				allCitations[c.URL] = c
			}
		}
	}

	// Show which models found what
	fmt.Println("📊 Coverage Analysis:")
	fmt.Println(strings.Repeat("─", 70))

	for _, mr := range results {
		if mr.Result.Error != nil {
			continue
		}
		p := mr.Provider

		// Extract key points
		keyPoints := extractKeyPoints(mr.Result.Text, 3)
		fmt.Printf("\n%s %s found:\n", p.Emoji(), p.DisplayName())
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

func stripThinkingTags(text string) string {
	re := regexp.MustCompile(`(?s)<thinking>.*?</thinking>\s*`)
	return strings.TrimSpace(re.ReplaceAllString(text, ""))
}

func calculateScore(r Result) int {
	if r.Error != nil {
		return 0
	}
	wordCount := len(strings.Fields(r.Text))
	citationScore := len(r.Citations) * 10
	wordScore := min(wordCount/10, 50)
	return citationScore + wordScore
}
