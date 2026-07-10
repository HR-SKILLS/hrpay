package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

type CashOutMobileMoneyParams struct {
	PhoneNumber string   `json:"phone_number"`
	Operator    Operator `json:"operator"`
	Amount      float64  `json:"amount"`
	Currency    Currency `json:"currency,omitempty"`
	Country     Country  `json:"country,omitempty"`
	Description string   `json:"description,omitempty"`
}

func (p *CashOutMobileMoneyParams) Validate() error {
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
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return nil
}

type CashOutResponse struct {
	TransactionID string  `json:"transaction_id"`
	Reference     string  `json:"reference"`
	Status        string  `json:"status"`
	Type          string  `json:"type"` // CASHOUT
	Amount        float64 `json:"amount"`
	Fee           float64 `json:"fee"`
	FeePercent    float64 `json:"fee_percent"`
	Currency      string  `json:"currency"`
	Operator      string  `json:"operator"`
	PhoneNumber   string  `json:"phone_number"`
	InitiatedAt   string  `json:"initiated_at"`
	PendingAction string  `json:"pending_action"`
}

type CashOutService struct {
	client *Client
}

func (s *CashOutService) MobileMoney(ctx context.Context, params CashOutMobileMoneyParams) (*CashOutResponse, error) {
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
		"operator":     params.Operator,
		"country":      params.Country,
		"phone_number": params.PhoneNumber,
		"amount":       params.Amount,
		"currency":     params.Currency,
	}

	if params.Description != "" {
		payload["description"] = params.Description
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathCashOutMobileMoney, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CashOutResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CashOutResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	if direct.Reference == "" {
		return nil, errors.New("[hrpay] Invalid CashOut response: missing reference")
	}
	return &direct, nil
}
