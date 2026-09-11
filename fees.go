package hrpay

import (
	"context"
	"encoding/json"
)

// FeeScheduleEntry is one country x direction row of the merchant's Mobile
// Money fee schedule. The platform default is 2% flat, but negotiated rates
// (0%, 1%, 1.25%, 1.5%, 3%, ...) exist per merchant — never hardcode a rate,
// always read it from FeesService.List.
type FeeScheduleEntry struct {
	Country       Country  `json:"country"`
	CountryName   string   `json:"country_name"`
	Currency      Currency `json:"currency"`
	Direction     string   `json:"direction"`   // CASHIN, CASHOUT
	FeePercent    float64  `json:"fee_percent"` // ratio, e.g. 0.02
	FeePct        float64  `json:"fee_pct"`     // percentage, e.g. 2
	FeeFixed      float64  `json:"fee_fixed"`
	DefaultFeePct float64  `json:"default_fee_pct"` // platform default (2), regardless of this merchant's actual rate
}

type FeeScheduleResponse struct {
	Currency string             `json:"currency"`
	Fees     []FeeScheduleEntry `json:"fees"`
}

type FeesService struct {
	client *Client
}

// List returns the merchant's actual Mobile Money fee schedule, one row per
// activated country x direction. Compare FeePct against DefaultFeePct to
// detect a negotiated rate.
// Endpoint: GET /v1/payments/fees
func (s *FeesService) List(ctx context.Context) (*FeeScheduleResponse, error) {
	respBody, err := s.client.request(ctx, "GET", PathPaymentFees, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct FeeScheduleResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
