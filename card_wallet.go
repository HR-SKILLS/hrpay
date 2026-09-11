package hrpay

import (
	"context"
	"encoding/json"
	"strconv"
)

func formatAmount(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// CardWalletBalance is the merchant's dedicated USD wallet for card
// operations — distinct from the main XAF wallet (WalletService.Balance).
type CardWalletBalance struct {
	ID       string  `json:"id"`
	Currency string  `json:"currency"` // "USD"
	Balance  float64 `json:"balance"`
	Held     float64 `json:"held"` // locked by a concurrent in-flight operation
}

// CardPricing is the merchant's card fee schedule. Always read live — never
// hardcode these values, they vary per plan/negotiation.
type CardPricing struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Label                   string  `json:"label"`
	IsDefault               bool    `json:"is_default"`
	IsActive                bool    `json:"is_active"`
	CreationFeeXAF          float64 `json:"creation_fee_xaf"`
	FundFeeFixedUSD         float64 `json:"fund_fee_fixed_usd"`
	FundFeePercent          float64 `json:"fund_fee_percent"`
	WithdrawFeeFixedUSD     float64 `json:"withdraw_fee_fixed_usd"`
	WithdrawFeePercent      float64 `json:"withdraw_fee_percent"`
	FxMarginPercent         float64 `json:"fx_margin_percent"`
	DeclineFeeFixedUSD      float64 `json:"decline_fee_fixed_usd"`
	DeclineFeeMarginPercent float64 `json:"decline_fee_margin_percent"`
	XBorderFeeFixedUSD      float64 `json:"x_border_fee_fixed_usd"`
	XBorderFeeMarginPercent float64 `json:"x_border_fee_margin_percent"`
	MaxCardsPerCustomer     int     `json:"max_cards_per_customer"`
	MinFundUSD              float64 `json:"min_fund_usd"`
	MaxFundUSD              float64 `json:"max_fund_usd"`
}

// CardWalletOverview is the response of GET /api/v1/card-wallet.
type CardWalletOverview struct {
	Wallet  CardWalletBalance `json:"wallet"`
	Pricing CardPricing       `json:"pricing"`
}

type CardFxRate struct {
	Mid       float64 `json:"mid"`
	Effective float64 `json:"effective"`
	Source    string  `json:"source"`
}

// CardPricingResponse is the response of GET /api/v1/card-pricing.
type CardPricingResponse struct {
	Pricing CardPricing `json:"pricing"`
	Rate    CardFxRate  `json:"rate"`
}

// CardWalletQuoteParams requests an indicative (non-executed) FX quote.
// Exactly one of AmountUSD or AmountSource must be set.
type CardWalletQuoteParams struct {
	AmountUSD    float64
	AmountSource float64
	// SourceCurrency: only "XAF" works today; leave empty to use the
	// server's default rather than implying broader currency support.
	SourceCurrency Currency
	Direction      string // "fund" (default) or "withdraw"
}

func (p *CardWalletQuoteParams) Validate() error {
	if (p.AmountUSD > 0) == (p.AmountSource > 0) {
		return &ValidationError{SDKError: SDKError{Code: ErrAmbiguousAmount.Error()}, Issues: []ValidationIssue{
			{Field: "AmountUSD", Message: "Exactly one of AmountUSD or AmountSource must be set"},
		}}
	}
	return nil
}

type CardQuote struct {
	AmountUSD     float64 `json:"amount_usd"`
	AmountXAF     float64 `json:"amount_xaf"`
	MidRate       float64 `json:"mid_rate"`
	EffectiveRate float64 `json:"effective_rate"`
	MarginPercent float64 `json:"margin_percent"`
	MarginUSD     float64 `json:"margin_usd"`
	RateSource    string  `json:"rate_source"`
}

type CardWalletQuoteResult struct {
	Quote          CardQuote `json:"quote"`
	SourceCurrency string    `json:"source_currency"`
}

// CardWalletFundParams funds (or, via CardWalletService.Withdraw, drains) the
// USD card wallet. Exactly one of AmountUSD or AmountSource must be set.
type CardWalletFundParams struct {
	AmountUSD      float64
	AmountSource   float64
	SourceCurrency Currency // only "XAF" works today
}

func (p *CardWalletFundParams) Validate() error {
	if (p.AmountUSD > 0) == (p.AmountSource > 0) {
		return &ValidationError{SDKError: SDKError{Code: ErrAmbiguousAmount.Error()}, Issues: []ValidationIssue{
			{Field: "AmountUSD", Message: "Exactly one of AmountUSD or AmountSource must be set"},
		}}
	}
	return nil
}

type CardWalletFundResult struct {
	AmountUSD      float64 `json:"amount_usd"`
	AmountXAF      float64 `json:"amount_xaf"`
	AmountSource   float64 `json:"amount_source"`
	EffectiveRate  float64 `json:"effective_rate"`
	SourceCurrency string  `json:"source_currency"`
	USDBalance     float64 `json:"usd_balance"`
}

// CardWalletWithdrawParams/Result mirror CardWalletFundParams/Result byte for
// byte — the withdraw endpoint reverses the same fields.
type CardWalletWithdrawParams = CardWalletFundParams
type CardWalletWithdrawResult = CardWalletFundResult

type CardWalletService struct {
	client *Client
}

// Get returns the USD card wallet balance and the merchant's current pricing.
func (s *CardWalletService) Get(ctx context.Context) (*CardWalletOverview, error) {
	respBody, err := s.client.request(ctx, "GET", PathCardWallet, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct CardWalletOverview
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

func quoteQueryParams(amountUSD, amountSource float64, sourceCurrency Currency, direction string) map[string]string {
	q := make(map[string]string)
	if amountUSD > 0 {
		q["amount_usd"] = formatAmount(amountUSD)
	}
	if amountSource > 0 {
		q["amount_source"] = formatAmount(amountSource)
	}
	if sourceCurrency != "" {
		q["source_currency"] = string(sourceCurrency)
	}
	if direction != "" {
		q["direction"] = direction
	}
	return q
}

// Quote returns an indicative, non-locked FX quote for a fund/withdraw
// operation — the actual rate is recomputed at execution time (server-side
// cache TTL is 10 minutes).
func (s *CardWalletService) Quote(ctx context.Context, params CardWalletQuoteParams) (*CardWalletQuoteResult, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	queryParams := quoteQueryParams(params.AmountUSD, params.AmountSource, params.SourceCurrency, params.Direction)
	respBody, err := s.client.request(ctx, "GET", PathCardWalletQuote, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct CardWalletQuoteResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Fund credits the USD card wallet from the main XAF wallet.
// Requires an Idempotency-Key (auto-generated unless overridden via
// WithIdempotencyKey).
func (s *CardWalletService) Fund(ctx context.Context, params CardWalletFundParams) (*CardWalletFundResult, error) {
	return s.moveFunds(ctx, PathCardWalletFund, params)
}

// Withdraw converts USD card wallet funds back to the main XAF wallet. Purely
// internal — no funds leave the platform. Requires an Idempotency-Key.
func (s *CardWalletService) Withdraw(ctx context.Context, params CardWalletWithdrawParams) (*CardWalletWithdrawResult, error) {
	return s.moveFunds(ctx, PathCardWalletWithdraw, params)
}

func (s *CardWalletService) moveFunds(ctx context.Context, path string, params CardWalletFundParams) (*CardWalletFundResult, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{}
	if params.AmountUSD > 0 {
		payload["amount_usd"] = params.AmountUSD
	}
	if params.AmountSource > 0 {
		payload["amount_source"] = params.AmountSource
	}
	if params.SourceCurrency != "" {
		payload["source_currency"] = params.SourceCurrency
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", path, body, nil)
	if err != nil {
		return nil, err
	}

	var direct CardWalletFundResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Pricing returns the merchant's card fee schedule and current FX rate.
func (s *CardWalletService) Pricing(ctx context.Context) (*CardPricingResponse, error) {
	respBody, err := s.client.request(ctx, "GET", PathCardPricing, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct CardPricingResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
