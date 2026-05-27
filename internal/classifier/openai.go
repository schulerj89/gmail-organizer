package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
	"github.com/schulerj89/gmail-organizer/internal/secrets"
)

type OpenAIResponsesClassifier struct {
	apiKey secrets.FileSecret
	model  string
	client *http.Client
}

func NewOpenAIResponsesClassifier(apiKey secrets.FileSecret, model string) OpenAIResponsesClassifier {
	return OpenAIResponsesClassifier{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c OpenAIResponsesClassifier) Classify(ctx context.Context, emails []domain.EmailSummary) ([]domain.Classification, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	key, err := c.apiKey.Read()
	if err != nil {
		return nil, err
	}
	input, err := json.Marshal(toPromptEmails(emails))
	if err != nil {
		return nil, err
	}

	requestBody := responsesRequest{
		Model:        c.model,
		Instructions: "Classify each email into one category: needs_review, promotions, newsletters, social, finance, travel, work, receipts, security, personal, unwanted. Return strict JSON only.",
		Input:        "Emails:\n" + string(input),
		Text: responseTextFormat{
			Format: jsonSchemaFormat{
				Type: "json_schema",
				Name: "email_classifications",
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"classifications": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"emailId":    map[string]any{"type": "string"},
									"category":   map[string]any{"type": "string"},
									"confidence": map[string]any{"type": "number"},
									"reason":     map[string]any{"type": "string"},
								},
								"required":             []string{"emailId", "category", "confidence", "reason"},
								"additionalProperties": false,
							},
						},
					},
					"required":             []string{"classifications"},
					"additionalProperties": false,
				},
				Strict: true,
			},
		},
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai classify failed: status %d", resp.StatusCode)
	}

	var parsed responsesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	text := parsed.OutputText()
	var output struct {
		Classifications []domain.Classification `json:"classifications"`
	}
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		return nil, fmt.Errorf("parse openai classification json: %w", err)
	}
	return output.Classifications, nil
}

type promptEmail struct {
	ID             string `json:"id"`
	From           string `json:"from"`
	Subject        string `json:"subject"`
	Snippet        string `json:"snippet"`
	HasUnsubscribe bool   `json:"hasUnsubscribe"`
}

func toPromptEmails(emails []domain.EmailSummary) []promptEmail {
	out := make([]promptEmail, 0, len(emails))
	for _, email := range emails {
		out = append(out, promptEmail{
			ID:             email.ID,
			From:           truncate(email.From, 180),
			Subject:        truncate(email.Subject, 240),
			Snippet:        truncate(email.Snippet, 420),
			HasUnsubscribe: email.HasUnsubscribe,
		})
	}
	return out
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

type responsesRequest struct {
	Model        string             `json:"model"`
	Instructions string             `json:"instructions"`
	Input        string             `json:"input"`
	Text         responseTextFormat `json:"text"`
}

type responseTextFormat struct {
	Format jsonSchemaFormat `json:"format"`
}

type jsonSchemaFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

type responsesResponse struct {
	Output []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func (r responsesResponse) OutputText() string {
	var builder strings.Builder
	for _, output := range r.Output {
		for _, content := range output.Content {
			if content.Text != "" {
				builder.WriteString(content.Text)
			}
		}
	}
	return builder.String()
}
