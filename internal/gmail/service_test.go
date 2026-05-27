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
