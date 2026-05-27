package store

import (
	"bufio"
	"encoding/json"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

const maxAuditEntryBytes = 8 << 20

type ReviewStore struct {
	statePath string
	auditPath string
	rulesPath string
	mu        sync.Mutex
}

type StoredClassification struct {
	EmailID            string          `json:"emailId"`
	ThreadID           string          `json:"threadId,omitempty"`
	From               string          `json:"from,omitempty"`
	Subject            string          `json:"subject,omitempty"`
	Snippet            string          `json:"snippet,omitempty"`
	ReceivedAt         time.Time       `json:"receivedAt,omitempty"`
	LabelIDs           []string        `json:"labelIds,omitempty"`
	Category           domain.Category `json:"category"`
	Confidence         float64         `json:"confidence"`
	Reason             string          `json:"reason"`
	HasUnsubscribe     bool            `json:"hasUnsubscribe,omitempty"`
	UnsubscribeTarget  string          `json:"unsubscribeTarget,omitempty"`
	UnsubscribeMethod  string          `json:"unsubscribeMethod,omitempty"`
	CanAutoUnsubscribe bool            `json:"canAutoUnsubscribe,omitempty"`
	UpdatedAt          time.Time       `json:"updatedAt"`
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
	SenderRules int                     `json:"senderRules"`
	ByCategory  map[domain.Category]int `json:"byCategory"`
	UpdatedAt   *time.Time              `json:"updatedAt,omitempty"`
}

type StoredEmailPage struct {
	Emails []domain.EmailSummary `json:"emails"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

type SenderRule struct {
	Sender    string          `json:"sender"`
	Category  domain.Category `json:"category"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func NewReviewStore(dataDir string) (*ReviewStore, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return &ReviewStore{
		statePath: filepath.Join(dataDir, "review_state.json"),
		auditPath: filepath.Join(dataDir, "action_audit.jsonl"),
		rulesPath: filepath.Join(dataDir, "sender_rules.json"),
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

func (s *ReviewStore) ApplySenderRules(emails []domain.EmailSummary) []domain.EmailSummary {
	rules, err := s.loadRules()
	if err != nil || len(rules) == 0 {
		return emails
	}
	out := make([]domain.EmailSummary, 0, len(emails))
	for _, email := range emails {
		sender := normalizeSender(email.From)
		if rule, ok := rules[sender]; ok {
			email.Category = rule.Category
			email.Confidence = 1
			email.Reason = "Sender rule."
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
			EmailID:            email.ID,
			ThreadID:           email.ThreadID,
			From:               email.From,
			Subject:            email.Subject,
			Snippet:            email.Snippet,
			ReceivedAt:         email.ReceivedAt,
			LabelIDs:           append([]string(nil), email.LabelIDs...),
			Category:           email.Category,
			Confidence:         email.Confidence,
			Reason:             email.Reason,
			HasUnsubscribe:     email.HasUnsubscribe,
			UnsubscribeTarget:  email.UnsubscribeTarget,
			UnsubscribeMethod:  email.UnsubscribeMethod,
			CanAutoUnsubscribe: email.CanAutoUnsubscribe,
			UpdatedAt:          now,
		}
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath, raw, 0o600)
}

func (s *ReviewStore) DeleteClassifications(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadStateLocked()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, id := range ids {
		delete(state, strings.TrimSpace(id))
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath, raw, 0o600)
}

func (s *ReviewStore) ListEmails(category domain.Category, limit int, offset int) (StoredEmailPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	state, err := s.loadState()
	if err != nil {
		if os.IsNotExist(err) {
			return StoredEmailPage{Emails: []domain.EmailSummary{}, Limit: limit, Offset: offset}, nil
		}
		return StoredEmailPage{}, err
	}
	all := make([]StoredClassification, 0, len(state))
	for _, item := range state {
		if category != "" && item.Category != category {
			continue
		}
		if item.EmailID == "" {
			continue
		}
		all = append(all, item)
	}
	sort.Slice(all, func(i, j int) bool {
		left := all[i].ReceivedAt
		right := all[j].ReceivedAt
		if left.Equal(right) {
			return all[i].UpdatedAt.After(all[j].UpdatedAt)
		}
		return left.After(right)
	})
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	emails := make([]domain.EmailSummary, 0, end-offset)
	for _, item := range all[offset:end] {
		emails = append(emails, item.toEmailSummary())
	}
	return StoredEmailPage{
		Emails: emails,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (s *ReviewStore) SaveSenderRules(emails []domain.EmailSummary, category domain.Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rules, err := s.loadRulesLocked()
	if err != nil {
		rules = map[string]SenderRule{}
	}
	now := time.Now().UTC()
	for _, email := range emails {
		sender := normalizeSender(email.From)
		if sender == "" {
			continue
		}
		rules[sender] = SenderRule{
			Sender:    sender,
			Category:  category,
			UpdatedAt: now,
		}
	}
	raw, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.rulesPath, raw, 0o600)
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
			stats := ReviewStats{ByCategory: map[domain.Category]int{}}
			if rules, rulesErr := s.loadRules(); rulesErr == nil {
				stats.SenderRules = len(rules)
			}
			return stats, nil
		}
		return ReviewStats{}, err
	}
	stats := ReviewStats{
		Total:      len(state),
		ByCategory: map[domain.Category]int{},
	}
	if rules, err := s.loadRules(); err == nil {
		stats.SenderRules = len(rules)
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
	scanner.Buffer(make([]byte, 64*1024), maxAuditEntryBytes)
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

func (s *ReviewStore) loadRules() (map[string]SenderRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadRulesLocked()
}

func (s *ReviewStore) loadRulesLocked() (map[string]SenderRule, error) {
	raw, err := os.ReadFile(s.rulesPath)
	if err != nil {
		return nil, err
	}
	var rules map[string]SenderRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func normalizeSender(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := mail.ParseAddress(value); err == nil {
		return strings.ToLower(strings.TrimSpace(parsed.Address))
	}
	value = strings.Trim(value, "<>")
	return strings.ToLower(strings.TrimSpace(value))
}

func (s StoredClassification) toEmailSummary() domain.EmailSummary {
	return domain.EmailSummary{
		ID:                 s.EmailID,
		ThreadID:           s.ThreadID,
		From:               s.From,
		Subject:            s.Subject,
		Snippet:            s.Snippet,
		ReceivedAt:         s.ReceivedAt,
		LabelIDs:           append([]string(nil), s.LabelIDs...),
		Category:           s.Category,
		Confidence:         s.Confidence,
		Reason:             s.Reason,
		HasUnsubscribe:     s.HasUnsubscribe,
		UnsubscribeTarget:  s.UnsubscribeTarget,
		UnsubscribeMethod:  s.UnsubscribeMethod,
		CanAutoUnsubscribe: s.CanAutoUnsubscribe,
	}
}
