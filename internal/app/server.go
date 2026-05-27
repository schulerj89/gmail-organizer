package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
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
	cfg           config.Config
	gmail         *gmail.Service
	heuristic     classifier.Classifier
	ai            classifier.Classifier
	store         *store.ReviewStore
	monitor       *monitor.Service
	scan          *scan.Service
	lastEmails    []domain.EmailSummary
	state         string
	confirmations map[string]pendingConfirmation
	mu            sync.RWMutex
	confirmMu     sync.Mutex
}

type pendingConfirmation struct {
	Action    domain.BulkAction
	IDs       []string
	ExpiresAt time.Time
}

const confirmationTTL = 10 * time.Minute

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
		cfg:           cfg,
		gmail:         gmailService,
		heuristic:     classifier.NewHeuristicClassifier(),
		ai:            classifier.NewOpenAIResponsesClassifier(secrets.FileSecret{Path: cfg.OpenAIKeyFile}, cfg.OpenAIModel),
		store:         reviewStore,
		state:         randomState(),
		confirmations: map[string]pendingConfirmation{},
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
	mux.HandleFunc("POST /api/categories", s.handleCategoryUpdate)
	mux.HandleFunc("POST /api/actions", s.handleAction)
	mux.HandleFunc("GET /api/audit", s.handleAudit)
	mux.HandleFunc("GET /api/review", s.handleReviewStats)
	mux.HandleFunc("GET /api/review/emails", s.handleReviewEmails)
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

func (s *Server) handleCategoryUpdate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		IDs             []string        `json:"ids"`
		Category        domain.Category `json:"category"`
		ApplySenderRule bool            `json:"applySenderRule"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(payload.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no email ids were provided")
		return
	}
	if !domain.ValidCategory(payload.Category) {
		writeError(w, http.StatusBadRequest, "unsupported category")
		return
	}
	emails := s.updateCategories(payload.IDs, payload.Category)
	if len(emails) == 0 {
		writeError(w, http.StatusNotFound, "none of the selected emails are currently loaded")
		return
	}
	if err := s.store.SaveClassifications(emails); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if payload.ApplySenderRule {
		if err := s.store.SaveSenderRules(emails, payload.Category); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"emails": s.snapshot()})
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Action            domain.BulkAction `json:"action"`
		IDs               []string          `json:"ids"`
		ConfirmationToken string            `json:"confirmationToken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ids := normalizeIDs(payload.IDs)
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no email ids were provided")
		return
	}
	if len(ids) > 1000 {
		writeError(w, http.StatusBadRequest, "bulk actions are limited to 1000 emails per request")
		return
	}
	if destructiveAction(payload.Action) && payload.ConfirmationToken == "" {
		results, err := s.previewAction(payload.Action, ids)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		requires := requiresConfirmation(results)
		response := map[string]any{"results": results, "requiresConfirmation": requires}
		if requires {
			token, expiresAt := s.createConfirmation(payload.Action, ids)
			response["confirmationToken"] = token
			response["confirmationExpiresAt"] = expiresAt
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if destructiveAction(payload.Action) && !s.consumeConfirmation(payload.ConfirmationToken, payload.Action, ids) {
		writeError(w, http.StatusBadRequest, "destructive action confirmation is missing, expired, or does not match the preview")
		return
	}
	var (
		results []domain.ActionResult
		err     error
	)
	switch payload.Action {
	case domain.ActionTrash:
		results, err = s.gmail.Trash(r.Context(), ids)
	case domain.ActionMarkRead:
		results, err = s.gmail.MarkRead(r.Context(), ids)
	case domain.ActionUnsubscribe:
		results = gmail.UnsubscribeResults(r.Context(), s.snapshot(), ids)
	default:
		err = errors.New("unsupported action")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.RecordAction(payload.Action, ids, results)
	if payload.Action == domain.ActionTrash {
		trashedIDs := successfulActionIDs(results, "trashed")
		if len(trashedIDs) > 0 {
			_ = s.store.DeleteClassifications(trashedIDs)
			s.forget(trashedIDs)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "requiresConfirmation": false})
}

func (s *Server) previewAction(action domain.BulkAction, ids []string) ([]domain.ActionResult, error) {
	switch action {
	case domain.ActionTrash:
		results := make([]domain.ActionResult, 0, len(ids))
		for _, id := range ids {
			results = append(results, domain.ActionResult{EmailID: id, Status: "needs_confirmation", Message: "Confirm to move this message to Gmail trash."})
		}
		return results, nil
	case domain.ActionUnsubscribe:
		return gmail.PreviewUnsubscribeResults(s.snapshot(), ids), nil
	default:
		return nil, errors.New("unsupported action")
	}
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.RecentAudit(int(int64FromQuery(r, "limit", 50)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) handleReviewStats(w http.ResponseWriter, _ *http.Request) {
	stats, err := s.store.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleReviewEmails(w http.ResponseWriter, r *http.Request) {
	category := domain.Category(strings.TrimSpace(r.URL.Query().Get("category")))
	if category != "" && !domain.ValidCategory(category) {
		writeError(w, http.StatusBadRequest, "unsupported category")
		return
	}
	limit := int(int64FromQuery(r, "limit", 100))
	offset := int(int64FromQuery(r, "offset", 0))
	page, err := s.store.ListEmails(category, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.remember(page.Emails)
	writeJSON(w, http.StatusOK, map[string]any{
		"source": "review_store",
		"emails": page.Emails,
		"total":  page.Total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
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
	classified = s.store.ApplySenderRules(classified)
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
	classified = s.store.ApplySenderRules(classified)
	classified = s.store.Apply(classified)
	s.remember(classified)
	return classified, nextToken, source, nil
}

func (s *Server) applyClassifications(ctx context.Context, emails []domain.EmailSummary, preferAI bool) []domain.EmailSummary {
	classifications, _ := s.heuristic.Classify(ctx, emails)
	if preferAI && s.cfg.EnableOpenAI && (secrets.FileSecret{Path: s.cfg.OpenAIKeyFile}).Exists() {
		classifications = overlayAIClassifications(ctx, classifications, emails, s.ai, 50)
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

func overlayAIClassifications(ctx context.Context, fallback []domain.Classification, emails []domain.EmailSummary, ai classifier.Classifier, chunkSize int) []domain.Classification {
	if chunkSize <= 0 {
		chunkSize = 50
	}
	byID := map[string]domain.Classification{}
	for _, item := range fallback {
		byID[item.EmailID] = item
	}
	for start := 0; start < len(emails); start += chunkSize {
		end := start + chunkSize
		if end > len(emails) {
			end = len(emails)
		}
		classifications, err := ai.Classify(ctx, emails[start:end])
		if err != nil {
			continue
		}
		for _, item := range classifications {
			byID[item.EmailID] = item
		}
	}
	out := make([]domain.Classification, 0, len(byID))
	for _, email := range emails {
		if item, ok := byID[email.ID]; ok {
			out = append(out, item)
		}
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

func (s *Server) forget(ids []string) {
	selected := map[string]struct{}{}
	for _, id := range ids {
		selected[id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.lastEmails[:0]
	for _, email := range s.lastEmails {
		if _, ok := selected[email.ID]; ok {
			continue
		}
		kept = append(kept, email)
	}
	s.lastEmails = append([]domain.EmailSummary(nil), kept...)
}

func (s *Server) updateCategories(ids []string, category domain.Category) []domain.EmailSummary {
	selected := map[string]struct{}{}
	for _, id := range ids {
		selected[id] = struct{}{}
	}
	updated := []domain.EmailSummary{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.lastEmails {
		if _, ok := selected[s.lastEmails[i].ID]; !ok {
			continue
		}
		s.lastEmails[i].Category = category
		s.lastEmails[i].Confidence = 1
		s.lastEmails[i].Reason = "Manually categorized."
		updated = append(updated, s.lastEmails[i])
	}
	return updated
}

func normalizeIDs(ids []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func destructiveAction(action domain.BulkAction) bool {
	return action == domain.ActionTrash || action == domain.ActionUnsubscribe
}

func requiresConfirmation(results []domain.ActionResult) bool {
	for _, result := range results {
		if result.Status == "needs_confirmation" {
			return true
		}
	}
	return false
}

func successfulActionIDs(results []domain.ActionResult, status string) []string {
	ids := []string{}
	for _, result := range results {
		if result.Status == status && strings.TrimSpace(result.EmailID) != "" {
			ids = append(ids, result.EmailID)
		}
	}
	return ids
}

func (s *Server) createConfirmation(action domain.BulkAction, ids []string) (string, time.Time) {
	token := randomState()
	expiresAt := time.Now().UTC().Add(confirmationTTL)
	s.confirmMu.Lock()
	defer s.confirmMu.Unlock()
	if s.confirmations == nil {
		s.confirmations = map[string]pendingConfirmation{}
	}
	s.cleanupConfirmationsLocked(time.Now().UTC())
	if len(s.confirmations) >= 128 {
		for key := range s.confirmations {
			delete(s.confirmations, key)
			break
		}
	}
	s.confirmations[token] = pendingConfirmation{
		Action:    action,
		IDs:       append([]string(nil), ids...),
		ExpiresAt: expiresAt,
	}
	return token, expiresAt
}

func (s *Server) consumeConfirmation(token string, action domain.BulkAction, ids []string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	s.confirmMu.Lock()
	defer s.confirmMu.Unlock()
	confirmation, ok := s.confirmations[token]
	if !ok {
		return false
	}
	delete(s.confirmations, token)
	if time.Now().UTC().After(confirmation.ExpiresAt) {
		return false
	}
	if confirmation.Action != action || len(confirmation.IDs) != len(ids) {
		return false
	}
	for i := range ids {
		if confirmation.IDs[i] != ids[i] {
			return false
		}
	}
	return true
}

func (s *Server) cleanupConfirmationsLocked(now time.Time) {
	for token, confirmation := range s.confirmations {
		if now.After(confirmation.ExpiresAt) {
			delete(s.confirmations, token)
		}
	}
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
		if mutatingMethod(r.Method) && !allowedLocalOrigin(r) {
			writeError(w, http.StatusForbidden, "cross-origin mutating requests are blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func allowedLocalOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		origin = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	requestHost, requestPort := splitHostPort(r.Host)
	originHost, originPort := splitHostPort(parsed.Host)
	if strings.EqualFold(originHost, requestHost) && originPort == requestPort {
		return true
	}
	return requestPort == originPort && loopbackHost(requestHost) && loopbackHost(originHost)
}

func splitHostPort(value string) (string, string) {
	value = strings.TrimSpace(value)
	host, port, err := net.SplitHostPort(value)
	if err == nil {
		return strings.Trim(host, "[]"), port
	}
	if strings.Contains(value, ":") && strings.Count(value, ":") == 1 {
		parts := strings.SplitN(value, ":", 2)
		return strings.Trim(parts[0], "[]"), parts[1]
	}
	return strings.Trim(value, "[]"), ""
}

func loopbackHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
