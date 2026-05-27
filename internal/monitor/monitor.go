package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

type PollFunc func(ctx context.Context, query string, max int64, useAI bool) ([]domain.EmailSummary, string, error)

type Service struct {
	pollFn     PollFunc
	interval   time.Duration
	cacheLimit int

	mu       sync.RWMutex
	cancel   context.CancelFunc
	running  bool
	query    string
	max      int64
	useAI    bool
	source   string
	lastErr  string
	lastPoll time.Time
	lastOK   time.Time
	cache    []domain.EmailSummary
}

type Options struct {
	Query string `json:"query"`
	Max   int64  `json:"max"`
	UseAI bool   `json:"useAI"`
}

type Status struct {
	Running         bool                  `json:"running"`
	Query           string                `json:"query"`
	Max             int64                 `json:"max"`
	UseAI           bool                  `json:"useAI"`
	Source          string                `json:"source"`
	CacheSize       int                   `json:"cacheSize"`
	CacheLimit      int                   `json:"cacheLimit"`
	IntervalSeconds int                   `json:"intervalSeconds"`
	LastPollAt      *time.Time            `json:"lastPollAt,omitempty"`
	LastSuccessAt   *time.Time            `json:"lastSuccessAt,omitempty"`
	LastError       string                `json:"lastError,omitempty"`
	Emails          []domain.EmailSummary `json:"emails"`
}

func NewService(pollFn PollFunc, interval time.Duration, cacheLimit int) *Service {
	if interval < 15*time.Second {
		interval = 15 * time.Second
	}
	if cacheLimit < 50 {
		cacheLimit = 50
	}
	return &Service{
		pollFn:     pollFn,
		interval:   interval,
		cacheLimit: cacheLimit,
		query:      "newer_than:30d",
		max:        50,
	}
}

func (s *Service) Start(parent context.Context, options Options) {
	s.Stop()
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cancel = cancel
	s.running = true
	s.query = defaultString(options.Query, "newer_than:30d")
	s.max = clampMax(options.Max)
	s.useAI = options.UseAI
	s.lastErr = ""
	s.mu.Unlock()

	go s.loop(ctx)
}

func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.running = false
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
		Running:         s.running,
		Query:           s.query,
		Max:             s.max,
		UseAI:           s.useAI,
		Source:          s.source,
		CacheSize:       len(s.cache),
		CacheLimit:      s.cacheLimit,
		IntervalSeconds: int(s.interval.Seconds()),
		LastPollAt:      timePtr(s.lastPoll),
		LastSuccessAt:   timePtr(s.lastOK),
		LastError:       s.lastErr,
		Emails:          emails,
	}
}

func (s *Service) loop(ctx context.Context) {
	s.poll(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

func (s *Service) poll(ctx context.Context) {
	s.mu.RLock()
	query, max, useAI := s.query, s.max, s.useAI
	s.mu.RUnlock()

	pollCtx, cancel := context.WithTimeout(ctx, minDuration(s.interval/2, 45*time.Second))
	defer cancel()
	emails, source, err := s.pollFn(pollCtx, query, max, useAI)
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastPoll = now
	if err != nil {
		s.lastErr = err.Error()
		return
	}
	s.source = source
	s.lastErr = ""
	s.lastOK = now
	s.cache = boundedMerge(s.cache, emails, s.cacheLimit)
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

func clampMax(value int64) int64 {
	if value <= 0 {
		return 50
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

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
