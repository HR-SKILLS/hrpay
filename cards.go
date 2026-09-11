package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// CardsService groups the three route families that together make up the
// Virtual Cards feature: KYC onboarding (Customers), the dedicated USD wallet
// and pricing (Wallet), and the card lifecycle itself (Virtual). They are
// nested here — mirroring BillsService's Eneo/Camwater/CanalPlus/Customs
// grouping — because issuing a card requires, in strict order, an ENROLLED
// customer and a funded card wallet: these are one feature, not three
// independent ones.
type CardsService struct {
	Customers *CardCustomersService
	Wallet    *CardWalletService
	Virtual   *VirtualCardsService
}

// CardProviderData is nested, ordinary snake_case data about the upstream
// issuer — the PascalCase quirk described on VirtualCard does not apply here.
type CardProviderData struct {
	Provider   string `json:"provider"`
	MaskedPan  string `json:"masked_pan"`
	ProviderID string `json:"provider_id"`
}

// VirtualCard is a card, as returned by Create/Get/List and inside
// CardCustomersService.Cards.
//
// The server serializes this specific object by dumping its internal Go
// struct with no json tags, so the wire keys are PascalCase ("ID",
// "MerchantID", "CardNetwork", ...) unlike every other response type in this
// SDK (which is snake_case throughout). The tags below deliberately spell out
// that exact PascalCase wire format instead of following this SDK's usual
// snake_case convention: encoding/json's case-insensitive Unmarshal fallback
// only helps for single-word fields (e.g. "id"/"ID", "status"/"Status") —
// it compares byte-for-byte with case folded, so it does NOT bridge
// "merchant_id" (snake_case tag) against "MerchantID" (PascalCase wire key)
// for any multi-word field. Do not "fix" these tags to snake_case. See
// TestVirtualCard_DecodesPascalCaseFields in conformance_test.go, which pins
// this against the literal JSON from the spec.
type VirtualCard struct {
	ID                     string            `json:"ID"`
	MerchantID             string            `json:"MerchantID"`
	CustomerID             string            `json:"CustomerID"`
	FundingWalletAccountID string            `json:"FundingWalletAccountID,omitempty"`
	ExternalID             string            `json:"ExternalID,omitempty"`
	CardNetwork            CardBrand         `json:"CardNetwork"`
	Last4                  string            `json:"Last4"`
	ExpiryMonth            int               `json:"ExpiryMonth"`
	ExpiryYear             int               `json:"ExpiryYear"`
	Currency               string            `json:"Currency"` // "USD"
	Balance                float64           `json:"Balance"`
	SpendingLimit          float64           `json:"SpendingLimit"`
	Status                 CardStatus        `json:"Status"`
	HolderName             string            `json:"HolderName"`
	HolderEmail            string            `json:"HolderEmail"`
	Label                  string            `json:"Label,omitempty"`
	IsTeamCard             bool              `json:"IsTeamCard"`
	AssignedToUserID       string            `json:"AssignedToUserID,omitempty"`
	IsTest                 bool              `json:"IsTest"`
	ProviderData           *CardProviderData `json:"ProviderData,omitempty"`
	FrozenAt               string            `json:"FrozenAt,omitempty"`
	LockedAt               string            `json:"LockedAt,omitempty"`
	CanceledAt             string            `json:"CanceledAt,omitempty"`
	CreatedAt              string            `json:"CreatedAt"`
	UpdatedAt              string            `json:"UpdatedAt"`
}

// VirtualCardCreateParams issues a card for a customer who must already be
// ENROLLED.
type VirtualCardCreateParams struct {
	CustomerID string    `json:"customer_id"`
	Brand      CardBrand `json:"brand,omitempty"`        // default VISA; "MC" is accepted and normalized to MASTERCARD
	NameOnCard string    `json:"name_on_card,omitempty"` // max 50 chars; defaults to the customer's name, uppercased, server-side
	Amount     float64   `json:"amount,omitempty"`       // initial USD funding
	Label      string    `json:"label,omitempty"`
}

func (p *VirtualCardCreateParams) Validate() error {
	var issues []ValidationIssue
	if p.CustomerID == "" {
		issues = append(issues, ValidationIssue{Field: "CustomerID", Message: "CustomerID is required"})
	}
	if len(p.NameOnCard) > 50 {
		issues = append(issues, ValidationIssue{Field: "NameOnCard", Message: "NameOnCard must be at most 50 characters"})
	}
	if p.Amount < 0 {
		issues = append(issues, ValidationIssue{Field: "Amount", Message: "Amount must be non-negative"})
	}
	switch p.Brand {
	case "", CardBrandVisa, CardBrandMastercard, "MC":
	default:
		issues = append(issues, ValidationIssue{Field: "Brand", Message: "Brand must be VISA, MASTERCARD, or MC"})
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

// VirtualCardCreateResult distinguishes the synchronous (201, Card populated)
// and pending (202, Cartevo confirmation still in flight) outcomes of
// VirtualCardsService.Create.
type VirtualCardCreateResult struct {
	Card    *VirtualCard // non-nil on synchronous success
	Pending bool         // true when issuance is PENDING, reconciled later via webhook/worker
	CardID  string       // set when Pending
	Message string       // set when Pending
}

type virtualCardCreateRaw struct {
	Card    *VirtualCard `json:"card"`
	CardID  string       `json:"card_id"`
	Status  string       `json:"status"`
	Message string       `json:"message"`
}

type VirtualCardGetParams struct {
	// Reveal adds the card's full PAN/CVV/expiry in clear text. Every reveal
	// is audited server-side. Never log or persist the returned Sensitive
	// block — treat it with the same PCI-level care as talking to Cartevo
	// directly.
	Reveal bool
	// Sync forces an opportunistic balance/status resync with Cartevo before
	// responding.
	Sync bool
}

type CardSensitiveData struct {
	Number      string `json:"number"`
	CVV         string `json:"cvv"`
	ExpiryMonth int    `json:"expiry_month"`
	ExpiryYear  int    `json:"expiry_year"`
}

type VirtualCardGetResult struct {
	Card      *VirtualCard       `json:"card"`
	Sensitive *CardSensitiveData `json:"sensitive,omitempty"`
}

type VirtualCardListParams struct {
	CustomerID string
	Status     string
}

type VirtualCardListResult struct {
	Cards []VirtualCard `json:"cards"`
}

type VirtualCardTopupParams struct {
	Amount float64 `json:"amount"`
}

func (p *VirtualCardTopupParams) Validate() error {
	if p.Amount < 1 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be at least 1 USD"}}}
	}
	return nil
}

// VirtualCardTopupResult distinguishes the synchronous and pending (202)
// outcomes of a topup.
type VirtualCardTopupResult struct {
	Pending     bool
	CardID      string  `json:"card_id"`
	Balance     float64 `json:"balance"`
	Amount      float64 `json:"amount"`
	Fee         float64 `json:"fee"`
	ProviderRef string  `json:"provider_ref"`
	Status      string  `json:"status,omitempty"` // present only on the 202 pending shape
	Message     string  `json:"message,omitempty"`
}

type VirtualCardWithdrawParams struct {
	Amount float64 `json:"amount"`
}

func (p *VirtualCardWithdrawParams) Validate() error {
	if p.Amount < 1 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be at least 1 USD"}}}
	}
	return nil
}

// VirtualCardWithdrawResult distinguishes the synchronous and pending (202)
// outcomes of a withdrawal. On Pending, the spec guarantees no local balance
// movement has happened yet.
type VirtualCardWithdrawResult struct {
	Pending     bool
	CardID      string  `json:"card_id"`
	Balance     float64 `json:"balance"`
	Credited    float64 `json:"credited"`
	Fee         float64 `json:"fee"`
	ProviderRef string  `json:"provider_ref"`
	Status      string  `json:"status,omitempty"`
	Message     string  `json:"message,omitempty"`
}

type VirtualCardFreezeParams struct {
	Lock bool `json:"lock,omitempty"` // also request a stronger upstream network-level lock
}

type VirtualCardUnfreezeParams struct {
	// Unlock is accepted for symmetry with Freeze, but the server always
	// clears any upstream network lock on unfreeze regardless of this value.
	Unlock bool `json:"unlock,omitempty"`
}

type VirtualCardStatusResult struct {
	CardID string     `json:"card_id"`
	Status CardStatus `json:"status"`
}

type VirtualCardTerminateResult struct {
	CardID   string     `json:"card_id"`
	Status   CardStatus `json:"status"` // "TERMINATED"
	Refunded float64    `json:"refunded"`
}

type CardTransactionCardRef struct {
	ID        string    `json:"id"`
	MaskedPan string    `json:"masked_pan"`
	Brand     CardBrand `json:"brand"`
}

type CardTransactionCustomerRef struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// CardTransaction covers both the local/sandbox log shape (Card/Customer nil)
// and the Cartevo-proxied live shape (Card/Customer populated).
type CardTransaction struct {
	ID          string                      `json:"id"`
	Category    CardTransactionCategory     `json:"category"`
	Type        CardTransactionType         `json:"type"`
	Status      string                      `json:"status"`
	Amount      float64                     `json:"amount"`
	Currency    string                      `json:"currency"`
	Description string                      `json:"description,omitempty"`
	CreatedAt   string                      `json:"created_at"`
	Card        *CardTransactionCardRef     `json:"card,omitempty"`
	Customer    *CardTransactionCustomerRef `json:"customer,omitempty"`
}

// VirtualCardTransactionsParams paginates a card's transaction history.
//
// Page is 0-INDEXED for a live card proxied from Cartevo — page 0 is the
// first page, unlike every other paginated endpoint in this SDK (see Meta /
// PaginatedResponse, which are 1-indexed). A loop that starts at Page: 1
// skips the true first page for a live card.
type VirtualCardTransactionsParams struct {
	Page   int
	Limit  int
	Type   string
	Status string
}

type VirtualCardTransactionsResult struct {
	Transactions []CardTransaction `json:"transactions"`
	Total        int               `json:"total"`
	Page         int               `json:"page,omitempty"`        // present only for a live (Cartevo-proxied) result
	TotalPages   int               `json:"total_pages,omitempty"` // present only for a live result
}

type VirtualCardsService struct {
	client *Client
}

func normalizeCardBrand(b CardBrand) CardBrand {
	if b == "MC" {
		return CardBrandMastercard
	}
	if b == "" {
		return CardBrandVisa
	}
	return b
}

// Create issues a card for an ENROLLED customer. An issuance fee
// (pricing.creation_fee_xaf, converted to USD at the mid FX rate) plus the
// requested initial Amount are debited from the USD card wallet up front,
// before any call to the card issuer. If the issuer's response is ambiguous
// (network/5xx), the card is left PENDING and reconciled later — check
// Pending on the result rather than assuming synchronous success.
func (s *VirtualCardsService) Create(ctx context.Context, params VirtualCardCreateParams) (*VirtualCardCreateResult, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	params.Brand = normalizeCardBrand(params.Brand)

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathVirtualCards, body, nil)
	if err != nil {
		return nil, err
	}

	var raw virtualCardCreateRaw
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, err
	}
	if raw.Card != nil {
		return &VirtualCardCreateResult{Card: raw.Card}, nil
	}
	if raw.Status == "PENDING" {
		return &VirtualCardCreateResult{Pending: true, CardID: raw.CardID, Message: raw.Message}, nil
	}
	return nil, &UnknownError{SDKError: SDKError{Message: "Unexpected virtual card creation response shape", RawBody: respBody}}
}

// List returns the merchant's virtual cards, optionally filtered.
func (s *VirtualCardsService) List(ctx context.Context, params VirtualCardListParams) (*VirtualCardListResult, error) {
	queryParams := make(map[string]string)
	if params.CustomerID != "" {
		queryParams["customer_id"] = params.CustomerID
	}
	if params.Status != "" {
		queryParams["status"] = params.Status
	}

	respBody, err := s.client.request(ctx, "GET", PathVirtualCards, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardListResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Get returns a card's details. Reveal adds the full PAN/CVV (PCI-sensitive,
// audited server-side, never persisted) — see VirtualCardGetParams.
func (s *VirtualCardsService) Get(ctx context.Context, cardID string, params VirtualCardGetParams) (*VirtualCardGetResult, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	queryParams := make(map[string]string)
	if params.Reveal {
		queryParams["reveal"] = "true"
	}
	if params.Sync {
		queryParams["sync"] = "true"
	}

	path := fmt.Sprintf("%s/%s", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "GET", path, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardGetResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Topup recharges an ACTIVE card from the USD card wallet.
func (s *VirtualCardsService) Topup(ctx context.Context, cardID string, params VirtualCardTopupParams) (*VirtualCardTopupResult, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/%s/topup", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "POST", path, body, nil)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardTopupResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	direct.Pending = direct.Status == "PENDING"
	return &direct, nil
}

// Withdraw pulls funds from an ACTIVE or FROZEN card back into the USD card
// wallet. On a Pending (202) result, the spec guarantees no local balance
// movement has occurred yet.
func (s *VirtualCardsService) Withdraw(ctx context.Context, cardID string, params VirtualCardWithdrawParams) (*VirtualCardWithdrawResult, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/%s/withdraw", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "POST", path, body, nil)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardWithdrawResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	direct.Pending = direct.Status == "PENDING"
	return &direct, nil
}

// Freeze blocks transactions on an ACTIVE card.
func (s *VirtualCardsService) Freeze(ctx context.Context, cardID string, params VirtualCardFreezeParams) (*VirtualCardStatusResult, error) {
	return s.statusAction(ctx, cardID, "freeze", params)
}

// Unfreeze restores a FROZEN card to ACTIVE.
func (s *VirtualCardsService) Unfreeze(ctx context.Context, cardID string, params VirtualCardUnfreezeParams) (*VirtualCardStatusResult, error) {
	return s.statusAction(ctx, cardID, "unfreeze", params)
}

func (s *VirtualCardsService) statusAction(ctx context.Context, cardID, action string, payload interface{}) (*VirtualCardStatusResult, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/%s/%s", PathVirtualCards, cardID, action)
	respBody, err := s.client.request(ctx, "POST", path, body, nil)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardStatusResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Terminate irreversibly closes a card. Any residual balance is
// automatically credited back to the USD card wallet.
func (s *VirtualCardsService) Terminate(ctx context.Context, cardID string) (*VirtualCardTerminateResult, error) {
	return s.terminateVia(ctx, cardID, "terminate")
}

// Cancel is equivalent to Terminate — the server treats both routes as the
// exact same action. Kept alongside Terminate for discoverability.
func (s *VirtualCardsService) Cancel(ctx context.Context, cardID string) (*VirtualCardTerminateResult, error) {
	return s.terminateVia(ctx, cardID, "cancel")
}

func (s *VirtualCardsService) terminateVia(ctx context.Context, cardID, action string) (*VirtualCardTerminateResult, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	path := fmt.Sprintf("%s/%s/%s", PathVirtualCards, cardID, action)
	respBody, err := s.client.request(ctx, "POST", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardTerminateResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Transactions returns a card's transaction history. For a live card proxied
// from Cartevo, Page is 0-indexed — see VirtualCardTransactionsParams.
func (s *VirtualCardsService) Transactions(ctx context.Context, cardID string, params VirtualCardTransactionsParams) (*VirtualCardTransactionsResult, error) {
	if cardID == "" {
		return nil, errors.New("[hrpay] Card ID is required")
	}

	queryParams := make(map[string]string)
	if params.Page > 0 {
		queryParams["page"] = strconv.Itoa(params.Page)
	}
	if params.Limit > 0 {
		queryParams["limit"] = strconv.Itoa(params.Limit)
	}
	if params.Type != "" {
		queryParams["type"] = params.Type
	}
	if params.Status != "" {
		queryParams["status"] = params.Status
	}

	path := fmt.Sprintf("%s/%s/transactions", PathVirtualCards, cardID)
	respBody, err := s.client.request(ctx, "GET", path, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardTransactionsResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}
