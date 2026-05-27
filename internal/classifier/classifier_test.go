package classifier

import (
	"context"
	"testing"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

func TestHeuristicClassifierCategorizesPromotions(t *testing.T) {
	c := NewHeuristicClassifier()
	results, err := c.Classify(context.Background(), []domain.EmailSummary{{
		ID:             "1",
		From:           "deals@example.com",
		Subject:        "Big discount today",
		Snippet:        "Use this coupon.",
		HasUnsubscribe: true,
	}})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got := results[0].Category; got != domain.CategoryPromotions {
		t.Fatalf("expected promotions, got %s", got)
	}
}

func TestHeuristicClassifierUsesNeedsReviewFallback(t *testing.T) {
	c := NewHeuristicClassifier()
	results, err := c.Classify(context.Background(), []domain.EmailSummary{{ID: "1", Subject: "Hello"}})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got := results[0].Category; got != domain.CategoryNeedsReview {
		t.Fatalf("expected needs_review, got %s", got)
	}
}

func TestHeuristicClassifierPrioritizesSecurityAlerts(t *testing.T) {
	c := NewHeuristicClassifier()
	results, err := c.Classify(context.Background(), []domain.EmailSummary{{
		ID:      "1",
		From:    "alerts@bank.example",
		Subject: "Security alert for your account",
		Snippet: "A new sign-in was detected.",
	}})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got := results[0].Category; got != domain.CategorySecurity {
		t.Fatalf("expected security, got %s", got)
	}
}
