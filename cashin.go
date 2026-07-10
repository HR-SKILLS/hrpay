package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

type CashInMobileMoneyParams struct {
	PhoneNumber string                 `json:"phone_number"`
	Operator    Operator               `json:"operator"`
	Amount      float64                `json:"amount"`
	Currency    Currency               `json:"currency,omitempty"`
	Country     Country                `json:"country,omitempty"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (p *CashInMobileMoneyParams) Validate() error {
	if p.PhoneNumber == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "PhoneNumber", Message: "Phone number is required"}}}
	}
	matched, _ := regexp.MatchString(`^\d+$`, p.PhoneNumber)
	if len(p.PhoneNumber) < 9 || !matched {
		return &ValidationError{Issues: []ValidationIssue{{Field: "PhoneNumber", Message: "Phone number must be at least 9 digits and contain only digits"}}}
	}
	if p.Operator == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Operator", Message: "Operator is required"}}}
	}
	if p.Amount < MinCashInAmount {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be at least 100"}}}
	}
	return nil
}

type CashInInitiateParams struct {
	Direction   string   `json:"direction"`
	Operator    Operator `json:"operator"`
	Country     Country  `json:"country,omitempty"`
	PhoneNumber string   `json:"phone_number"`
	Amount      float64  `json:"amount"`
	Currency    Currency `json:"currency,omitempty"`
}

func (p *CashInInitiateParams) Validate() error {
	if p.Direction != "CASHIN" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Direction", Message: "Direction must be CASHIN"}}}
	}
	if p.PhoneNumber == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "PhoneNumber", Message: "Phone number is required"}}}
	}
	if len(p.PhoneNumber) < 9 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "PhoneNumber", Message: "Phone number must be at least 9 digits"}}}
	}
	if p.Operator == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Operator", Message: "Operator is required"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return nil
}

type CashInResponse struct {
	TransactionID string  `json:"transaction_id"`
	Reference     string  `json:"reference"`
	Status        string  `json:"status"`
	Type          string  `json:"type"` // CASHIN
	Amount        float64 `json:"amount"`
	Fee           float64 `json:"fee"`
	FeePercent    float64 `json:"fee_percent"`
	NetAmount     float64 `json:"net_amount"`
	Currency      string  `json:"currency"`
	Operator      string  `json:"operator"`
	PhoneNumber   string  `json:"phone_number"`
	InitiatedAt   string  `json:"initiated_at"`
	PendingAction string  `json:"pending_action"`
}

type CashInService struct {
	client *Client
}

func (s *CashInService) MobileMoney(ctx context.Context, params CashInMobileMoneyParams) (*CashInResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	if params.Country == "" {
		params.Country = CountryCm
	}
	if params.Currency == "" {
		params.Currency = currencyForCountry(params.Country)
	}

	// Format payload for remote API
	payload := map[string]interface{}{
		"operator":     params.Operator,
		"country":      params.Country,
		"phone_number": params.PhoneNumber,
		"amount":       params.Amount,
		"currency":     params.Currency,
	}
	if params.Description != "" {
		payload["description"] = params.Description
	}
	if params.Metadata != nil {
		payload["metadata"] = params.Metadata
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathCashInMobileMoney, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CashInResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	// Fallback to direct mapping
	var direct CashInResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	if direct.Reference == "" {
		return nil, errors.New("[hrpay] Invalid CashIn response: missing reference")
	}
	return &direct, nil
}

func (s *CashInService) Initiate(ctx context.Context, params CashInInitiateParams) (*CashInResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	if params.Country == "" {
		params.Country = CountryCm
	}
	if params.Currency == "" {
		params.Currency = currencyForCountry(params.Country)
	}

	payload := map[string]interface{}{
		"direction":    params.Direction,
		"operator":     params.Operator,
		"country":      params.Country,
		"phone_number": params.PhoneNumber,
		"amount":       params.Amount,
		"currency":     params.Currency,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathCashInInitiate, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CashInResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CashInResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
