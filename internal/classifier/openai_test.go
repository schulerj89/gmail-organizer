package classifier

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestResponsesRequestIncludesMaxOutputTokens(t *testing.T) {
	payload, err := json.Marshal(responsesRequest{
		Model:           "gpt-5-mini",
		MaxOutputTokens: 1500,
		Input:           "test",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(payload) {
		t.Fatal("payload should be valid json")
	}
	if got := string(payload); !strings.Contains(got, `"max_output_tokens":1500`) {
		t.Fatalf("expected max_output_tokens in payload, got %s", got)
	}
}

func TestRetryDelayUsesRetryAfterHeader(t *testing.T) {
	header := http.Header{}
	header.Set("Retry-After", "2")

	if got := retryDelay(0, header); got != 2*time.Second {
		t.Fatalf("retryDelay() = %v, want 2s", got)
	}
}

func TestRetryDelayUsesRateLimitResetHeaders(t *testing.T) {
	header := http.Header{}
	header.Set("x-ratelimit-reset-requests", "1s")
	header.Set("x-ratelimit-reset-tokens", "3s")

	if got := retryDelay(0, header); got != 3*time.Second {
		t.Fatalf("retryDelay() = %v, want 3s", got)
	}
}

func TestRetryableStatus(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusRequestTimeout, http.StatusInternalServerError} {
		if !retryableStatus(status) {
			t.Fatalf("expected status %d to be retryable", status)
		}
	}
	if retryableStatus(http.StatusBadRequest) {
		t.Fatal("400 should not be retryable")
	}
}
