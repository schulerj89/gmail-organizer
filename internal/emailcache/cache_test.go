package emailcache

import (
	"testing"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

func TestMergeNewestUniqueKeepsIncomingFirst(t *testing.T) {
	got := MergeNewestUnique(
		[]domain.EmailSummary{{ID: "old-1"}, {ID: "shared"}, {ID: "old-2"}},
		[]domain.EmailSummary{{ID: "new-1"}, {ID: "shared"}, {ID: "new-2"}},
		4,
	)

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

func TestMergeNewestUniqueSkipsBlankIDs(t *testing.T) {
	got := MergeNewestUnique(
		[]domain.EmailSummary{{ID: ""}, {ID: "old-1"}},
		[]domain.EmailSummary{{ID: ""}, {ID: "new-1"}},
		3,
	)

	want := []string{"new-1", "old-1"}
	if len(got) != len(want) {
		t.Fatalf("expected %d emails, got %d", len(want), len(got))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("index %d expected %s, got %s", i, id, got[i].ID)
		}
	}
}

func TestMergeNewestUniqueHonorsLimit(t *testing.T) {
	got := MergeNewestUnique(
		[]domain.EmailSummary{{ID: "old-1"}},
		[]domain.EmailSummary{{ID: "new-1"}, {ID: "new-2"}},
		2,
	)
	if len(got) != 2 || got[0].ID != "new-1" || got[1].ID != "new-2" {
		t.Fatalf("unexpected emails: %#v", got)
	}
}
