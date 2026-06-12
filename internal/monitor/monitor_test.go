package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

func TestServiceKeepsNewestUniqueCachedEmails(t *testing.T) {
	calls := 0
	service := NewService(func(_ context.Context, _ string, _ int64, _ bool) ([]domain.EmailSummary, string, error) {
		calls++
		if calls == 1 {
			return []domain.EmailSummary{{ID: "old-1"}, {ID: "shared"}, {ID: "old-2"}}, "test", nil
		}
		return []domain.EmailSummary{{ID: "new-1"}, {ID: "shared"}, {ID: "new-2"}}, "test", nil
	}, time.Minute, 50)
	service.cacheLimit = 4

	service.poll(context.Background())
	service.poll(context.Background())

	got := service.Status().Emails
	want := []string{"new-1", "shared", "new-2", "old-1"}
	if len(got) != len(want) {
		t.Fatalf("expected %d emails, got %d", len(want), len(got))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("index %d expected %s, got %s", i, id, got[i].ID)
		}
	}
}

func TestServiceStartsPollsAndStops(t *testing.T) {
	calls := 0
	service := NewService(func(_ context.Context, _ string, _ int64, _ bool) ([]domain.EmailSummary, string, error) {
		calls++
		return []domain.EmailSummary{{ID: fmt.Sprintf("email-%d", calls)}}, "test", nil
	}, 15*time.Second, 50)

	service.Start(context.Background(), Options{Query: "newer_than:1d", Max: 10})
	waitFor(t, func() bool { return service.Status().CacheSize == 1 })
	service.Stop()

	status := service.Status()
	if status.Running {
		t.Fatal("expected service to be stopped")
	}
	if status.CacheSize != 1 {
		t.Fatalf("expected one cached email, got %d", status.CacheSize)
	}
	if status.Source != "test" {
		t.Fatalf("expected source test, got %s", status.Source)
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
