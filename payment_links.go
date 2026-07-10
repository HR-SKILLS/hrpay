package hrpay

import (
	"context"
	"encoding/json"
	"strconv"
)

type PaymentLinkCreateParams struct {
	Amount      float64  `json:"amount"`
	Currency    Currency `json:"currency,omitempty"`
	Description string   `json:"description"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
}

func (p *PaymentLinkCreateParams) Validate() error {
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	if p.Description == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Description", Message: "Description is required"}}}
	}
	return nil
}

type PaymentLink struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Description string  `json:"description"`
	Status      string  `json:"status"` // ACTIVE, EXPIRED, PAID
	ExpiresAt   string  `json:"expires_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type PaymentLinkListParams struct {
	Page  int `json:"page,omitempty"`
	Limit int `json:"limit,omitempty"`
}

type PaymentLinksService struct {
	client *Client
}

// Create generates a shareable payment link.
func (s *PaymentLinksService) Create(ctx context.Context, params PaymentLinkCreateParams) (*PaymentLink, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	if params.Currency == "" {
		params.Currency = CurrencyXaf
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathPaymentLinks, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[PaymentLink]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct PaymentLink
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// List all payment links.
func (s *PaymentLinksService) List(ctx context.Context, params PaymentLinkListParams) (*PaginatedResponse[PaymentLink], error) {
	queryParams := make(map[string]string)
	if params.Page > 0 {
		queryParams["page"] = strconv.Itoa(params.Page)
	}
	if params.Limit > 0 {
		queryParams["limit"] = strconv.Itoa(params.Limit)
	}

	respBody, err := s.client.request(ctx, "GET", PathPaymentLinks, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct PaginatedResponse[PaymentLink]
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
