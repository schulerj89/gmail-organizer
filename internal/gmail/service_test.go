package gmail

import "testing"

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
