package hrpay

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
)

type AirtimeRechargeParams struct {
	Operator Operator `json:"operator"`
	Phone    string   `json:"phone"`
	Amount   float64  `json:"amount"`
}

func (p *AirtimeRechargeParams) Validate() error {
	if p.Operator == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Operator", Message: "Operator is required"}}}
	}
	if p.Phone == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Phone", Message: "Phone is required"}}}
	}
	matched, _ := regexp.MatchString(`^\d+$`, p.Phone)
	if len(p.Phone) < 9 || !matched {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Phone", Message: "Phone must be at least 9 digits and contain only digits"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return nil
}

type AirtimeBatchItem struct {
	Phone    string   `json:"phone"`
	Operator Operator `json:"operator"`
	Amount   float64  `json:"amount"`
}

type AirtimeBatchParams struct {
	Items []AirtimeBatchItem `json:"items"`
}

func (p *AirtimeBatchParams) Validate() error {
	if len(p.Items) == 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Items", Message: "At least one item required"}}}
	}
	if len(p.Items) > 500 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Items", Message: "Maximum 500 items per batch"}}}
	}
	for i, item := range p.Items {
		if item.Operator == "" {
			return &ValidationError{Issues: []ValidationIssue{{Field: fmt.Sprintf("Items[%d].Operator", i), Message: "Operator is required"}}}
		}
		if item.Phone == "" {
			return &ValidationError{Issues: []ValidationIssue{{Field: fmt.Sprintf("Items[%d].Phone", i), Message: "Phone is required"}}}
		}
		matched, _ := regexp.MatchString(`^\d+$`, item.Phone)
		if len(item.Phone) < 9 || !matched {
			return &ValidationError{Issues: []ValidationIssue{{Field: fmt.Sprintf("Items[%d].Phone", i), Message: "Phone must be at least 9 digits and contain only digits"}}}
		}
		if item.Amount <= 0 {
			return &ValidationError{Issues: []ValidationIssue{{Field: fmt.Sprintf("Items[%d].Amount", i), Message: "Amount must be positive"}}}
		}
	}
	return nil
}

type AirtimeRechargeResponse struct {
	Reference  string  `json:"reference"`
	Status     string  `json:"status"`
	Phone      string  `json:"phone"`
	Operator   string  `json:"operator"`
	Amount     float64 `json:"amount"`
	Commission float64 `json:"commission"`
	CreatedAt  string  `json:"created_at"`
}

type AirtimeBatchItemResult struct {
	Phone     string  `json:"phone"`
	Operator  string  `json:"operator"`
	Amount    float64 `json:"amount"`
	Status    string  `json:"status"` // success, failed
	Reference string  `json:"reference,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type AirtimeBatchResponse struct {
	BatchID string                   `json:"batch_id"`
	Total   int                      `json:"total"`
	Success int                      `json:"success"`
	Failed  int                      `json:"failed"`
	Items   []AirtimeBatchItemResult `json:"items"`
}

type AirtimeOffer struct {
	Operator       string  `json:"operator"`
	Name           string  `json:"name"`
	MinAmount      float64 `json:"min_amount"`
	MaxAmount      float64 `json:"max_amount"`
	Currency       string  `json:"currency"`
	CommissionRate float64 `json:"commission_rate"`
}

type AirtimeService struct {
	client *Client
}

// Recharge single recipient.
func (s *AirtimeService) Recharge(ctx context.Context, params AirtimeRechargeParams) (*AirtimeRechargeResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathAirtimeRecharge, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[AirtimeRechargeResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct AirtimeRechargeResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Batch recharges up to 500 recipients.
func (s *AirtimeService) Batch(ctx context.Context, params AirtimeBatchParams) (*AirtimeBatchResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathAirtimeBatch, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[AirtimeBatchResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct AirtimeBatchResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Offers lists operators.
func (s *AirtimeService) Offers(ctx context.Context) ([]AirtimeOffer, error) {
	respBody, err := s.client.request(ctx, "GET", PathAirtimeOffers, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[[]AirtimeOffer]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return envelope.Data, nil
	}

	var direct []AirtimeOffer
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return direct, nil
}
