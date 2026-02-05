package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

const (
	novaModelID       = "us.amazon.nova-premier-v1:0"
	novaGroundingTool = "nova_grounding"
)

func init() {
	Register(&NovaProvider{})
}

// NovaProvider implements Provider for Amazon Nova Premier via AWS Bedrock.
type NovaProvider struct{}

func (p *NovaProvider) Name() string        { return "nova" }
func (p *NovaProvider) DisplayName() string { return "Nova Premier (AWS)" }
func (p *NovaProvider) Emoji() string       { return "🟠" }

func (p *NovaProvider) CheckAuth() error {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return fmt.Errorf("AWS credentials not configured")
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil || creds.AccessKeyID == "" {
		return fmt.Errorf("AWS credentials not found")
	}
	return nil
}

func (p *NovaProvider) Query(ctx context.Context, query string, verbose bool) Result {
	start := time.Now()
	result := Result{}

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

	// Extract token usage
	if output.Usage != nil {
		result.Tokens.Input = int(aws.ToInt32(output.Usage.InputTokens))
		result.Tokens.Output = int(aws.ToInt32(output.Usage.OutputTokens))
	}

	parseBedrockResponse(output, &result)
	return result
}

// --- Helpers ---

type httpClientWithTimeout struct {
	timeout time.Duration
}

func (c *httpClientWithTimeout) Do(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: c.timeout}
	return client.Do(req)
}

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

func parseBedrockResponse(output *bedrockruntime.ConverseOutput, result *Result) {
	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		result.Error = fmt.Errorf("unexpected output type")
		return
	}

	var text string
	seen := make(map[string]bool)

	for _, block := range msg.Value.Content {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			text += b.Value

		case *types.ContentBlockMemberCitationsContent:
			for _, content := range b.Value.Content {
				if textContent, ok := content.(*types.CitationGeneratedContentMemberText); ok {
					text += textContent.Value
				}
			}

			for _, citation := range b.Value.Citations {
				if citation.Location != nil {
					if webLoc, ok := citation.Location.(*types.CitationLocationMemberWeb); ok {
						url := aws.ToString(webLoc.Value.Url)
						domain := aws.ToString(webLoc.Value.Domain)
						DeduplicateCitations(&result.Citations, seen, Citation{
							URL:    url,
							Domain: domain,
						})
					}
				}
			}
		}
	}

	result.Text = text
}
