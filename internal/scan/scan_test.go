package scan

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

func TestServiceScansPagesUntilLimit(t *testing.T) {
	saved := 0
	service := NewService(func(_ context.Context, _ string, token string, batchSize int64, _ bool) ([]domain.EmailSummary, string, string, error) {
		start := 0
		if token == "page-2" {
			start = 2
		}
		emails := make([]domain.EmailSummary, 0, batchSize)
		for i := 0; i < int(batchSize); i++ {
			emails = append(emails, domain.EmailSummary{ID: fmt.Sprintf("email-%d", start+i)})
		}
		if token == "" {
			return emails, "page-2", "test", nil
		}
		return emails, "", "test", nil
	}, func(emails []domain.EmailSummary) error {
		saved += len(emails)
		return nil
	}, 100)

	service.Start(context.Background(), Options{Limit: 3, BatchSize: 2})
	waitFor(t, func() bool { return service.Status().Completed })

	status := service.Status()
	if status.Processed != 3 {
		t.Fatalf("expected 3 processed, got %d", status.Processed)
	}
	if saved != 3 {
		t.Fatalf("expected 3 saved, got %d", saved)
	}
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}
