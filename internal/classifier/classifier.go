package classifier

import (
	"context"
	"strings"

	"github.com/schulerj89/gmail-organizer/internal/domain"
)

type Classifier interface {
	Classify(ctx context.Context, emails []domain.EmailSummary) ([]domain.Classification, error)
}

type HeuristicClassifier struct{}

func NewHeuristicClassifier() HeuristicClassifier {
	return HeuristicClassifier{}
}

func (h HeuristicClassifier) Classify(_ context.Context, emails []domain.EmailSummary) ([]domain.Classification, error) {
	results := make([]domain.Classification, 0, len(emails))
	for _, email := range emails {
		results = append(results, classifyOne(email))
	}
	return results, nil
}

func classifyOne(email domain.EmailSummary) domain.Classification {
	text := strings.ToLower(email.From + " " + email.Subject + " " + email.Snippet)
	category := domain.CategoryNeedsReview
	confidence := 0.46
	reason := "No strong local signal found."

	switch {
	case containsAny(text, "unsubscribe", "sale", "deal", "discount", "offer", "coupon", "promotion", "shop", "cart"):
		category, confidence, reason = domain.CategoryPromotions, 0.76, "Promotional language or unsubscribe signal."
	case containsAny(text, "newsletter", "digest", "weekly", "roundup", "edition"):
		category, confidence, reason = domain.CategoryNewsletters, 0.78, "Newsletter or digest wording."
	case containsAny(text, "password", "security", "sign-in", "login", "verification", "2fa", "alert"):
		category, confidence, reason = domain.CategorySecurity, 0.82, "Security or authentication signal."
	case containsAny(text, "receipt", "invoice", "payment", "order", "shipped", "delivery", "statement"):
		category, confidence, reason = domain.CategoryReceipts, 0.80, "Transaction or receipt wording."
	case containsAny(text, "bank", "credit", "debit", "loan", "mortgage", "investment", "brokerage"):
		category, confidence, reason = domain.CategoryFinance, 0.74, "Finance-related sender or subject."
	case containsAny(text, "flight", "hotel", "reservation", "booking", "trip", "itinerary"):
		category, confidence, reason = domain.CategoryTravel, 0.78, "Travel reservation signal."
	case containsAny(text, "linkedin", "facebook", "instagram", "x.com", "twitter", "notification"):
		category, confidence, reason = domain.CategorySocial, 0.73, "Social network notification signal."
	case containsAny(text, "meeting", "calendar", "project", "ticket", "pull request", "deadline"):
		category, confidence, reason = domain.CategoryWork, 0.68, "Work coordination wording."
	case containsAny(text, "no-reply", "noreply") && email.HasUnsubscribe:
		category, confidence, reason = domain.CategoryUnwanted, 0.69, "Automated sender with unsubscribe header."
	}

	return domain.Classification{
		EmailID:    email.ID,
		Category:   category,
		Confidence: confidence,
		Reason:     reason,
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
