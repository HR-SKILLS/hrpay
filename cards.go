package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type VirtualCardCreateParams struct {
	Label    string   `json:"label"`
	Currency Currency `json:"currency,omitempty"`
	Amount   float64  `json:"amount"`
}

func (p *VirtualCardCreateParams) Validate() error {
	if p.Label == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Label", Message: "Label is required"}}}
	}
	if p.Amount < 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Initial amount must be non-negative"}}}
	}
	return nil
}

type VirtualCard struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Currency  string `json:"currency"`
	Balance   float64 `json:"balance"`
	Status    string `json:"status"` // ACTIVE, FROZEN, CANCELLED
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type VirtualCardsService struct {
	client *Client
}

// Create generates a virtual card.
func (s *VirtualCardsService) Create(ctx context.Context, params VirtualCardCreateParams) (*VirtualCard, error) {
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

	respBody, err := s.client.request(ctx, "POST", PathVirtualCards, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[VirtualCard]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct VirtualCard
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// List all virtual cards.
func (s *VirtualCardsService) List(ctx context.Context) (*PaginatedResponse[VirtualCard], error) {
	respBody, err := s.client.request(ctx, "GET", PathVirtualCards, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct PaginatedResponse[VirtualCard]
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Get details of a virtual card.
func (s *VirtualCardsService) Get(ctx context.Context, cardID string) (*VirtualCard, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	path := fmt.Sprintf("%s/%s", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[VirtualCard]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct VirtualCard
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Topup adds money from wallet to card.
func (s *VirtualCardsService) Topup(ctx context.Context, cardID string, amount float64) (*VirtualCard, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}
	if amount <= 0 {
		return nil, &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Topup amount must be positive"}}}
	}

	path := fmt.Sprintf("%s/%s/topup", PathVirtualCards, cardID)
	payload := map[string]float64{
		"amount": amount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", path, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[VirtualCard]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct VirtualCard
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Freeze blocks transaction on card.
func (s *VirtualCardsService) Freeze(ctx context.Context, cardID string) (*VirtualCard, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	path := fmt.Sprintf("%s/%s/freeze", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "POST", path, []byte("{}"), nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[VirtualCard]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct VirtualCard
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Unfreeze unblocks the card.
func (s *VirtualCardsService) Unfreeze(ctx context.Context, cardID string) (*VirtualCard, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	path := fmt.Sprintf("%s/%s/unfreeze", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "POST", path, []byte("{}"), nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[VirtualCard]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct VirtualCard
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Cancel permanently destroys the card.
func (s *VirtualCardsService) Cancel(ctx context.Context, cardID string) (*VirtualCard, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	path := fmt.Sprintf("%s/%s/cancel", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "POST", path, []byte("{}"), nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[VirtualCard]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct VirtualCard
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
