package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

type fakeClassifier struct {
	calls int
	fail  bool
}

func (f *fakeClassifier) Classify(_ context.Context, emails []domain.EmailSummary) ([]domain.Classification, error) {
	f.calls++
	if f.fail {
		return nil, context.Canceled
	}
	out := make([]domain.Classification, 0, len(emails))
	for _, email := range emails {
		out = append(out, domain.Classification{
			EmailID:    email.ID,
			Category:   domain.CategoryUnwanted,
			Confidence: 0.91,
			Reason:     "ai",
		})
	}
	return out, nil
}

func TestOverlayAIClassificationsChunks(t *testing.T) {
	emails := make([]domain.EmailSummary, 0, 5)
	fallback := make([]domain.Classification, 0, 5)
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		emails = append(emails, domain.EmailSummary{ID: id})
		fallback = append(fallback, domain.Classification{EmailID: id, Category: domain.CategoryNeedsReview})
	}
	fake := &fakeClassifier{}
	got := overlayAIClassifications(context.Background(), fallback, emails, fake, 2)
	if fake.calls != 3 {
		t.Fatalf("expected 3 chunks, got %d", fake.calls)
	}
	for _, item := range got {
		if item.Category != domain.CategoryUnwanted {
			t.Fatalf("expected ai category, got %s", item.Category)
		}
	}
}

func TestOverlayAIClassificationsKeepsFallbackOnFailure(t *testing.T) {
	emails := []domain.EmailSummary{{ID: "a"}}
	fallback := []domain.Classification{{EmailID: "a", Category: domain.CategoryNeedsReview}}
	fake := &fakeClassifier{fail: true}
	got := overlayAIClassifications(context.Background(), fallback, emails, fake, 10)
	if got[0].Category != domain.CategoryNeedsReview {
		t.Fatalf("expected fallback category, got %s", got[0].Category)
	}
}

func TestPreviewActionRequiresTrashConfirmation(t *testing.T) {
	server := &Server{}
	results, err := server.previewAction(domain.ActionTrash, []string{"a", "b"})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !requiresConfirmation(results) {
		t.Fatalf("expected confirmation requirement, got %#v", results)
	}
}

func TestNormalizeIDsDeduplicatesAndSkipsBlanks(t *testing.T) {
	got := normalizeIDs([]string{" a ", "", "a", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected ids: %#v", got)
	}
}

func TestSuccessfulActionIDs(t *testing.T) {
	got := successfulActionIDs([]domain.ActionResult{
		{EmailID: "a", Status: "trashed"},
		{EmailID: "b", Status: "failed"},
		{EmailID: "", Status: "trashed"},
		{EmailID: "c", Status: "trashed"},
	}, "trashed")
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("unexpected ids: %#v", got)
	}
}

func TestSummarizeActionResults(t *testing.T) {
	got := summarizeActionResults([]domain.ActionResult{
		{EmailID: "a", Status: "trashed"},
		{EmailID: "b", Status: "failed"},
		{EmailID: "c", Status: "needs_confirmation"},
		{EmailID: "d", Status: "skipped"},
		{EmailID: "e", Status: "blocked"},
	})
	if got.Total != 5 || got.Succeeded != 1 || got.Failed != 1 || got.Pending != 1 || got.Skipped != 2 {
		t.Fatalf("unexpected summary: %#v", got)
	}
	if got.ByStatus["trashed"] != 1 || got.ByStatus["blocked"] != 1 {
		t.Fatalf("unexpected status counts: %#v", got.ByStatus)
	}
}

func TestForgetRemovesRememberedEmails(t *testing.T) {
	server := &Server{}
	server.remember([]domain.EmailSummary{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	server.forget([]string{"b"})
	got := server.snapshot()
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("unexpected snapshot: %#v", got)
	}
}

func TestConfirmationTokenMatchesActionAndIDsOnce(t *testing.T) {
	server := &Server{}
	token, expiresAt := server.createConfirmation(domain.ActionTrash, []string{"a", "b"})
	if token == "" || expiresAt.IsZero() {
		t.Fatalf("expected token and expiry, got token=%q expiry=%v", token, expiresAt)
	}
	if server.consumeConfirmation(token, domain.ActionUnsubscribe, []string{"a", "b"}) {
		t.Fatal("token should not match a different action")
	}
	token, _ = server.createConfirmation(domain.ActionTrash, []string{"a", "b"})
	if server.consumeConfirmation(token, domain.ActionTrash, []string{"b", "a"}) {
		t.Fatal("token should not match reordered ids")
	}
	token, _ = server.createConfirmation(domain.ActionTrash, []string{"a", "b"})
	if !server.consumeConfirmation(token, domain.ActionTrash, []string{"a", "b"}) {
		t.Fatal("expected matching token to be consumed")
	}
	if server.consumeConfirmation(token, domain.ActionTrash, []string{"a", "b"}) {
		t.Fatal("token should be single-use")
	}
}

func TestConfirmationTokenExpires(t *testing.T) {
	server := &Server{confirmations: map[string]pendingConfirmation{
		"expired": {
			Action:    domain.ActionTrash,
			IDs:       []string{"a"},
			ExpiresAt: time.Now().UTC().Add(-time.Minute),
		},
	}}
	if server.consumeConfirmation("expired", domain.ActionTrash, []string{"a"}) {
		t.Fatal("expired token should not be accepted")
	}
}

func TestSecurityHeadersBlockCrossOriginMutatingRequest(t *testing.T) {
	called := false
	handler := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/actions", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "https://evil.example")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d", resp.Code)
	}
	if called {
		t.Fatal("handler should not have been called")
	}
}

func TestSecurityHeadersAllowLoopbackMutatingRequest(t *testing.T) {
	handler := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/actions", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "http://localhost:8787")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected no content, got %d", resp.Code)
	}
}

func TestSecurityHeadersAllowOriginlessMutatingRequest(t *testing.T) {
	handler := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/actions", nil)
	req.Host = "127.0.0.1:8787"
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected no content, got %d", resp.Code)
	}
}
