package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

type ReviewStore struct {
	statePath string
	auditPath string
	mu        sync.Mutex
}

type StoredClassification struct {
	EmailID    string          `json:"emailId"`
	Category   domain.Category `json:"category"`
	Confidence float64         `json:"confidence"`
	Reason     string          `json:"reason"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type AuditEntry struct {
	Action    domain.BulkAction     `json:"action"`
	EmailIDs  []string              `json:"emailIds"`
	Results   []domain.ActionResult `json:"results"`
	CreatedAt time.Time             `json:"createdAt"`
}

type ReviewStats struct {
	Total       int                     `json:"total"`
	Manual      int                     `json:"manual"`
	NeedsReview int                     `json:"needsReview"`
	ByCategory  map[domain.Category]int `json:"byCategory"`
	UpdatedAt   *time.Time              `json:"updatedAt,omitempty"`
}

func NewReviewStore(dataDir string) (*ReviewStore, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return &ReviewStore{
		statePath: filepath.Join(dataDir, "review_state.json"),
		auditPath: filepath.Join(dataDir, "action_audit.jsonl"),
	}, nil
}

func (s *ReviewStore) Apply(emails []domain.EmailSummary) []domain.EmailSummary {
	state, err := s.loadState()
	if err != nil {
		return emails
	}
	out := make([]domain.EmailSummary, 0, len(emails))
	for _, email := range emails {
		if saved, ok := state[email.ID]; ok {
			email.Category = saved.Category
			email.Confidence = saved.Confidence
			email.Reason = saved.Reason
		}
		out = append(out, email)
	}
	return out
}

func (s *ReviewStore) SaveClassifications(emails []domain.EmailSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadStateLocked()
	if err != nil {
		state = map[string]StoredClassification{}
	}
	now := time.Now().UTC()
	for _, email := range emails {
		if email.ID == "" {
			continue
		}
		state[email.ID] = StoredClassification{
			EmailID:    email.ID,
			Category:   email.Category,
			Confidence: email.Confidence,
			Reason:     email.Reason,
			UpdatedAt:  now,
		}
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath, raw, 0o600)
}

func (s *ReviewStore) RecordAction(action domain.BulkAction, ids []string, results []domain.ActionResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(s.auditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	entry := AuditEntry{
		Action:    action,
		EmailIDs:  append([]string(nil), ids...),
		Results:   append([]domain.ActionResult(nil), results...),
		CreatedAt: time.Now().UTC(),
	}
	return json.NewEncoder(file).Encode(entry)
}

func (s *ReviewStore) Stats() (ReviewStats, error) {
	state, err := s.loadState()
	if err != nil {
		if os.IsNotExist(err) {
			return ReviewStats{ByCategory: map[domain.Category]int{}}, nil
		}
		return ReviewStats{}, err
	}
	stats := ReviewStats{
		Total:      len(state),
		ByCategory: map[domain.Category]int{},
	}
	var latest time.Time
	for _, item := range state {
		stats.ByCategory[item.Category]++
		if item.Category == domain.CategoryNeedsReview {
			stats.NeedsReview++
		}
		if item.Reason == "Manually categorized." {
			stats.Manual++
		}
		if item.UpdatedAt.After(latest) {
			latest = item.UpdatedAt
		}
	}
	if !latest.IsZero() {
		stats.UpdatedAt = &latest
	}
	return stats, nil
}

func (s *ReviewStore) RecentAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []AuditEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()
	entries := make([]AuditEntry, 0, limit)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			entries = append(entries, entry)
			if len(entries) > limit {
				entries = entries[1:]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *ReviewStore) loadState() (map[string]StoredClassification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadStateLocked()
}

func (s *ReviewStore) loadStateLocked() (map[string]StoredClassification, error) {
	raw, err := os.ReadFile(s.statePath)
	if err != nil {
		return nil, err
	}
	var state map[string]StoredClassification
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return state, nil
}
