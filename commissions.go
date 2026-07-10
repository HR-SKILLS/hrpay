package hrpay

import (
	"context"
	"encoding/json"
	"strconv"
)

type VasService string

const (
	VasServiceAirtime   VasService = "AIRTIME"
	VasServiceData      VasService = "DATA"
	VasServiceEneo      VasService = "ENEO"
	VasServiceCamwater  VasService = "CAMWATER"
	VasServiceCanalPlus VasService = "CANALPLUS"
	VasServiceCustoms   VasService = "CUSTOMS"
)

type CommissionRate struct {
	Service     VasService `json:"service"`
	Rate        float64    `json:"rate"`
	Description string     `json:"description"`
}

type CommissionEntry struct {
	ID         string     `json:"id"`
	Service    VasService `json:"service"`
	Reference  string     `json:"reference"`
	Amount     float64    `json:"amount"`
	Commission float64    `json:"commission"`
	Rate       float64    `json:"rate"`
	CreatedAt  string     `json:"created_at"`
}

type CommissionSummary struct {
	From            string                `json:"from"`
	To              string                `json:"to"`
	TotalCommission float64               `json:"total_commission"`
	Currency        string                `json:"currency"`
	ByService       map[VasService]float64 `json:"by_service"`
}

type CommissionHistoryParams struct {
	Service VasService `json:"service,omitempty"`
	From    string     `json:"from,omitempty"`
	To      string     `json:"to,omitempty"`
	Page    int        `json:"page,omitempty"`
	Limit   int        `json:"limit,omitempty"`
}

type CommissionsService struct {
	client *Client
}

// Rates lists commission rates.
func (s *CommissionsService) Rates(ctx context.Context) ([]CommissionRate, error) {
	respBody, err := s.client.request(ctx, "GET", PathVasRates, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[[]CommissionRate]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return envelope.Data, nil
	}

	var direct []CommissionRate
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return direct, nil
}

// History retrieves historical commissions.
func (s *CommissionsService) History(ctx context.Context, params CommissionHistoryParams) ([]CommissionEntry, error) {
	queryParams := make(map[string]string)
	if params.Service != "" {
		queryParams["service"] = string(params.Service)
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

	respBody, err := s.client.request(ctx, "GET", PathVasCommissions, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[[]CommissionEntry]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return envelope.Data, nil
	}

	var direct []CommissionEntry
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return direct, nil
}

// Summary calculates VAS commissions within a date range.
func (s *CommissionsService) Summary(ctx context.Context, from, to string) (*CommissionSummary, error) {
	queryParams := map[string]string{
		"from": from,
		"to":   to,
	}

	respBody, err := s.client.request(ctx, "GET", PathVasCommissionsSummary, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CommissionSummary]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CommissionSummary
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
