package store

import (
	"testing"
	"time"

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

func TestReviewStoreStats(t *testing.T) {
	store, err := NewReviewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	emails := []domain.EmailSummary{
		{ID: "email-1", Category: domain.CategoryPromotions, Confidence: 0.8, Reason: "test"},
		{ID: "email-2", Category: domain.CategoryNeedsReview, Confidence: 0.4, Reason: "test"},
		{ID: "email-3", Category: domain.CategoryUnwanted, Confidence: 1, Reason: "Manually categorized."},
	}
	if err := store.SaveClassifications(emails); err != nil {
		t.Fatalf("save: %v", err)
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Total != 3 || stats.NeedsReview != 1 || stats.Manual != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if got := stats.ByCategory[domain.CategoryUnwanted]; got != 1 {
		t.Fatalf("expected one unwanted email, got %d", got)
	}
	if stats.UpdatedAt == nil {
		t.Fatal("expected updated timestamp")
	}
}

func TestReviewStoreListsStoredEmailsByCategory(t *testing.T) {
	store, err := NewReviewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().UTC()
	emails := []domain.EmailSummary{
		{
			ID:                 "email-1",
			ThreadID:           "thread-1",
			From:               "Deals <deals@example.com>",
			Subject:            "Sale",
			Snippet:            "Save now.",
			ReceivedAt:         now.Add(-time.Hour),
			Category:           domain.CategoryUnwanted,
			Confidence:         0.9,
			Reason:             "test",
			HasUnsubscribe:     true,
			UnsubscribeTarget:  "mailto:u@example.com",
			UnsubscribeMethod:  "mailto",
			CanAutoUnsubscribe: false,
		},
		{
			ID:         "email-2",
			Subject:    "Receipt",
			ReceivedAt: now,
			Category:   domain.CategoryReceipts,
			Confidence: 0.8,
			Reason:     "test",
		},
	}
	if err := store.SaveClassifications(emails); err != nil {
		t.Fatalf("save: %v", err)
	}
	page, err := store.ListEmails(domain.CategoryUnwanted, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Total != 1 || len(page.Emails) != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	got := page.Emails[0]
	if got.ID != "email-1" || got.Subject != "Sale" || !got.HasUnsubscribe || got.UnsubscribeTarget == "" {
		t.Fatalf("stored metadata was not preserved: %#v", got)
	}
}

func TestReviewStoreListEmailsPaginates(t *testing.T) {
	store, err := NewReviewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now().UTC()
	emails := []domain.EmailSummary{
		{ID: "email-1", ReceivedAt: now.Add(-2 * time.Hour), Category: domain.CategoryUnwanted},
		{ID: "email-2", ReceivedAt: now.Add(-1 * time.Hour), Category: domain.CategoryUnwanted},
		{ID: "email-3", ReceivedAt: now, Category: domain.CategoryUnwanted},
	}
	if err := store.SaveClassifications(emails); err != nil {
		t.Fatalf("save: %v", err)
	}
	page, err := store.ListEmails(domain.CategoryUnwanted, 1, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Total != 3 || len(page.Emails) != 1 || page.Emails[0].ID != "email-2" {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestReviewStoreAppliesSenderRules(t *testing.T) {
	store, err := NewReviewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.SaveSenderRules([]domain.EmailSummary{{From: "Deals <deals@example.com>"}}, domain.CategoryUnwanted); err != nil {
		t.Fatalf("save rule: %v", err)
	}
	applied := store.ApplySenderRules([]domain.EmailSummary{{
		ID:       "email-1",
		From:     "deals@example.com",
		Category: domain.CategoryPromotions,
	}})
	if got := applied[0].Category; got != domain.CategoryUnwanted {
		t.Fatalf("expected unwanted, got %s", got)
	}
	if applied[0].Reason != "Sender rule." || applied[0].Confidence != 1 {
		t.Fatalf("expected sender rule confidence/reason, got %#v", applied[0])
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.SenderRules != 1 {
		t.Fatalf("expected one sender rule, got %d", stats.SenderRules)
	}
}
