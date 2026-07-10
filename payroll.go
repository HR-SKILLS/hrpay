package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type PayrollRecipient struct {
	PhoneNumber string   `json:"phone_number"`
	Operator    Operator `json:"operator"`
	Amount      float64  `json:"amount"`
	Name        string   `json:"name,omitempty"`
}

type PayrollImportParams struct {
	Label      string             `json:"label"`
	Currency   Currency           `json:"currency,omitempty"`
	Recipients []PayrollRecipient `json:"recipients,omitempty"`
	FileBase64 string             `json:"file_base64,omitempty"`
	CSVData    string             `json:"csv_data,omitempty"`
}

func (p *PayrollImportParams) Validate() error {
	if p.Label == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Label", Message: "Label is required"}}}
	}
	if len(p.Recipients) == 0 && p.FileBase64 == "" && p.CSVData == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Recipients", Message: "At least one of 'recipients', 'file_base64', or 'csv_data' must be provided"}}}
	}
	for i, r := range p.Recipients {
		if r.PhoneNumber == "" {
			return &ValidationError{Issues: []ValidationIssue{{Field: fmt.Sprintf("Recipients[%d].PhoneNumber", i), Message: "PhoneNumber is required"}}}
		}
		if r.Operator == "" {
			return &ValidationError{Issues: []ValidationIssue{{Field: fmt.Sprintf("Recipients[%d].Operator", i), Message: "Operator is required"}}}
		}
		if r.Amount <= 0 {
			return &ValidationError{Issues: []ValidationIssue{{Field: fmt.Sprintf("Recipients[%d].Amount", i), Message: "Amount must be positive"}}}
		}
	}
	return nil
}

type PayrollImportResponse struct {
	BatchID string `json:"batch_id"`
}

type PayrollBatch struct {
	BatchID         string   `json:"batch_id"`
	Label           string   `json:"label"`
	Currency        string   `json:"currency"`
	Status          string   `json:"status"` // DRAFT, PENDING, PROCESSING, COMPLETED, PARTIALLY_COMPLETED, FAILED
	TotalRecipients int      `json:"total_recipients"`
	TotalAmount     float64  `json:"total_amount"`
	SuccessCount    int      `json:"success_count,omitempty"`
	FailedCount     int      `json:"failed_count,omitempty"`
	CreatedAt       string   `json:"created_at"`
	ExecutedAt      string   `json:"executed_at,omitempty"`
}

type PayrollReportItem struct {
	PhoneNumber string `json:"phone_number"`
	Name        string `json:"name,omitempty"`
	Operator    string `json:"operator"`
	Amount      float64 `json:"amount"`
	Status      string `json:"status"` // SUCCESS, FAILED
	Reference   string `json:"reference,omitempty"`
	Error       string `json:"error,omitempty"`
}

type PayrollReport struct {
	BatchID string `json:"batch_id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Summary struct {
		Total       int     `json:"total"`
		Success     int     `json:"success"`
		Failed      int     `json:"failed"`
		TotalAmount float64 `json:"total_amount"`
		TotalFees   float64 `json:"total_fees"`
	} `json:"summary"`
	Items []PayrollReportItem `json:"items"`
}

type PayrollListParams struct {
	Page  int `json:"page,omitempty"`
	Limit int `json:"limit,omitempty"`
}

type PayrollService struct {
	client *Client
}

// Import creates a draft payroll batch.
func (s *PayrollService) Import(ctx context.Context, params PayrollImportParams) (*PayrollImportResponse, error) {
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

	respBody, err := s.client.request(ctx, "POST", PathPayrollImport, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[PayrollImportResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct PayrollImportResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Execute triggers disbursements for an imported batch.
func (s *PayrollService) Execute(ctx context.Context, batchID string) (*PayrollBatch, error) {
	if batchID == "" {
		return nil, errors.New("[hrpay] Batch ID is required")
	}

	path := fmt.Sprintf("%s/%s/execute", PathPayrollBatch, batchID)
	respBody, err := s.client.request(ctx, "POST", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[PayrollBatch]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct PayrollBatch
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Status checks payroll status.
func (s *PayrollService) Status(ctx context.Context, batchID string) (*PayrollBatch, error) {
	if batchID == "" {
		return nil, errors.New("[hrpay] Batch ID is required")
	}

	path := fmt.Sprintf("%s/%s", PathPayrollBatch, batchID)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[PayrollBatch]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct PayrollBatch
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Report details per-recipient results of a batch.
func (s *PayrollService) Report(ctx context.Context, batchID string) (*PayrollReport, error) {
	if batchID == "" {
		return nil, errors.New("[hrpay] Batch ID is required")
	}

	path := fmt.Sprintf("%s/%s/report", PathPayrollBatch, batchID)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[PayrollReport]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct PayrollReport
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// List all batches.
func (s *PayrollService) List(ctx context.Context, params PayrollListParams) (*PaginatedResponse[PayrollBatch], error) {
	queryParams := make(map[string]string)
	if params.Page > 0 {
		queryParams["page"] = strconv.Itoa(params.Page)
	}
	if params.Limit > 0 {
		queryParams["limit"] = strconv.Itoa(params.Limit)
	}

	respBody, err := s.client.request(ctx, "GET", PathPayrollBatches, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct PaginatedResponse[PayrollBatch]
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
