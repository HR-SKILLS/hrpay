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

// Transaction represents the core Payment/Disbursement transaction model.
type Transaction struct {
	Reference    string    `json:"reference"`
	Status       string    `json:"status"` // PENDING, SUCCESS, FAILED, HOLD, REFUNDED
	Direction    string    `json:"direction"` // CASHIN, CASHOUT
	Amount       float64   `json:"amount"`
	Currency     Currency  `json:"currency"`
	Operator     Operator  `json:"operator"`
	PhoneNumber  string    `json:"phone_number,omitempty"`
	Description  string    `json:"description,omitempty"`
	Fees         float64   `json:"fees,omitempty"`
	NetAmount    float64   `json:"net_amount,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
