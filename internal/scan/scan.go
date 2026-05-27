package scan

import (
	"context"
	"sync"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

type FetchPageFunc func(ctx context.Context, query string, pageToken string, batchSize int64, useAI bool) ([]domain.EmailSummary, string, string, error)
type SaveFunc func([]domain.EmailSummary) error

type Service struct {
	fetchPage  FetchPageFunc
	save       SaveFunc
	cacheLimit int

	mu        sync.RWMutex
	cancel    context.CancelFunc
	running   bool
	completed bool
	query     string
	useAI     bool
	total     int
	limit     int
	batchSize int64
	source    string
	nextToken string
	lastErr   string
	startedAt time.Time
	endedAt   time.Time
	cache     []domain.EmailSummary
}

type Options struct {
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
	BatchSize int64  `json:"batchSize"`
	UseAI     bool   `json:"useAI"`
}

type Status struct {
	Running    bool                  `json:"running"`
	Completed  bool                  `json:"completed"`
	Query      string                `json:"query"`
	UseAI      bool                  `json:"useAI"`
	Processed  int                   `json:"processed"`
	Limit      int                   `json:"limit"`
	BatchSize  int64                 `json:"batchSize"`
	Source     string                `json:"source"`
	HasMore    bool                  `json:"hasMore"`
	CacheSize  int                   `json:"cacheSize"`
	CacheLimit int                   `json:"cacheLimit"`
	LastError  string                `json:"lastError,omitempty"`
	StartedAt  *time.Time            `json:"startedAt,omitempty"`
	EndedAt    *time.Time            `json:"endedAt,omitempty"`
	Emails     []domain.EmailSummary `json:"emails"`
}

func NewService(fetchPage FetchPageFunc, save SaveFunc, cacheLimit int) *Service {
	if cacheLimit < 100 {
		cacheLimit = 100
	}
	return &Service{
		fetchPage:  fetchPage,
		save:       save,
		cacheLimit: cacheLimit,
		query:      "newer_than:365d",
		limit:      1000,
		batchSize:  100,
	}
}

func (s *Service) Start(parent context.Context, options Options) {
	s.Stop()
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cancel = cancel
	s.running = true
	s.completed = false
	s.query = defaultString(options.Query, "newer_than:365d")
	s.useAI = options.UseAI
	s.limit = clampLimit(options.Limit)
	s.batchSize = clampBatchSize(options.BatchSize)
	s.total = 0
	s.source = ""
	s.nextToken = ""
	s.lastErr = ""
	s.startedAt = time.Now().UTC()
	s.endedAt = time.Time{}
	s.cache = []domain.EmailSummary{}
	s.mu.Unlock()

	go s.loop(ctx)
}

func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.running = false
	if !s.completed && !s.startedAt.IsZero() && s.endedAt.IsZero() {
		s.endedAt = time.Now().UTC()
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	emails := append([]domain.EmailSummary(nil), s.cache...)
	if emails == nil {
		emails = []domain.EmailSummary{}
	}
	return Status{
		Running:    s.running,
		Completed:  s.completed,
		Query:      s.query,
		UseAI:      s.useAI,
		Processed:  s.total,
		Limit:      s.limit,
		BatchSize:  s.batchSize,
		Source:     s.source,
		HasMore:    s.nextToken != "",
		CacheSize:  len(s.cache),
		CacheLimit: s.cacheLimit,
		LastError:  s.lastErr,
		StartedAt:  timePtr(s.startedAt),
		EndedAt:    timePtr(s.endedAt),
		Emails:     emails,
	}
}

func (s *Service) loop(ctx context.Context) {
	defer s.finish()
	for {
		s.mu.RLock()
		query, token, batchSize, limit, total, useAI := s.query, s.nextToken, s.batchSize, s.limit, s.total, s.useAI
		s.mu.RUnlock()
		if total >= limit {
			return
		}
		remaining := int64(limit - total)
		if remaining < batchSize {
			batchSize = remaining
		}
		batchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		emails, nextToken, source, err := s.fetchPage(batchCtx, query, token, batchSize, useAI)
		cancel()
		if err != nil {
			s.recordError(err.Error())
			return
		}
		if len(emails) > 0 && s.save != nil {
			_ = s.save(emails)
		}
		s.recordBatch(emails, nextToken, source)
		if nextToken == "" || len(emails) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (s *Service) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.completed = s.lastErr == ""
	s.cancel = nil
	s.endedAt = time.Now().UTC()
}

func (s *Service) recordBatch(emails []domain.EmailSummary, nextToken string, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += len(emails)
	s.source = source
	s.nextToken = nextToken
	s.cache = boundedMerge(s.cache, emails, s.cacheLimit)
}

func (s *Service) recordError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = message
}

func boundedMerge(existing []domain.EmailSummary, incoming []domain.EmailSummary, limit int) []domain.EmailSummary {
	if limit <= 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	merged := make([]domain.EmailSummary, 0, minInt(limit, len(existing)+len(incoming)))
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

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func clampLimit(value int) int {
	if value <= 0 {
		return 1000
	}
	if value > 10000 {
		return 10000
	}
	return value
}

func clampBatchSize(value int64) int64 {
	if value <= 0 {
		return 100
	}
	if value > 200 {
		return 200
	}
	return value
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
