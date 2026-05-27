package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
