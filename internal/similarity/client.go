package similarity

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/storacha/project-agent/internal/github"
	"google.golang.org/api/option"
)

// Client handles semantic similarity detection using Gemini
type Client struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

// NewClient creates a new Gemini client for similarity detection
func NewClient(apiKey string) (*Client, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	model := client.GenerativeModel("gemini-3-flash-preview")

	// Configure model for structured output
	model.SetTemperature(0.1) // Low temperature for consistent results
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(`You are an expert at analyzing GitHub issues and determining if they are duplicates or highly similar.
When comparing two issues, consider:
1. The core problem or feature being described
2. The technical concepts involved
3. The user's goal or desired outcome
4. Similar error messages or symptoms

Respond with a JSON object containing:
- "similar": boolean indicating if the issues are duplicates or highly similar (>85% similar)
- "similarity": float between 0.0 and 1.0 indicating similarity score
- "reasoning": brief explanation of why they are or aren't similar

Be strict - only mark as similar if they're truly about the same issue or feature.`),
		},
	}

	return &Client{
		client: client,
		model:  model,
	}, nil
}

// CompareSimilarity compares two issues and returns a similarity score
func (c *Client) CompareSimilarity(ctx context.Context, issue1, issue2 github.Issue) (float64, error) {
	prompt := fmt.Sprintf(`Compare these two GitHub issues and determine if they are duplicates or highly similar:

Issue #%d: %s
%s

Issue #%d: %s
%s

Are these issues duplicates or highly similar? Respond in JSON format.`,
		issue1.Number, issue1.Title, truncateBody(issue1.Body),
		issue2.Number, issue2.Title, truncateBody(issue2.Body))

	resp, err := c.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return 0, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return 0, fmt.Errorf("no response from Gemini")
	}

	// Parse the response
	responseText := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])

	// Extract similarity score from response
	// The model should return a JSON, but we'll parse it robustly
	similarity := parseSimilarityFromResponse(responseText)

	return similarity, nil
}

// truncateBody limits issue body to first 500 characters to save on API costs
func truncateBody(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 500 {
		return body[:500] + "..."
	}
	return body
}

// parseSimilarityFromResponse extracts similarity score from Gemini response
func parseSimilarityFromResponse(response string) float64 {
	// Look for "similarity": number pattern
	// This is a simple parser - in production you might want to use proper JSON parsing
	response = strings.ToLower(response)

	// Try to find the similarity value
	if strings.Contains(response, `"similar": true`) || strings.Contains(response, `"similar":true`) {
		// Look for similarity score
		if strings.Contains(response, "0.9") || strings.Contains(response, "0.95") || strings.Contains(response, "1.0") {
			return 0.9
		}
		if strings.Contains(response, "0.85") || strings.Contains(response, "0.8") {
			return 0.85
		}
		return 0.9 // Default high score if marked as similar
	}

	// If not similar, return low score
	return 0.0
}

// Close closes the Gemini client
func (c *Client) Close() error {
	return c.client.Close()
}
