package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type TransactionListParams struct {
	Status   string   `json:"status,omitempty"`
	Type     string   `json:"type,omitempty"`
	Operator Operator `json:"operator,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Page     int      `json:"page,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type PollOptions struct {
	Interval    time.Duration
	Timeout     time.Duration
	MaxAttempts int
	OnStatus    func(status string, attempt int)
}

type TransactionsService struct {
	client *Client
}

// Status returns payment status by reference.
// Endpoint: /v1/payments/:reference
func (s *TransactionsService) Status(ctx context.Context, reference string) (*Transaction, error) {
	if reference == "" {
		return nil, errors.New("[hrpay] Reference is required")
	}

	path := fmt.Sprintf("%s/%s", PathPaymentStatus, reference)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
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
	return &direct, nil
}

// Refund reverses a completed CASHIN transaction by reference. Requires an
// Idempotency-Key (auto-generated unless you attach one via
// WithIdempotencyKey for a safe client-side retry). Triggers the
// payment.refunded webhook on success.
// Endpoint: POST /v1/payments/:reference/refund
func (s *TransactionsService) Refund(ctx context.Context, reference string) (*Transaction, error) {
	if reference == "" {
		return nil, errors.New("[hrpay] Reference is required")
	}

	path := fmt.Sprintf("%s/%s/refund", PathPaymentStatus, reference)
	respBody, err := s.client.request(ctx, "POST", path, nil, nil)
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
	return &direct, nil
}

// Get returns transaction details by reference.
// Endpoint: /v1/transactions/:reference
func (s *TransactionsService) Get(ctx context.Context, reference string) (*Transaction, error) {
	if reference == "" {
		return nil, errors.New("[hrpay] Reference is required")
	}

	path := fmt.Sprintf("%s/%s", PathTransactionDetail, reference)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
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
	return &direct, nil
}

// List transactions.
// Endpoint: /v1/transactions
func (s *TransactionsService) List(ctx context.Context, params TransactionListParams) (*PaginatedResponse[Transaction], error) {
	if params.Limit > MaxPageLimit {
		return nil, &ValidationError{Issues: []ValidationIssue{{Field: "Limit", Message: "Limit must not exceed 100"}}}
	}

	queryParams := make(map[string]string)

	if params.Status != "" {
		queryParams["status"] = params.Status
	}
	if params.Type != "" {
		queryParams["type"] = params.Type
	}
	if params.Operator != "" {
		queryParams["operator"] = string(params.Operator)
	}
	if params.From != "" {
		queryParams["from"] = params.From
	}
	if params.To != "" {
		queryParams["to"] = params.To
	}
	if params.Page > 0 {
		queryParams["page"] = strconv.Itoa(params.Page)
	}
	if params.Limit > 0 {
		queryParams["limit"] = strconv.Itoa(params.Limit)
	}

	respBody, err := s.client.request(ctx, "GET", PathTransactionsList, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct PaginatedResponse[Transaction]
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Poll fetches a transaction periodically until SUCCESS, FAILED, or REFUNDED status is reached.
func (s *TransactionsService) Poll(ctx context.Context, reference string, opts PollOptions) (*Transaction, error) {
	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultPollTimeout
	}

	deadline := time.Now().Add(timeout)
	attempt := 0

	for {
		attempt++

		tx, err := s.Status(ctx, reference)
		if err != nil {
			return nil, err
		}

		if opts.OnStatus != nil {
			opts.OnStatus(tx.Status, attempt)
		}

		if TerminalStatuses[tx.Status] {
			return tx, nil
		}

		if opts.MaxAttempts > 0 && attempt >= opts.MaxAttempts {
			return nil, &ApiError{SDKError: SDKError{
				StatusCode: 408,
				Code:       "POLL_MAX_ATTEMPTS_REACHED",
				Message:    fmt.Sprintf("Polling stopped after %d attempts. Last status: %s", attempt, tx.Status),
			}}
		}

		if time.Now().Add(interval).After(deadline) {
			return nil, &ApiError{SDKError: SDKError{
				StatusCode: 408,
				Code:       "POLL_TIMEOUT",
				Message:    fmt.Sprintf("Polling timed out after %v. Last status: %s", timeout, tx.Status),
			}}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
