package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schulerj89/gmail-organizer/internal/domain"
	"github.com/schulerj89/gmail-organizer/internal/secrets"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type Service struct {
	clientSecret secrets.FileSecret
	tokenPath    string
	config       *oauth2.Config
}

func NewService(ctx context.Context, clientSecret secrets.FileSecret, dataDir string, redirectURL string) (*Service, error) {
	content, err := os.ReadFile(clientSecret.Path)
	if err != nil {
		return nil, err
	}
	oauthConfig, err := google.ConfigFromJSON(content, gmailapi.GmailModifyScope)
	if err != nil {
		return nil, err
	}
	oauthConfig.RedirectURL = redirectURL
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	_ = ctx
	return &Service{
		clientSecret: clientSecret,
		tokenPath:    filepath.Join(dataDir, "gmail_token.json"),
		config:       oauthConfig,
	}, nil
}

func (s *Service) Authenticated() bool {
	_, err := s.loadToken()
	return err == nil
}

func (s *Service) AuthURL(state string) string {
	return s.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

func (s *Service) Exchange(ctx context.Context, code string) error {
	token, err := s.config.Exchange(ctx, code)
	if err != nil {
		return err
	}
	return s.saveToken(token)
}

func (s *Service) List(ctx context.Context, query string, max int64) ([]domain.EmailSummary, error) {
	client, err := s.authorizedClient(ctx)
	if err != nil {
		return nil, err
	}
	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	if max <= 0 || max > 200 {
		max = 50
	}
	if strings.TrimSpace(query) == "" {
		query = "newer_than:365d"
	}

	listCall := service.Users.Messages.List("me").Q(query).MaxResults(max)
	resp, err := listCall.Do()
	if err != nil {
		return nil, err
	}

	emails := make([]domain.EmailSummary, 0, len(resp.Messages))
	for _, message := range resp.Messages {
		item, err := service.Users.Messages.Get("me", message.Id).
			Format("metadata").
			MetadataHeaders("From", "Subject", "Date", "List-Unsubscribe").
			Do()
		if err != nil {
			return nil, err
		}
		emails = append(emails, toEmailSummary(item))
	}
	return emails, nil
}

func (s *Service) Trash(ctx context.Context, ids []string) ([]domain.ActionResult, error) {
	client, err := s.authorizedClient(ctx)
	if err != nil {
		return nil, err
	}
	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	results := make([]domain.ActionResult, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		_, err := service.Users.Messages.Trash("me", id).Do()
		if err != nil {
			results = append(results, domain.ActionResult{EmailID: id, Status: "failed", Message: err.Error()})
			continue
		}
		results = append(results, domain.ActionResult{EmailID: id, Status: "trashed"})
	}
	return results, nil
}

func (s *Service) MarkRead(ctx context.Context, ids []string) ([]domain.ActionResult, error) {
	client, err := s.authorizedClient(ctx)
	if err != nil {
		return nil, err
	}
	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	results := make([]domain.ActionResult, 0, len(ids))
	for _, id := range ids {
		_, err := service.Users.Messages.Modify("me", id, &gmailapi.ModifyMessageRequest{RemoveLabelIds: []string{"UNREAD"}}).Do()
		if err != nil {
			results = append(results, domain.ActionResult{EmailID: id, Status: "failed", Message: err.Error()})
			continue
		}
		results = append(results, domain.ActionResult{EmailID: id, Status: "marked_read"})
	}
	return results, nil
}

func (s *Service) authorizedClient(ctx context.Context) (*http.Client, error) {
	token, err := s.loadToken()
	if err != nil {
		return nil, errors.New("gmail is not authenticated; visit /api/auth/google/url first")
	}
	return s.config.Client(ctx, token), nil
}

func (s *Service) loadToken() (*oauth2.Token, error) {
	raw, err := os.ReadFile(s.tokenPath)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *Service) saveToken(token *oauth2.Token) error {
	raw, err := json.Marshal(token)
	if err != nil {
		return err
	}
	return os.WriteFile(s.tokenPath, raw, 0o600)
}

func toEmailSummary(message *gmailapi.Message) domain.EmailSummary {
	headers := map[string]string{}
	if message.Payload != nil {
		for _, header := range message.Payload.Headers {
			headers[strings.ToLower(header.Name)] = header.Value
		}
	}
	receivedAt := time.UnixMilli(message.InternalDate)
	unsubscribe := firstUnsubscribeTarget(headers["list-unsubscribe"])
	return domain.EmailSummary{
		ID:                message.Id,
		ThreadID:          message.ThreadId,
		From:              headers["from"],
		Subject:           headers["subject"],
		Snippet:           message.Snippet,
		ReceivedAt:        receivedAt,
		LabelIDs:          message.LabelIds,
		Category:          domain.CategoryNeedsReview,
		Confidence:        0,
		Reason:            "Not classified yet.",
		HasUnsubscribe:    unsubscribe != "",
		UnsubscribeTarget: unsubscribe,
	}
}

func firstUnsubscribeTarget(header string) string {
	parts := strings.Split(header, ",")
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		candidate = strings.TrimPrefix(candidate, "<")
		candidate = strings.TrimSuffix(candidate, ">")
		if strings.HasPrefix(candidate, "https://") || strings.HasPrefix(candidate, "mailto:") {
			return candidate
		}
	}
	return ""
}

func DemoEmails() []domain.EmailSummary {
	now := time.Now()
	return []domain.EmailSummary{
		{ID: "demo-1", ThreadID: "demo-1", From: "deals@example.com", Subject: "Last chance: 40% off", Snippet: "Use this promotion before midnight.", ReceivedAt: now.Add(-2 * time.Hour), HasUnsubscribe: true, UnsubscribeTarget: "https://example.com/unsubscribe"},
		{ID: "demo-2", ThreadID: "demo-2", From: "alerts@bank.example", Subject: "Security alert for your account", Snippet: "A new sign-in was detected.", ReceivedAt: now.Add(-4 * time.Hour)},
		{ID: "demo-3", ThreadID: "demo-3", From: "news@example.com", Subject: "Weekly product digest", Snippet: "Your newsletter roundup is ready.", ReceivedAt: now.Add(-8 * time.Hour), HasUnsubscribe: true, UnsubscribeTarget: "mailto:unsubscribe@example.com"},
		{ID: "demo-4", ThreadID: "demo-4", From: "store@example.com", Subject: "Your receipt", Snippet: "Thanks for your order.", ReceivedAt: now.Add(-14 * time.Hour)},
	}
}

func UnsubscribeResults(emails []domain.EmailSummary, ids []string) []domain.ActionResult {
	index := map[string]domain.EmailSummary{}
	for _, email := range emails {
		index[email.ID] = email
	}
	results := make([]domain.ActionResult, 0, len(ids))
	for _, id := range ids {
		email, ok := index[id]
		if !ok || email.UnsubscribeTarget == "" {
			results = append(results, domain.ActionResult{EmailID: id, Status: "skipped", Message: "No unsubscribe header was available."})
			continue
		}
		results = append(results, domain.ActionResult{
			EmailID:  id,
			Status:   "prepared",
			Message:  "Review this unsubscribe target before opening it.",
			SafeLink: email.UnsubscribeTarget,
		})
	}
	return results
}

func (s *Service) String() string {
	return fmt.Sprintf("gmail token: %s", s.tokenPath)
}
