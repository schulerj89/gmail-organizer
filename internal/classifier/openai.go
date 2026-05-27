package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
	"github.com/schulerj89/gmail-organizer/internal/secrets"
)

type OpenAIResponsesClassifier struct {
	apiKey          secrets.FileSecret
	model           string
	maxOutputTokens int
	maxRetries      int
	requestDelay    time.Duration
	client          *http.Client
	mu              sync.Mutex
	lastRequestAt   time.Time
}

type OpenAIOptions struct {
	MaxOutputTokens int
	MaxRetries      int
	RequestDelay    time.Duration
	Timeout         time.Duration
}

func NewOpenAIResponsesClassifier(apiKey secrets.FileSecret, model string, options OpenAIOptions) *OpenAIResponsesClassifier {
	if options.MaxOutputTokens <= 0 {
		options.MaxOutputTokens = 2000
	}
	if options.MaxRetries < 0 {
		options.MaxRetries = 0
	}
	if options.Timeout <= 0 {
		options.Timeout = 45 * time.Second
	}
	return &OpenAIResponsesClassifier{
		apiKey:          apiKey,
		model:           model,
		maxOutputTokens: options.MaxOutputTokens,
		maxRetries:      options.MaxRetries,
		requestDelay:    options.RequestDelay,
		client:          &http.Client{Timeout: options.Timeout},
	}
}

func (c *OpenAIResponsesClassifier) Classify(ctx context.Context, emails []domain.EmailSummary) ([]domain.Classification, error) {
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
		Model:           c.model,
		MaxOutputTokens: c.maxOutputTokens,
		Instructions:    "Classify each email into one category: needs_review, promotions, newsletters, social, finance, travel, work, receipts, security, personal, unwanted. Return strict JSON only.",
		Input:           "Emails:\n" + string(input),
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

	body, err := c.doRequest(ctx, key, payload)
	if err != nil {
		return nil, err
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

func (c *OpenAIResponsesClassifier) doRequest(ctx context.Context, key string, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := c.waitForPace(ctx); err != nil {
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
			lastErr = err
			if attempt == c.maxRetries {
				break
			}
			if err := sleepContext(ctx, retryDelay(attempt, nil)); err != nil {
				return nil, err
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < 300 {
			return body, nil
		}
		lastErr = openAIHTTPError(resp, body)
		if !retryableStatus(resp.StatusCode) || attempt == c.maxRetries {
			break
		}
		if err := sleepContext(ctx, retryDelay(attempt, resp.Header)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *OpenAIResponsesClassifier) waitForPace(ctx context.Context) error {
	if c.requestDelay <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	wait := time.Until(c.lastRequestAt.Add(c.requestDelay))
	if wait > 0 {
		if err := sleepContext(ctx, wait); err != nil {
			return err
		}
	}
	c.lastRequestAt = time.Now()
	return nil
}

func openAIHTTPError(resp *http.Response, body []byte) error {
	requestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	if requestID != "" {
		return fmt.Errorf("openai classify failed: status %d request_id %s", resp.StatusCode, requestID)
	}
	return fmt.Errorf("openai classify failed: status %d: %s", resp.StatusCode, truncate(string(body), 300))
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

func retryDelay(attempt int, header http.Header) time.Duration {
	if header != nil {
		if parsed := parseRetryAfter(header.Get("Retry-After")); parsed > 0 {
			return parsed
		}
		if parsed := parseRateLimitReset(header); parsed > 0 {
			return parsed
		}
	}
	delay := time.Duration(1<<attempt) * time.Second
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return time.Until(at)
	}
	return 0
}

func parseRateLimitReset(header http.Header) time.Duration {
	var longest time.Duration
	for _, key := range []string{"x-ratelimit-reset-requests", "x-ratelimit-reset-tokens"} {
		if parsed := parseResetDuration(header.Get(key)); parsed > longest {
			longest = parsed
		}
	}
	if longest > 30*time.Second {
		return 30 * time.Second
	}
	return longest
}

func parseResetDuration(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	if strings.HasSuffix(value, "ms") {
		ms, err := strconv.Atoi(strings.TrimSuffix(value, "ms"))
		if err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 0
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	Model           string             `json:"model"`
	MaxOutputTokens int                `json:"max_output_tokens,omitempty"`
	Instructions    string             `json:"instructions"`
	Input           string             `json:"input"`
	Text            responseTextFormat `json:"text"`
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
