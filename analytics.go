package hrpay

import (
	"context"
	"encoding/json"
)

type AnalyticsSummary struct {
	Period            string  `json:"period"`
	TotalTransactions int     `json:"total_transactions"`
	TotalCashIn       float64 `json:"total_cashin"`
	TotalCashOut      float64 `json:"total_cashout"`
	TotalFees         float64 `json:"total_fees"`
	NetRevenue        float64 `json:"net_revenue"`
	Currency          string  `json:"currency"`
}

type AnalyticsTransactions struct {
	ByStatus   map[string]int `json:"by_status"`
	ByOperator map[string]int `json:"by_operator"`
	ByType     struct {
		CashIn  int `json:"cashin"`
		CashOut int `json:"cashout"`
	} `json:"by_type"`
}

type RevenueBreakdown struct {
	Source     string  `json:"source"`
	Amount     float64 `json:"amount"`
	Percentage float64 `json:"percentage"`
}

type AnalyticsRevenue struct {
	Total     float64            `json:"total"`
	Currency  string             `json:"currency"`
	Breakdown []RevenueBreakdown `json:"breakdown"`
}

type AnalyticsService struct {
	client *Client
}

// Summary gets a summary of transaction statistics.
func (s *AnalyticsService) Summary(ctx context.Context) (*AnalyticsSummary, error) {
	respBody, err := s.client.request(ctx, "GET", PathStatsSummary, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[AnalyticsSummary]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct AnalyticsSummary
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Transactions gets status/operator/type transaction counts.
func (s *AnalyticsService) Transactions(ctx context.Context) (*AnalyticsTransactions, error) {
	respBody, err := s.client.request(ctx, "GET", PathStatsTransactions, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[AnalyticsTransactions]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct AnalyticsTransactions
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Revenue gets total/source breakdown revenue stats.
func (s *AnalyticsService) Revenue(ctx context.Context) (*AnalyticsRevenue, error) {
	respBody, err := s.client.request(ctx, "GET", PathStatsRevenue, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[AnalyticsRevenue]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct AnalyticsRevenue
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
