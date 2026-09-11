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
	Country     Country  `json:"country"`
	Reference   string   `json:"reference,omitempty"`
	Currency    Currency `json:"currency,omitempty"`
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
	if p.Country == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Country", Message: "Country is required"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return nil
}

type CashOutService struct {
	client *Client
}

// MobileMoney disburses a payment from the merchant wallet to a beneficiary's
// mobile money account. Country is required and never defaulted by the SDK
// (see CashInService.MobileMoney). Requires an existing wallet in the
// resulting currency (WALLET_NOT_FOUND otherwise, HTTP 402) — a merchant with
// no prior cashin in that currency must fund the wallet first.
func (s *CashOutService) MobileMoney(ctx context.Context, params CashOutMobileMoneyParams) (*Transaction, error) {
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

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathCashOutMobileMoney, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[Transaction]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct Transaction
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	if direct.Reference == "" {
		return nil, errors.New("[hrpay] Invalid CashOut response: missing reference")
	}
	return &direct, nil
}
