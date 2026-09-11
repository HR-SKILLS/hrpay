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
	Country     Country                `json:"country"`
	Reference   string                 `json:"reference,omitempty"`
	Currency    Currency               `json:"currency,omitempty"`
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
	if p.Country == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Country", Message: "Country is required"}}}
	}
	if p.Amount < MinCashInAmount {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be at least 100"}}}
	}
	return nil
}

type CashInService struct {
	client *Client
}

// MobileMoney collects a payment from a customer's mobile money account.
// Country is required and is never defaulted by the SDK: currency is always
// derived from the country server-side, so the country must be explicit and
// correct. Returns a *Transaction with Status "SUCCESS" (HTTP 200) or
// "PENDING" (HTTP 202, awaiting the customer's on-phone confirmation) — poll
// Transactions.Status or Transactions.Poll, or wait for the payment.succeeded
// / payment.failed webhook, to learn the final outcome of a PENDING cashin.
func (s *CashInService) MobileMoney(ctx context.Context, params CashInMobileMoneyParams) (*Transaction, error) {
	if err := params.Validate(); err != nil {
		return nil, err
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
	if params.Reference != "" {
		payload["reference"] = params.Reference
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

	var envelope ApiResponse[Transaction]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	// Fallback to direct mapping
	var direct Transaction
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	if direct.Reference == "" {
		return nil, errors.New("[hrpay] Invalid CashIn response: missing reference")
	}
	return &direct, nil
}
