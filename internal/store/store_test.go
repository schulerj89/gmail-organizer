package store

import (
	"testing"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

func TestReviewStoreSavesAndAppliesClassifications(t *testing.T) {
	store, err := NewReviewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	email := domain.EmailSummary{
		ID:         "email-1",
		Category:   domain.CategoryPromotions,
		Confidence: 0.88,
		Reason:     "test",
	}
	if err := store.SaveClassifications([]domain.EmailSummary{email}); err != nil {
		t.Fatalf("save: %v", err)
	}
	applied := store.Apply([]domain.EmailSummary{{ID: "email-1"}})
	if got := applied[0].Category; got != domain.CategoryPromotions {
		t.Fatalf("expected promotions, got %s", got)
	}
}

func TestReviewStoreRecordsAudit(t *testing.T) {
	store, err := NewReviewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.RecordAction(domain.ActionTrash, []string{"email-1"}, []domain.ActionResult{{EmailID: "email-1", Status: "trashed"}}); err != nil {
		t.Fatalf("record: %v", err)
	}
	entries, err := store.RecentAudit(10)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != domain.ActionTrash {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}
