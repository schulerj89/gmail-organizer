package emailcache

import "github.com/schulerj89/gmail-organizer/internal/domain"

// MergeNewestUnique returns incoming emails first, then older cached emails,
// skipping blank and duplicate IDs while respecting the configured limit.
func MergeNewestUnique(existing []domain.EmailSummary, incoming []domain.EmailSummary, limit int) []domain.EmailSummary {
	if limit <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]domain.EmailSummary, 0, min(limit, len(existing)+len(incoming)))
	for _, email := range incoming {
		if email.ID == "" {
			continue
		}
		if _, ok := seen[email.ID]; ok {
			continue
		}
		seen[email.ID] = struct{}{}
		merged = append(merged, email)
		if len(merged) == limit {
			return merged
		}
	}
	for _, email := range existing {
		if email.ID == "" {
			continue
		}
		if _, ok := seen[email.ID]; ok {
			continue
		}
		seen[email.ID] = struct{}{}
		merged = append(merged, email)
		if len(merged) == limit {
			return merged
		}
	}
	return merged
}
