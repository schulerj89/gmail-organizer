package gmail

import (
	"context"
	"testing"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

func TestFirstUnsubscribeTargetPrefersHTTPS(t *testing.T) {
	got := firstUnsubscribeTarget("<https://example.com/u>, <mailto:u@example.com>")
	if got != "https://example.com/u" {
		t.Fatalf("unexpected target %q", got)
	}
}

func TestFirstUnsubscribeTargetRejectsUnsafeScheme(t *testing.T) {
	got := firstUnsubscribeTarget("<javascript:alert(1)>")
	if got != "" {
		t.Fatalf("expected empty target, got %q", got)
	}
}

func TestUnsubscribeCapabilitiesAllowOneClickHTTPS(t *testing.T) {
	method, canAuto := unsubscribeCapabilities("https://example.com/unsubscribe", "List-Unsubscribe=One-Click")
	if method != "one_click_post" || !canAuto {
		t.Fatalf("expected one click support, got method=%s auto=%v", method, canAuto)
	}
}

func TestUnsubscribeCapabilitiesRejectLocalhostOneClick(t *testing.T) {
	method, canAuto := unsubscribeCapabilities("https://localhost/unsubscribe", "List-Unsubscribe=One-Click")
	if method != "https_review" || canAuto {
		t.Fatalf("expected review-only localhost target, got method=%s auto=%v", method, canAuto)
	}
}

func TestUnsubscribeResultsPrepareReviewLinks(t *testing.T) {
	results := UnsubscribeResults(context.Background(), []domain.EmailSummary{{
		ID:                "1",
		HasUnsubscribe:    true,
		UnsubscribeTarget: "mailto:unsubscribe@example.com",
		UnsubscribeMethod: "mailto",
	}}, []string{"1"})
	if len(results) != 1 || results[0].Status != "prepared" || results[0].SafeLink == "" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestPreviewUnsubscribeRequiresConfirmationForOneClick(t *testing.T) {
	results := PreviewUnsubscribeResults([]domain.EmailSummary{{
		ID:                 "1",
		HasUnsubscribe:     true,
		UnsubscribeTarget:  "https://example.com/unsubscribe",
		UnsubscribeMethod:  "one_click_post",
		CanAutoUnsubscribe: true,
	}}, []string{"1"})
	if len(results) != 1 || results[0].Status != "needs_confirmation" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestPreviewUnsubscribePreparesReviewLinks(t *testing.T) {
	results := PreviewUnsubscribeResults([]domain.EmailSummary{{
		ID:                "1",
		HasUnsubscribe:    true,
		UnsubscribeTarget: "mailto:unsubscribe@example.com",
		UnsubscribeMethod: "mailto",
	}}, []string{"1"})
	if len(results) != 1 || results[0].Status != "prepared" || results[0].SafeLink == "" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestChunkIDsSkipsBlanksAndChunks(t *testing.T) {
	chunks := chunkIDs([]string{"a", "", " b ", "c", "d"}, 2)
	if len(chunks) != 2 {
		t.Fatalf("expected two chunks, got %#v", chunks)
	}
	if chunks[0][0] != "a" || chunks[0][1] != "b" || chunks[1][0] != "c" || chunks[1][1] != "d" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestActionResultsForIDs(t *testing.T) {
	results := actionResultsForIDs([]string{"a", "b"}, "marked_read", "")
	if len(results) != 2 || results[0].EmailID != "a" || results[0].Status != "marked_read" || results[0].Message != "" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
