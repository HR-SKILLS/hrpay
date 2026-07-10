package hrpay

import (
	"context"
	"encoding/json"
	"regexp"
)

type DataSendParams struct {
	Operator Operator `json:"operator"`
	Phone    string   `json:"phone"`
	Amount   float64  `json:"amount"`
}

func (p *DataSendParams) Validate() error {
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

type DataPackage struct {
	ID             string  `json:"id"`
	Operator       string  `json:"operator"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Validity       string  `json:"validity"`
	CommissionRate float64 `json:"commission_rate"`
}

type DataSendResponse struct {
	Reference  string  `json:"reference"`
	Status     string  `json:"status"`
	Phone      string  `json:"phone"`
	Operator   string  `json:"operator"`
	Amount     float64 `json:"amount"`
	Commission float64 `json:"commission"`
	CreatedAt  string  `json:"created_at"`
}

type DataService struct {
	client *Client
}

// Packages lists packages for an operator.
func (s *DataService) Packages(ctx context.Context, operator string) ([]DataPackage, error) {
	params := make(map[string]string)
	if operator != "" {
		params["operator"] = operator
	}

	respBody, err := s.client.request(ctx, "GET", PathDataPackages, nil, params)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[[]DataPackage]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return envelope.Data, nil
	}

	var direct []DataPackage
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return direct, nil
}

// Send internet package to phone.
func (s *DataService) Send(ctx context.Context, params DataSendParams) (*DataSendResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathDataSend, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[DataSendResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct DataSendResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
