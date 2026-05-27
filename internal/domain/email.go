package domain

import "time"

type Category string

const (
	CategoryNeedsReview Category = "needs_review"
	CategoryPromotions  Category = "promotions"
	CategoryNewsletters Category = "newsletters"
	CategorySocial      Category = "social"
	CategoryFinance     Category = "finance"
	CategoryTravel      Category = "travel"
	CategoryWork        Category = "work"
	CategoryReceipts    Category = "receipts"
	CategorySecurity    Category = "security"
	CategoryPersonal    Category = "personal"
	CategoryUnwanted    Category = "unwanted"
)

type EmailSummary struct {
	ID                 string    `json:"id"`
	ThreadID           string    `json:"threadId"`
	From               string    `json:"from"`
	Subject            string    `json:"subject"`
	Snippet            string    `json:"snippet"`
	ReceivedAt         time.Time `json:"receivedAt"`
	LabelIDs           []string  `json:"labelIds"`
	Category           Category  `json:"category"`
	Confidence         float64   `json:"confidence"`
	Reason             string    `json:"reason"`
	HasUnsubscribe     bool      `json:"hasUnsubscribe"`
	UnsubscribeTarget  string    `json:"unsubscribeTarget,omitempty"`
	UnsubscribeMethod  string    `json:"unsubscribeMethod,omitempty"`
	CanAutoUnsubscribe bool      `json:"canAutoUnsubscribe"`
}

type Classification struct {
	EmailID    string   `json:"emailId"`
	Category   Category `json:"category"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
}

type BulkAction string

const (
	ActionTrash       BulkAction = "trash"
	ActionMarkRead    BulkAction = "mark_read"
	ActionUnsubscribe BulkAction = "unsubscribe"
)

type ActionResult struct {
	EmailID  string `json:"emailId"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	SafeLink string `json:"safeLink,omitempty"`
}
