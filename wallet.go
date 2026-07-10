package hrpay

import (
	"context"
	"encoding/json"
	"strconv"
)

// WalletHold is an amount credited by a Cash-In that stays unavailable
// until available_at (48h hold).
type WalletHold struct {
	Amount      float64 `json:"amount"`
	AvailableAt string  `json:"available_at"`
}

type WalletBalance struct {
	AccountType string `json:"account_type"`
	Balance     struct {
		Available float64 `json:"available"`
		Held      float64 `json:"held"`
		Total     float64 `json:"total"`
	} `json:"balance"`
	Holds []WalletHold `json:"holds,omitempty"`
	Currency    string `json:"currency"`
	Environment string `json:"environment"`
	IsFrozen    bool   `json:"is_frozen,omitempty"`
	FrozenReason string `json:"frozen_reason,omitempty"`
	HoldHours   int    `json:"hold_hours,omitempty"`
	Limits      *struct {
		DailyCashInLimit       float64 `json:"daily_cashin_limit"`
		DailyCashInRemaining   float64 `json:"daily_cashin_remaining"`
		DailyCashInUsed        float64 `json:"daily_cashin_used"`
		DailyCashOutLimit      float64 `json:"daily_cashout_limit"`
		DailyCashOutRemaining  float64 `json:"daily_cashout_remaining"`
		DailyCashOutUsed       float64 `json:"daily_cashout_used"`
		MaxSingleCashIn        float64 `json:"max_single_cashin"`
		MaxSingleCashOut       float64 `json:"max_single_cashout"`
		MinSingleCashIn        float64 `json:"min_single_cashin"`
		MinSingleCashOut       float64 `json:"min_single_cashout"`
		MonthlyCashInLimit     float64 `json:"monthly_cashin_limit"`
		MonthlyCashInUsed      float64 `json:"monthly_cashin_used"`
		MonthlyCashOutLimit    float64 `json:"monthly_cashout_limit"`
		MonthlyCashOutUsed     float64 `json:"monthly_cashout_used"`
	} `json:"limits,omitempty"`
	StatsToday *struct {
		CashInCount    int     `json:"cashin_count"`
		CashInVolume   float64 `json:"cashin_volume"`
		CashOutCount   int     `json:"cashout_count"`
		CashOutVolume  float64 `json:"cashout_volume"`
		FeesPaid       float64 `json:"fees_paid"`
	} `json:"stats_today,omitempty"`
}

type WalletMovement struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"` // CREDIT, DEBIT
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	Reference    string    `json:"reference,omitempty"`
	Description  string    `json:"description,omitempty"`
	BalanceAfter float64   `json:"balance_after"`
	CreatedAt    string    `json:"created_at"`
}

type WalletMovementsParams struct {
	Page  int    `json:"page,omitempty"`
	Limit int    `json:"limit,omitempty"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

type WalletService struct {
	client *Client
}

// Balance retrieves wallet balances and limits.
func (s *WalletService) Balance(ctx context.Context) (*WalletBalance, error) {
	respBody, err := s.client.request(ctx, "GET", PathWalletBalance, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[WalletBalance]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct WalletBalance
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Movements retrieves credit/debit movements for your wallet.
func (s *WalletService) Movements(ctx context.Context, params WalletMovementsParams) (*PaginatedResponse[WalletMovement], error) {
	queryParams := make(map[string]string)
	if params.Page > 0 {
		queryParams["page"] = strconv.Itoa(params.Page)
	}
	if params.Limit > 0 {
		queryParams["limit"] = strconv.Itoa(params.Limit)
	}
	if params.From != "" {
		queryParams["from"] = params.From
	}
	if params.To != "" {
		queryParams["to"] = params.To
	}

	respBody, err := s.client.request(ctx, "GET", PathWalletMovements, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct PaginatedResponse[WalletMovement]
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
