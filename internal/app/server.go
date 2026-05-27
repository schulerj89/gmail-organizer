package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/classifier"
	"github.com/schulerj89/gmail-organizer/internal/config"
	"github.com/schulerj89/gmail-organizer/internal/domain"
	"github.com/schulerj89/gmail-organizer/internal/gmail"
	"github.com/schulerj89/gmail-organizer/internal/monitor"
	"github.com/schulerj89/gmail-organizer/internal/scan"
	"github.com/schulerj89/gmail-organizer/internal/secrets"
	"github.com/schulerj89/gmail-organizer/internal/store"
)

type Server struct {
	cfg        config.Config
	gmail      *gmail.Service
	heuristic  classifier.Classifier
	ai         classifier.Classifier
	store      *store.ReviewStore
	monitor    *monitor.Service
	scan       *scan.Service
	lastEmails []domain.EmailSummary
	state      string
	mu         sync.RWMutex
}

func NewServer(cfg config.Config) (*Server, error) {
	googleSecret := secrets.FileSecret{Path: cfg.GoogleClientSecretFile}
	gmailService, err := gmail.NewService(context.Background(), googleSecret, cfg.DataDir, cfg.OAuthRedirectURL())
	if err != nil {
		return nil, err
	}
	reviewStore, err := store.NewReviewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	server := &Server{
		cfg:       cfg,
		gmail:     gmailService,
		heuristic: classifier.NewHeuristicClassifier(),
		ai:        classifier.NewOpenAIResponsesClassifier(secrets.FileSecret{Path: cfg.OpenAIKeyFile}, cfg.OpenAIModel),
		store:     reviewStore,
		state:     randomState(),
	}
	server.monitor = monitor.NewService(server.fetchAndClassify, time.Duration(cfg.MonitorInterval)*time.Second, cfg.MonitorCacheLimit)
	server.scan = scan.NewService(server.fetchPageAndClassify, server.store.SaveClassifications, cfg.ScanCacheLimit)
	return server, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/auth/google/url", s.handleGoogleAuthURL)
	mux.HandleFunc("GET /api/auth/google/callback", s.handleGoogleCallback)
	mux.HandleFunc("GET /api/emails", s.handleEmails)
	mux.HandleFunc("POST /api/classify", s.handleClassify)
	mux.HandleFunc("POST /api/actions", s.handleAction)
	mux.HandleFunc("GET /api/audit", s.handleAudit)
	mux.HandleFunc("GET /api/monitor", s.handleMonitorStatus)
	mux.HandleFunc("POST /api/monitor/start", s.handleMonitorStart)
	mux.HandleFunc("POST /api/monitor/stop", s.handleMonitorStop)
	mux.HandleFunc("GET /api/scan", s.handleScanStatus)
	mux.HandleFunc("POST /api/scan/start", s.handleScanStart)
	mux.HandleFunc("POST /api/scan/stop", s.handleScanStop)
	mux.Handle("/", s.staticHandler())
	return withSecurityHeaders(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"gmailAuthenticated": s.gmail.Authenticated(),
		"googleClientSecret": secrets.FileSecret{Path: s.cfg.GoogleClientSecretFile}.SafeStatus(),
		"openAIKey":          secrets.FileSecret{Path: s.cfg.OpenAIKeyFile}.SafeStatus(),
		"openAIModel":        s.cfg.OpenAIModel,
		"openAIEnabled":      s.cfg.EnableOpenAI,
	})
}

func (s *Server) handleGoogleAuthURL(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"url": s.gmail.AuthURL(s.state)})
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("state") != s.state {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}
	if err := s.gmail.Exchange(r.Context(), r.URL.Query().Get("code")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<!doctype html><title>Gmail Organizer</title><p>Gmail authorization saved. You can close this tab and return to the app.</p>"))
}

func (s *Server) handleEmails(w http.ResponseWriter, r *http.Request) {
	max := int64FromQuery(r, "max", 50)
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	emails, source, err := s.fetchAndClassify(r.Context(), query, max, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": source, "emails": emails})
}

func (s *Server) handleClassify(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Emails []domain.EmailSummary `json:"emails"`
		UseAI  bool                  `json:"useAI"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(payload.Emails) == 0 {
		payload.Emails = s.snapshot()
	}
	classified := s.applyClassifications(r.Context(), payload.Emails, payload.UseAI)
	_ = s.store.SaveClassifications(classified)
	s.remember(classified)
	writeJSON(w, http.StatusOK, map[string]any{"emails": classified})
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Action domain.BulkAction `json:"action"`
		IDs    []string          `json:"ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(payload.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no email ids were provided")
		return
	}
	var (
		results []domain.ActionResult
		err     error
	)
	switch payload.Action {
	case domain.ActionTrash:
		results, err = s.gmail.Trash(r.Context(), payload.IDs)
	case domain.ActionMarkRead:
		results, err = s.gmail.MarkRead(r.Context(), payload.IDs)
	case domain.ActionUnsubscribe:
		results = gmail.UnsubscribeResults(r.Context(), s.snapshot(), payload.IDs)
	default:
		err = errors.New("unsupported action")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.RecordAction(payload.Action, payload.IDs, results)
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.RecentAudit(int(int64FromQuery(r, "limit", 50)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) handleMonitorStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.monitor.Status())
}

func (s *Server) handleMonitorStart(w http.ResponseWriter, r *http.Request) {
	var payload monitor.Options
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.monitor.Start(context.Background(), payload)
	writeJSON(w, http.StatusOK, s.monitor.Status())
}

func (s *Server) handleMonitorStop(w http.ResponseWriter, _ *http.Request) {
	s.monitor.Stop()
	writeJSON(w, http.StatusOK, s.monitor.Status())
}

func (s *Server) handleScanStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.scan.Status())
}

func (s *Server) handleScanStart(w http.ResponseWriter, r *http.Request) {
	var payload scan.Options
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.scan.Start(context.Background(), payload)
	writeJSON(w, http.StatusOK, s.scan.Status())
}

func (s *Server) handleScanStop(w http.ResponseWriter, _ *http.Request) {
	s.scan.Stop()
	writeJSON(w, http.StatusOK, s.scan.Status())
}

func (s *Server) fetchAndClassify(ctx context.Context, query string, max int64, preferAI bool) ([]domain.EmailSummary, string, error) {
	emails, err := s.gmail.List(ctx, query, max)
	source := "gmail"
	if err != nil {
		emails = gmail.DemoEmails()
		source = "demo"
	}
	classified := s.applyClassifications(ctx, emails, preferAI)
	classified = s.store.Apply(classified)
	s.remember(classified)
	return classified, source, nil
}

func (s *Server) fetchPageAndClassify(ctx context.Context, query string, pageToken string, batchSize int64, preferAI bool) ([]domain.EmailSummary, string, string, error) {
	emails, nextToken, err := s.gmail.ListPage(ctx, query, pageToken, batchSize)
	source := "gmail"
	if err != nil {
		if pageToken != "" {
			return nil, "", "", err
		}
		emails = gmail.DemoEmails()
		nextToken = ""
		source = "demo"
	}
	classified := s.applyClassifications(ctx, emails, preferAI)
	classified = s.store.Apply(classified)
	s.remember(classified)
	return classified, nextToken, source, nil
}

func (s *Server) applyClassifications(ctx context.Context, emails []domain.EmailSummary, preferAI bool) []domain.EmailSummary {
	classifications, _ := s.heuristic.Classify(ctx, emails)
	if preferAI && s.cfg.EnableOpenAI && (secrets.FileSecret{Path: s.cfg.OpenAIKeyFile}).Exists() {
		if aiClassifications, aiErr := s.ai.Classify(ctx, emails); aiErr == nil {
			classifications = aiClassifications
		}
	}
	byID := map[string]domain.Classification{}
	for _, item := range classifications {
		byID[item.EmailID] = item
	}
	out := make([]domain.EmailSummary, 0, len(emails))
	for _, email := range emails {
		if classification, ok := byID[email.ID]; ok {
			email.Category = classification.Category
			email.Confidence = classification.Confidence
			email.Reason = classification.Reason
		}
		out = append(out, email)
	}
	return out
}

func (s *Server) remember(emails []domain.EmailSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEmails = append([]domain.EmailSummary(nil), emails...)
}

func (s *Server) snapshot() []domain.EmailSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.EmailSummary(nil), s.lastEmails...)
}

func (s *Server) staticHandler() http.Handler {
	if _, err := os.Stat(filepath.Join(s.cfg.FrontendDistDir, "index.html")); err == nil {
		return http.FileServer(http.Dir(s.cfg.FrontendDistDir))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Frontend is not built yet. Run npm install && npm run build in web/."})
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func int64FromQuery(r *http.Request, key string, fallback int64) int64 {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func randomState() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(raw[:])
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}
