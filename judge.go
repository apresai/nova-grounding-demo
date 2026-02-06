package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const judgeModelID = "claude-haiku-4-5-20251001"

// CitationCheck holds the result of an HTTP HEAD validation for a citation URL.
type CitationCheck struct {
	URL        string
	StatusCode int
	Healthy    bool
	Latency    time.Duration
	Error      string
}

// validateCitations performs parallel HTTP HEAD requests to check citation URLs.
func validateCitations(citations []Citation) []CitationCheck {
	checks := make([]CitationCheck, len(citations))
	var wg sync.WaitGroup

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // follow redirects
		},
	}

	for i, c := range citations {
		wg.Add(1)
		go func(idx int, citation Citation) {
			defer wg.Done()
			check := CitationCheck{URL: citation.URL}
			start := time.Now()

			resp, err := client.Head(citation.URL)
			check.Latency = time.Since(start)

			if err != nil {
				check.Error = err.Error()
			} else {
				resp.Body.Close()
				check.StatusCode = resp.StatusCode
				check.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 400
			}

			checks[idx] = check
		}(i, c)
	}

	wg.Wait()
	return checks
}

// linkHealthScore computes a 1-10 score from citation check results.
// Returns 5 if there are no citations (neutral).
func linkHealthScore(checks []CitationCheck) int {
	if len(checks) == 0 {
		return 5
	}
	healthy := 0
	for _, c := range checks {
		if c.Healthy {
			healthy++
		}
	}
	pct := float64(healthy) / float64(len(checks))
	score := int(pct*9) + 1 // 1-10 scale
	if score > 10 {
		score = 10
	}
	return score
}

// judgeEvaluation is the structured response from the LLM judge per model.
type judgeEvaluation struct {
	Model        string `json:"model"`
	Quality      int    `json:"quality"`
	Recency      int    `json:"recency"`
	Significance int    `json:"significance"`
	Impact       int    `json:"impact"`
	Reasoning    string `json:"reasoning"`
}

// judgeToolResponse is the structured tool_use response.
type judgeToolResponse struct {
	Evaluations []judgeEvaluation `json:"evaluations"`
}

// buildJudgePrompt constructs the prompt for the LLM judge.
func buildJudgePrompt(results []ModelResult, query string, allChecks map[string][]CitationCheck) string {
	var b strings.Builder

	b.WriteString("You are a news editor evaluating web search results from multiple AI models.\n\n")
	b.WriteString(fmt.Sprintf("QUERY: %q\n\n", query))
	b.WriteString("For EACH model below, score these dimensions from 1-10:\n")
	b.WriteString("- quality: depth, coherence, factual accuracy of the response\n")
	b.WriteString("- recency: how current the information and cited sources are (today > this week > this month > older)\n")
	b.WriteString("- significance: is this newsworthy and substantial? Would it make WSJ or major outlets?\n")
	b.WriteString("- impact: how impactful is this to the relevant business, industry, or topic?\n\n")
	b.WriteString("I have already validated citation links. Link health scores are provided.\n\n")

	for _, mr := range results {
		if mr.Result.Error != nil {
			continue
		}
		p := mr.Provider
		r := mr.Result

		wordCount := len(strings.Fields(r.Text))
		checks := allChecks[p.Name()]
		healthyCount := 0
		for _, c := range checks {
			if c.Healthy {
				healthyCount++
			}
		}
		lhScore := linkHealthScore(checks)

		b.WriteString(fmt.Sprintf("=== MODEL: %s ===\n", p.DisplayName()))

		// Truncate text to ~500 words
		text := r.Text
		words := strings.Fields(text)
		if len(words) > 500 {
			text = strings.Join(words[:500], " ") + "..."
		}
		b.WriteString(fmt.Sprintf("Response (%d words, %d citations):\n", wordCount, len(r.Citations)))
		b.WriteString(text)
		b.WriteString("\n\n")

		b.WriteString(fmt.Sprintf("Citations (%d/%d links working):\n", healthyCount, len(r.Citations)))
		for i, c := range r.Citations {
			status := "unknown"
			if i < len(checks) {
				if checks[i].Healthy {
					status = fmt.Sprintf("%d OK", checks[i].StatusCode)
				} else if checks[i].Error != "" {
					status = "error"
				} else {
					status = fmt.Sprintf("%d", checks[i].StatusCode)
				}
			}
			b.WriteString(fmt.Sprintf("  %d. %s - %s\n", i+1, c.URL, status))
		}
		b.WriteString(fmt.Sprintf("Link Health Score: %d/10\n", lhScore))
		b.WriteString("===\n\n")
	}

	b.WriteString("Return your evaluation using the score_models tool. Provide one evaluation per model, in the same order presented above.\n")

	return b.String()
}

// Judge evaluates all model results using link validation and an LLM judge.
func Judge(ctx context.Context, results []ModelResult, query string, verbose bool) ([]ModelResult, error) {
	// Phase 1: Validate all citations in parallel
	if verbose {
		fmt.Println("  [Judge] Validating citation links...")
	}

	allChecks := make(map[string][]CitationCheck)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, mr := range results {
		if mr.Result.Error != nil {
			continue
		}
		wg.Add(1)
		go func(mr ModelResult) {
			defer wg.Done()
			checks := validateCitations(mr.Result.Citations)
			mu.Lock()
			allChecks[mr.Provider.Name()] = checks
			mu.Unlock()
		}(mr)
	}
	wg.Wait()

	if verbose {
		for name, checks := range allChecks {
			healthy := 0
			for _, c := range checks {
				if c.Healthy {
					healthy++
				}
			}
			fmt.Printf("  [Judge] %s: %d/%d links healthy\n", name, healthy, len(checks))
		}
	}

	// Count valid (non-error) results
	validCount := 0
	for _, mr := range results {
		if mr.Result.Error == nil {
			validCount++
		}
	}
	if validCount == 0 {
		return results, nil
	}

	// Phase 2: Call LLM judge
	if verbose {
		fmt.Println("  [Judge] Calling LLM judge (Claude Haiku 4.5)...")
	}

	prompt := buildJudgePrompt(results, query, allChecks)

	client := anthropic.NewClient()

	// Define the scoring tool schema
	evaluationItemSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"model":        map[string]any{"type": "string"},
			"quality":      map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"recency":      map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"significance": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"impact":       map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"reasoning":    map[string]any{"type": "string"},
		},
		"required": []any{"model", "quality", "recency", "significance", "impact", "reasoning"},
	}

	message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     judgeModelID,
		MaxTokens: 2048,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
		ToolChoice: anthropic.ToolChoiceParamOfTool("score_models"),
		Tools: []anthropic.ToolUnionParam{
			{
				OfTool: &anthropic.ToolParam{
					Name:        "score_models",
					Description: anthropic.String("Score each AI model's web search results across quality, recency, significance, and impact dimensions."),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: map[string]any{
							"evaluations": map[string]any{
								"type":  "array",
								"items": evaluationItemSchema,
							},
						},
						Required: []string{"evaluations"},
					},
				},
			},
		},
	})

	if err != nil {
		return results, fmt.Errorf("judge API error: %w", err)
	}

	// Parse the tool_use response
	var toolInput judgeToolResponse
	for _, block := range message.Content {
		if tb := block.AsToolUse(); tb.Name == "score_models" {
			if err := json.Unmarshal(tb.Input, &toolInput); err != nil {
				return results, fmt.Errorf("judge parse error: %w", err)
			}
			break
		}
	}

	if len(toolInput.Evaluations) == 0 {
		return results, fmt.Errorf("judge returned no evaluations")
	}

	if verbose {
		fmt.Printf("  [Judge] Received %d evaluations\n", len(toolInput.Evaluations))
	}

	// Phase 3: Attach scores to results
	// Build a lookup from display name to evaluation
	evalMap := make(map[string]judgeEvaluation)
	for _, eval := range toolInput.Evaluations {
		evalMap[eval.Model] = eval
	}

	for i := range results {
		if results[i].Result.Error != nil {
			continue
		}
		p := results[i].Provider

		// Try matching by display name first, then by provider name
		eval, ok := evalMap[p.DisplayName()]
		if !ok {
			// Try fuzzy matching — the judge may have returned a slightly different name
			for name, e := range evalMap {
				if strings.Contains(strings.ToLower(name), strings.ToLower(p.Name())) ||
					strings.Contains(strings.ToLower(p.DisplayName()), strings.ToLower(name)) {
					eval = e
					ok = true
					break
				}
			}
		}

		lhScore := linkHealthScore(allChecks[p.Name()])

		if ok {
			overall := float64(eval.Quality)*0.25 +
				float64(lhScore)*0.15 +
				float64(eval.Recency)*0.20 +
				float64(eval.Significance)*0.20 +
				float64(eval.Impact)*0.20

			results[i].JudgeScore = &JudgeScore{
				Quality:      eval.Quality,
				LinkHealth:   lhScore,
				Recency:      eval.Recency,
				Significance: eval.Significance,
				Impact:       eval.Impact,
				Overall:      overall,
				Reasoning:    eval.Reasoning,
			}
		} else {
			// Fallback: assign link health score only
			results[i].JudgeScore = &JudgeScore{
				LinkHealth: lhScore,
				Overall:    float64(lhScore),
				Reasoning:  "Judge did not return evaluation for this model",
			}
		}
	}

	// Sort by Overall score descending
	sort.SliceStable(results, func(i, j int) bool {
		si, sj := 0.0, 0.0
		if results[i].JudgeScore != nil {
			si = results[i].JudgeScore.Overall
		}
		if results[j].JudgeScore != nil {
			sj = results[j].JudgeScore.Overall
		}
		return si > sj
	})

	return results, nil
}
