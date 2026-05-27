package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

func TestBoundedMergeKeepsNewestUniqueEmails(t *testing.T) {
	existing := []domain.EmailSummary{{ID: "old-1"}, {ID: "shared"}, {ID: "old-2"}}
	incoming := []domain.EmailSummary{{ID: "new-1"}, {ID: "shared"}, {ID: "new-2"}}

	got := boundedMerge(existing, incoming, 4)

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
	time.Sleep(25 * time.Millisecond)
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
