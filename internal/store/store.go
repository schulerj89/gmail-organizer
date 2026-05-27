package store

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
	_ "modernc.org/sqlite"
)

const maxAuditEntryBytes = 8 << 20

type ReviewStore struct {
	db        *sql.DB
	dbPath    string
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
	dbPath := filepath.Join(dataDir, "review_state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &ReviewStore{
		db:        db,
		dbPath:    dbPath,
		statePath: filepath.Join(dataDir, "review_state.json"),
		auditPath: filepath.Join(dataDir, "action_audit.jsonl"),
		rulesPath: filepath.Join(dataDir, "sender_rules.json"),
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *ReviewStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *ReviewStore) init() error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS classifications (
			email_id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL DEFAULT '',
			from_addr TEXT NOT NULL DEFAULT '',
			subject TEXT NOT NULL DEFAULT '',
			snippet TEXT NOT NULL DEFAULT '',
			received_at TEXT NOT NULL DEFAULT '',
			label_ids TEXT NOT NULL DEFAULT '[]',
			category TEXT NOT NULL,
			confidence REAL NOT NULL DEFAULT 0,
			reason TEXT NOT NULL DEFAULT '',
			has_unsubscribe INTEGER NOT NULL DEFAULT 0,
			unsubscribe_target TEXT NOT NULL DEFAULT '',
			unsubscribe_method TEXT NOT NULL DEFAULT '',
			can_auto_unsubscribe INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_classifications_category_received ON classifications(category, received_at DESC, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS sender_rules (
			sender TEXT PRIMARY KEY,
			category TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			email_ids TEXT NOT NULL,
			results TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return s.importLegacyIfEmpty()
}

func (s *ReviewStore) Apply(emails []domain.EmailSummary) []domain.EmailSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.db.Prepare(`SELECT category, confidence, reason FROM classifications WHERE email_id = ?`)
	if err != nil {
		return emails
	}
	defer stmt.Close()
	out := make([]domain.EmailSummary, 0, len(emails))
	for _, email := range emails {
		var category string
		var confidence float64
		var reason string
		if err := stmt.QueryRow(email.ID).Scan(&category, &confidence, &reason); err == nil {
			email.Category = domain.Category(category)
			email.Confidence = confidence
			email.Reason = reason
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
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	if err := saveClassificationsTx(tx, emails, time.Now().UTC()); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *ReviewStore) DeleteClassifications(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`DELETE FROM classifications WHERE email_id = ?`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := stmt.Exec(id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *ReviewStore) ListEmails(category domain.Category, limit int, offset int) (StoredEmailPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	args := []any{}
	where := ""
	if category != "" {
		where = "WHERE category = ?"
		args = append(args, string(category))
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM classifications `+where, args...).Scan(&total); err != nil {
		return StoredEmailPage{}, err
	}
	if offset > total {
		offset = total
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(`SELECT email_id, thread_id, from_addr, subject, snippet, received_at, label_ids, category, confidence, reason, has_unsubscribe, unsubscribe_target, unsubscribe_method, can_auto_unsubscribe, updated_at
		FROM classifications `+where+`
		ORDER BY received_at DESC, updated_at DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return StoredEmailPage{}, err
	}
	defer rows.Close()
	emails := []domain.EmailSummary{}
	for rows.Next() {
		item, err := scanClassification(rows)
		if err != nil {
			return StoredEmailPage{}, err
		}
		emails = append(emails, item.toEmailSummary())
	}
	if err := rows.Err(); err != nil {
		return StoredEmailPage{}, err
	}
	return StoredEmailPage{Emails: emails, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *ReviewStore) SaveSenderRules(emails []domain.EmailSummary, category domain.Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO sender_rules(sender, category, updated_at)
		VALUES(?, ?, ?)
		ON CONFLICT(sender) DO UPDATE SET category = excluded.category, updated_at = excluded.updated_at`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	now := formatTime(time.Now().UTC())
	for _, email := range emails {
		sender := normalizeSender(email.From)
		if sender == "" {
			continue
		}
		if _, err := stmt.Exec(sender, string(category), now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *ReviewStore) RecordAction(action domain.BulkAction, ids []string, results []domain.ActionResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	emailIDs, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	rawResults, err := json.Marshal(results)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO audit_entries(action, email_ids, results, created_at) VALUES(?, ?, ?, ?)`,
		string(action), string(emailIDs), string(rawResults), formatTime(time.Now().UTC()))
	return err
}

func (s *ReviewStore) Stats() (ReviewStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := ReviewStats{ByCategory: map[domain.Category]int{}}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM classifications`).Scan(&stats.Total); err != nil {
		return ReviewStats{}, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM classifications WHERE category = ?`, string(domain.CategoryNeedsReview)).Scan(&stats.NeedsReview); err != nil {
		return ReviewStats{}, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM classifications WHERE reason = ?`, "Manually categorized.").Scan(&stats.Manual); err != nil {
		return ReviewStats{}, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sender_rules`).Scan(&stats.SenderRules); err != nil {
		return ReviewStats{}, err
	}
	var latest string
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(updated_at), '') FROM classifications`).Scan(&latest); err != nil {
		return ReviewStats{}, err
	}
	if parsed := parseTime(latest); !parsed.IsZero() {
		stats.UpdatedAt = &parsed
	}
	rows, err := s.db.Query(`SELECT category, COUNT(*) FROM classifications GROUP BY category`)
	if err != nil {
		return ReviewStats{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var category string
		var count int
		if err := rows.Scan(&category, &count); err != nil {
			return ReviewStats{}, err
		}
		stats.ByCategory[domain.Category(category)] = count
	}
	if err := rows.Err(); err != nil {
		return ReviewStats{}, err
	}
	return stats, nil
}

func (s *ReviewStore) RecentAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT action, email_ids, results, created_at FROM audit_entries ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []AuditEntry{}
	for rows.Next() {
		var entry AuditEntry
		var action, ids, results, createdAt string
		if err := rows.Scan(&action, &ids, &results, &createdAt); err != nil {
			return nil, err
		}
		entry.Action = domain.BulkAction(action)
		entry.CreatedAt = parseTime(createdAt)
		_ = json.Unmarshal([]byte(ids), &entry.EmailIDs)
		_ = json.Unmarshal([]byte(results), &entry.Results)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

func (s *ReviewStore) loadRules() (map[string]SenderRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT sender, category, updated_at FROM sender_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := map[string]SenderRule{}
	for rows.Next() {
		var sender, category, updatedAt string
		if err := rows.Scan(&sender, &category, &updatedAt); err != nil {
			return nil, err
		}
		rules[sender] = SenderRule{
			Sender:    sender,
			Category:  domain.Category(category),
			UpdatedAt: parseTime(updatedAt),
		}
	}
	return rules, rows.Err()
}

func saveClassificationsTx(tx *sql.Tx, emails []domain.EmailSummary, now time.Time) error {
	stmt, err := tx.Prepare(`INSERT INTO classifications(email_id, thread_id, from_addr, subject, snippet, received_at, label_ids, category, confidence, reason, has_unsubscribe, unsubscribe_target, unsubscribe_method, can_auto_unsubscribe, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(email_id) DO UPDATE SET
			thread_id = excluded.thread_id,
			from_addr = excluded.from_addr,
			subject = excluded.subject,
			snippet = excluded.snippet,
			received_at = excluded.received_at,
			label_ids = excluded.label_ids,
			category = excluded.category,
			confidence = excluded.confidence,
			reason = excluded.reason,
			has_unsubscribe = excluded.has_unsubscribe,
			unsubscribe_target = excluded.unsubscribe_target,
			unsubscribe_method = excluded.unsubscribe_method,
			can_auto_unsubscribe = excluded.can_auto_unsubscribe,
			updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, email := range emails {
		if email.ID == "" {
			continue
		}
		labels, err := json.Marshal(email.LabelIDs)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(
			email.ID,
			email.ThreadID,
			email.From,
			email.Subject,
			email.Snippet,
			formatTime(email.ReceivedAt),
			string(labels),
			string(email.Category),
			email.Confidence,
			email.Reason,
			boolInt(email.HasUnsubscribe),
			email.UnsubscribeTarget,
			email.UnsubscribeMethod,
			boolInt(email.CanAutoUnsubscribe),
			formatTime(now),
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *ReviewStore) importLegacyIfEmpty() error {
	var classificationCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM classifications`).Scan(&classificationCount); err != nil {
		return err
	}
	if classificationCount == 0 {
		if err := s.importLegacyClassifications(); err != nil {
			return err
		}
	}
	var ruleCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sender_rules`).Scan(&ruleCount); err != nil {
		return err
	}
	if ruleCount == 0 {
		if err := s.importLegacyRules(); err != nil {
			return err
		}
	}
	var auditCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_entries`).Scan(&auditCount); err != nil {
		return err
	}
	if auditCount == 0 {
		return s.importLegacyAudit()
	}
	return nil
}

func (s *ReviewStore) importLegacyClassifications() error {
	raw, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var state map[string]StoredClassification
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	emails := make([]domain.EmailSummary, 0, len(state))
	for _, item := range state {
		emails = append(emails, item.toEmailSummary())
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	if err := saveClassificationsTx(tx, emails, time.Now().UTC()); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *ReviewStore) importLegacyRules() error {
	raw, err := os.ReadFile(s.rulesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var rules map[string]SenderRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO sender_rules(sender, category, updated_at) VALUES(?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for sender, rule := range rules {
		if _, err := stmt.Exec(sender, string(rule.Category), formatTime(rule.UpdatedAt)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *ReviewStore) importLegacyAudit() error {
	file, err := os.Open(s.auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO audit_entries(action, email_ids, results, created_at) VALUES(?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxAuditEntryBytes)
	for scanner.Scan() {
		var entry AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		emailIDs, err := json.Marshal(entry.EmailIDs)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		results, err := json.Marshal(entry.Results)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := stmt.Exec(string(entry.Action), string(emailIDs), string(results), formatTime(entry.CreatedAt)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type classificationScanner interface {
	Scan(dest ...any) error
}

func scanClassification(scanner classificationScanner) (StoredClassification, error) {
	var item StoredClassification
	var receivedAt, updatedAt, labelIDs string
	var category string
	var hasUnsubscribe, canAutoUnsubscribe int
	err := scanner.Scan(
		&item.EmailID,
		&item.ThreadID,
		&item.From,
		&item.Subject,
		&item.Snippet,
		&receivedAt,
		&labelIDs,
		&category,
		&item.Confidence,
		&item.Reason,
		&hasUnsubscribe,
		&item.UnsubscribeTarget,
		&item.UnsubscribeMethod,
		&canAutoUnsubscribe,
		&updatedAt,
	)
	if err != nil {
		return StoredClassification{}, err
	}
	item.ReceivedAt = parseTime(receivedAt)
	item.UpdatedAt = parseTime(updatedAt)
	item.Category = domain.Category(category)
	item.HasUnsubscribe = hasUnsubscribe != 0
	item.CanAutoUnsubscribe = canAutoUnsubscribe != 0
	_ = json.Unmarshal([]byte(labelIDs), &item.LabelIDs)
	return item, nil
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

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
