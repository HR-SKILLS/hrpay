package hrpay

import "time"

// PaginationParams represents query parameter page and limit.
type PaginationParams struct {
	Page  int `json:"page,omitempty"`
	Limit int `json:"limit,omitempty"`
}

// Meta represents pagination metadata in responses.
type Meta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Pages int `json:"pages"`
}

// PaginatedResponse wraps arrays of items with metadata.
type PaginatedResponse[T any] struct {
	Data []T  `json:"data"`
	Meta Meta `json:"meta"`
}

// ApiResponse standard envelope.
type ApiResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data"`
	Message string `json:"message,omitempty"`
}

// Transaction represents the core Payment/Disbursement transaction model,
// returned by CashIn.MobileMoney, CashOut.MobileMoney, Transactions.Status,
// Transactions.Get, Transactions.List, and Transactions.Refund.
type Transaction struct {
	TransactionID string   `json:"transaction_id,omitempty"`
	ExternalID    string   `json:"external_id,omitempty"`
	Reference     string   `json:"reference"`
	Status        string   `json:"status"` // PENDING, SUCCESS, FAILED, HOLD, REFUNDED
	Direction     string   `json:"type"`   // CASHIN, CASHOUT
	Amount        float64  `json:"amount"`
	Currency      Currency `json:"currency"`
	Country       Country  `json:"country,omitempty"`
	Operator      Operator `json:"operator"`
	PhoneNumber   string   `json:"phone_number,omitempty"`
	Description   string   `json:"description,omitempty"`
	Fees          float64  `json:"fee,omitempty"`
	FeePercent    float64  `json:"fee_percent,omitempty"`
	FeeFixed      float64  `json:"fee_fixed,omitempty"`
	NetAmount     float64  `json:"net_amount,omitempty"`

	OtpRequired           bool   `json:"otp_required,omitempty"`
	Provider              string `json:"provider,omitempty"`
	ProviderRef           string `json:"provider_ref,omitempty"`
	InitiatedAt           string `json:"initiated_at,omitempty"`
	CompletedAt           string `json:"completed_at,omitempty"`
	PendingAction         string `json:"pending_action,omitempty"`
	ErrorMessage          string `json:"error_message,omitempty"`
	ErrorCode             string `json:"error_code,omitempty"`
	FailureReasonCategory string `json:"failure_reason_category,omitempty"`

	WalletBalanceBefore float64 `json:"wallet_balance_before,omitempty"`
	WalletBalanceAfter  float64 `json:"wallet_balance_after,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
