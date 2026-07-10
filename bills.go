package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// ─── ENEO Structs ─────────────────────────────────────────────────

// validateOptionalPhone enforces the phone format only when a value is given.
// customer_phone is optional on every bill/VAS endpoint.
func validateOptionalPhone(field, phone string) error {
	if phone == "" {
		return nil
	}
	matched, _ := regexp.MatchString(`^\d+$`, phone)
	if len(phone) < 9 || !matched {
		return &ValidationError{Issues: []ValidationIssue{{Field: field, Message: field + " must be at least 9 digits and contain only digits"}}}
	}
	return nil
}

type EneoPrepaidParams struct {
	Meter         string  `json:"meter"`
	Amount        float64 `json:"amount"`
	CustomerPhone string  `json:"customer_phone,omitempty"`
}

func (p *EneoPrepaidParams) Validate() error {
	if p.Meter == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Meter", Message: "Meter number is required"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return validateOptionalPhone("CustomerPhone", p.CustomerPhone)
}

type EneoPostpaidParams struct {
	Meter         string  `json:"meter"`
	Amount        float64 `json:"amount"`
	CustomerPhone string  `json:"customer_phone,omitempty"`
}

func (p *EneoPostpaidParams) Validate() error {
	if p.Meter == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Meter", Message: "Meter number is required"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return validateOptionalPhone("CustomerPhone", p.CustomerPhone)
}

type EneoInvoice struct {
	Meter      string  `json:"meter"`
	Name       string  `json:"name"`
	Balance    float64 `json:"balance"`
	BillNumber string  `json:"bill_number,omitempty"`
	DueDate    string  `json:"due_date,omitempty"`
}

type EneoPayResponse struct {
	Reference string  `json:"reference"`
	Status    string  `json:"status"`
	Meter     string  `json:"meter"`
	Amount    float64 `json:"amount"`
	Token     string  `json:"token,omitempty"` // For prepaid only
	CreatedAt string  `json:"created_at"`
}

// ─── CAMWATER Structs ─────────────────────────────────────────────

type CamwaterPayParams struct {
	Meter         string  `json:"meter"`
	Amount        float64 `json:"amount"`
	CustomerPhone string  `json:"customer_phone,omitempty"`
}

func (p *CamwaterPayParams) Validate() error {
	if p.Meter == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Meter", Message: "Meter number is required"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return validateOptionalPhone("CustomerPhone", p.CustomerPhone)
}

type CamwaterInvoice struct {
	Meter      string  `json:"meter"`
	Name       string  `json:"name"`
	Balance    float64 `json:"balance"`
	BillNumber string  `json:"bill_number,omitempty"`
	DueDate    string  `json:"due_date,omitempty"`
}

type CamwaterPayResponse struct {
	Reference string  `json:"reference"`
	Status    string  `json:"status"`
	Meter     string  `json:"meter"`
	Amount    float64 `json:"amount"`
	CreatedAt string  `json:"created_at"`
}

// ─── CANAL+ Structs ───────────────────────────────────────────────

type CanalPlusPayParams struct {
	DecoderNumber string  `json:"decoder_number"`
	Amount        float64 `json:"amount"`
	CustomerPhone string  `json:"customer_phone,omitempty"`
	CustomerName  string  `json:"customer_name,omitempty"`
	// OfferCode is not part of the documented v1 contract. It is forwarded
	// only when set, for forward compatibility.
	OfferCode string `json:"offer_code,omitempty"`
}

func (p *CanalPlusPayParams) Validate() error {
	if p.DecoderNumber == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "DecoderNumber", Message: "DecoderNumber is required"}}}
	}
	matched, _ := regexp.MatchString(`^\d{8,12}$`, p.DecoderNumber)
	if !matched {
		return &ValidationError{Issues: []ValidationIssue{{Field: "DecoderNumber", Message: "DecoderNumber must be 8 to 12 digits"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return validateOptionalPhone("CustomerPhone", p.CustomerPhone)
}

type CanalPlusPayResponse struct {
	Reference     string  `json:"reference"`
	Status        string  `json:"status"`
	DecoderNumber string  `json:"decoder_number"`
	Amount        float64 `json:"amount"`
	CreatedAt     string  `json:"created_at"`
}

// ─── CUSTOMS Structs ──────────────────────────────────────────────

type CustomsPayParams struct {
	DeclarationRef string  `json:"declaration_ref"`
	Amount         float64 `json:"amount"`
	CustomerName   string  `json:"customer_name,omitempty"`
	CustomerPhone  string  `json:"customer_phone,omitempty"`
}

func (p *CustomsPayParams) Validate() error {
	if p.DeclarationRef == "" {
		return &ValidationError{Issues: []ValidationIssue{{Field: "DeclarationRef", Message: "DeclarationRef is required"}}}
	}
	if p.Amount <= 0 {
		return &ValidationError{Issues: []ValidationIssue{{Field: "Amount", Message: "Amount must be positive"}}}
	}
	return validateOptionalPhone("CustomerPhone", p.CustomerPhone)
}

type CustomsDeclaration struct {
	DeclarationRef string  `json:"declaration_ref"`
	CustomerName   string  `json:"customer_name"`
	Amount         float64 `json:"amount"`
	Status         string  `json:"status"`
	DueDate        string  `json:"due_date,omitempty"`
}

type CustomsPayResponse struct {
	Reference      string  `json:"reference"`
	Status         string  `json:"status"`
	DeclarationRef string  `json:"declaration_ref"`
	Amount         float64 `json:"amount"`
	CreatedAt      string  `json:"created_at"`
}

// ─── Modules ──────────────────────────────────────────────────────

type EneoModule struct {
	client *Client
}

func (m *EneoModule) Invoice(ctx context.Context, meterNumber string) (*EneoInvoice, error) {
	if meterNumber == "" {
		return nil, errors.New("[hrpay] Meter number is required")
	}

	path := fmt.Sprintf("%s/%s", PathBillsEneoInvoice, meterNumber)
	respBody, err := m.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[EneoInvoice]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct EneoInvoice
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

func (m *EneoModule) Prepaid(ctx context.Context, params EneoPrepaidParams) (*EneoPayResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"meter":  params.Meter,
		"amount": params.Amount,
	}
	if params.CustomerPhone != "" {
		payload["customer_phone"] = params.CustomerPhone
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := m.client.request(ctx, "POST", PathBillsEneoPrepaid, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[EneoPayResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct EneoPayResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

func (m *EneoModule) Postpaid(ctx context.Context, params EneoPostpaidParams) (*EneoPayResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"meter":  params.Meter,
		"amount": params.Amount,
	}
	if params.CustomerPhone != "" {
		payload["customer_phone"] = params.CustomerPhone
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := m.client.request(ctx, "POST", PathBillsEneoPostpaid, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[EneoPayResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct EneoPayResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

type CamwaterModule struct {
	client *Client
}

func (m *CamwaterModule) Invoice(ctx context.Context, meterNumber string) (*CamwaterInvoice, error) {
	if meterNumber == "" {
		return nil, errors.New("[hrpay] Meter number is required")
	}

	path := fmt.Sprintf("%s/%s", PathBillsCamwaterInvoice, meterNumber)
	respBody, err := m.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CamwaterInvoice]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CamwaterInvoice
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

func (m *CamwaterModule) Pay(ctx context.Context, params CamwaterPayParams) (*CamwaterPayResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"meter":  params.Meter,
		"amount": params.Amount,
	}
	if params.CustomerPhone != "" {
		payload["customer_phone"] = params.CustomerPhone
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := m.client.request(ctx, "POST", PathBillsCamwaterPay, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CamwaterPayResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CamwaterPayResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

type CanalPlusModule struct {
	client *Client
}

func (m *CanalPlusModule) Pay(ctx context.Context, params CanalPlusPayParams) (*CanalPlusPayResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"decoder_number": params.DecoderNumber,
		"amount":         params.Amount,
	}
	if params.CustomerPhone != "" {
		payload["customer_phone"] = params.CustomerPhone
	}
	if params.CustomerName != "" {
		payload["customer_name"] = params.CustomerName
	}
	if params.OfferCode != "" {
		payload["offer_code"] = params.OfferCode
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := m.client.request(ctx, "POST", PathBillsCanalPlusPay, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CanalPlusPayResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CanalPlusPayResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

type CustomsModule struct {
	client *Client
}

func (m *CustomsModule) Get(ctx context.Context, declarationRef string) (*CustomsDeclaration, error) {
	if declarationRef == "" {
		return nil, errors.New("[hrpay] Declaration reference is required")
	}

	path := fmt.Sprintf("%s/%s", PathBillsCustomsGet, declarationRef)
	respBody, err := m.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CustomsDeclaration]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CustomsDeclaration
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

func (m *CustomsModule) Pay(ctx context.Context, params CustomsPayParams) (*CustomsPayResponse, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"declaration_ref": params.DeclarationRef,
		"amount":          params.Amount,
	}
	if params.CustomerName != "" {
		payload["customer_name"] = params.CustomerName
	}
	if params.CustomerPhone != "" {
		payload["customer_phone"] = params.CustomerPhone
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	respBody, err := m.client.request(ctx, "POST", PathBillsCustomsPay, body, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[CustomsPayResponse]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return &envelope.Data, nil
	}

	var direct CustomsPayResponse
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// ─── Bills Service ────────────────────────────────────────────────

type BillsService struct {
	Eneo      *EneoModule
	Camwater  *CamwaterModule
	CanalPlus *CanalPlusModule
	Customs   *CustomsModule
}
