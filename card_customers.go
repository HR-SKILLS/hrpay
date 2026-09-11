package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CardCustomerCreateParams enrolls a card holder for KYC review. Every field
// is required by the API.
type CardCustomerCreateParams struct {
	FirstName            string         `json:"first_name"`
	LastName             string         `json:"last_name"`
	Email                string         `json:"email"`
	Country              string         `json:"country"`            // full country name, e.g. "Cameroon"
	CountryIsoCode       string         `json:"country_iso_code"`   // ISO 3166-1 alpha-2, e.g. "CM"
	CountryPhoneCode     string         `json:"country_phone_code"` // e.g. "+237"
	PhoneNumber          string         `json:"phone_number"`       // local number only, without the country code
	Street               string         `json:"street"`
	City                 string         `json:"city"`
	State                string         `json:"state"`
	PostalCode           string         `json:"postal_code"`
	IdentificationNumber string         `json:"identification_number"`
	IDDocumentType       IDDocumentType `json:"id_document_type"`
	DateOfBirth          string         `json:"date_of_birth"`     // YYYY-MM-DD
	IDDocumentFront      string         `json:"id_document_front"` // data-URI or raw base64
	IDDocumentBack       string         `json:"id_document_back"`
}

func minLen(s string, n int) bool { return len(strings.TrimSpace(s)) >= n }

// Validate applies the same client-side checks the server enforces, so
// obviously-invalid submissions fail fast without a round trip. The server
// remains authoritative — a Validate() pass here does not guarantee approval.
func (p *CardCustomerCreateParams) Validate() error {
	var issues []ValidationIssue
	addIssue := func(field, msg string) { issues = append(issues, ValidationIssue{Field: field, Message: msg}) }

	if !minLen(p.FirstName, 3) {
		addIssue("FirstName", "First name must be at least 3 characters")
	}
	if !minLen(p.LastName, 3) {
		addIssue("LastName", "Last name must be at least 3 characters")
	}
	if !strings.Contains(p.Email, "@") {
		addIssue("Email", "Email must be a valid address")
	}
	if !minLen(p.Country, 2) {
		addIssue("Country", "Country is required")
	}
	if len(p.CountryIsoCode) != 2 {
		addIssue("CountryIsoCode", "CountryIsoCode must be exactly 2 characters (ISO 3166-1 alpha-2)")
	}
	if !strings.HasPrefix(p.CountryPhoneCode, "+") {
		addIssue("CountryPhoneCode", `CountryPhoneCode must start with "+"`)
	}
	if !minLen(p.PhoneNumber, 6) {
		addIssue("PhoneNumber", "PhoneNumber must be at least 6 characters")
	} else if p.CountryPhoneCode != "" {
		stripped := strings.TrimPrefix(p.CountryPhoneCode, "+")
		if strings.HasPrefix(p.PhoneNumber, stripped) {
			addIssue("PhoneNumber", "PhoneNumber must be local only — do not repeat the country code")
		}
	}
	if !minLen(p.Street, 2) {
		addIssue("Street", "Street must be at least 2 characters")
	}
	if !minLen(p.City, 2) {
		addIssue("City", "City must be at least 2 characters")
	}
	if !minLen(p.State, 2) {
		addIssue("State", "State must be at least 2 characters")
	}
	if !minLen(p.PostalCode, 3) {
		addIssue("PostalCode", "PostalCode must be at least 3 characters")
	}
	if !minLen(p.IdentificationNumber, 3) {
		addIssue("IdentificationNumber", "IdentificationNumber must be at least 3 characters")
	}
	switch p.IDDocumentType {
	case IDDocumentNIN, IDDocumentPassport, IDDocumentVotersCard, IDDocumentDriversLicense:
	default:
		addIssue("IDDocumentType", "IDDocumentType must be one of NIN, PASSPORT, VOTERS_CARD, DRIVERS_LICENSE")
	}
	if dob, err := time.Parse("2006-01-02", p.DateOfBirth); err != nil {
		addIssue("DateOfBirth", "DateOfBirth must be in YYYY-MM-DD format")
	} else if !isAtLeast18(dob, time.Now()) {
		addIssue("DateOfBirth", "Card holder must be at least 18 years old")
	}
	if p.IDDocumentFront == "" {
		addIssue("IDDocumentFront", "IDDocumentFront is required")
	}
	if p.IDDocumentBack == "" {
		addIssue("IDDocumentBack", "IDDocumentBack is required")
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func isAtLeast18(dob, now time.Time) bool {
	age := now.Year() - dob.Year()
	if now.Month() < dob.Month() || (now.Month() == dob.Month() && now.Day() < dob.Day()) {
		age--
	}
	return age >= 18
}

// CardCustomer is a card holder's KYC record.
type CardCustomer struct {
	ID                   string         `json:"id"`
	FirstName            string         `json:"first_name"`
	LastName             string         `json:"last_name"`
	Email                string         `json:"email"`
	Country              string         `json:"country"`
	CountryIsoCode       string         `json:"country_iso_code"`
	CountryPhoneCode     string         `json:"country_phone_code"`
	PhoneNumber          string         `json:"phone_number"`
	Street               string         `json:"street"`
	City                 string         `json:"city"`
	State                string         `json:"state"`
	PostalCode           string         `json:"postal_code"`
	IdentificationNumber string         `json:"identification_number"`
	IDDocumentType       IDDocumentType `json:"id_document_type"`
	DateOfBirth          string         `json:"date_of_birth"`
	KYCStatus            KYCStatus      `json:"kyc_status"`
	RejectionReason      string         `json:"rejection_reason,omitempty"`
	IsActive             bool           `json:"is_active"`
	IsTest               bool           `json:"is_test"`
	EnrolledAt           string         `json:"enrolled_at,omitempty"`
	ReviewedAt           string         `json:"reviewed_at,omitempty"`
	CreatedAt            string         `json:"created_at"`
}

type cardCustomerCreateResult struct {
	Customer CardCustomer `json:"customer"`
	Message  string       `json:"message"`
}

type CardCustomerListParams struct {
	Status string // filters by kyc_status
	Search string // name/email search
}

type CardCustomerListResult struct {
	Customers []CardCustomer `json:"customers"`
}

type CardCustomerCardsResult struct {
	Cards    []VirtualCard `json:"cards"`
	Customer CardCustomer  `json:"customer"`
}

type CardCustomersService struct {
	client *Client
}

// Create submits a card holder for KYC review. The returned customer's
// KYCStatus is always "PENDING_REVIEW" — approval is a manual, out-of-band
// action by an HR-Skills Pay administrator; there is no API call that
// accelerates it. Use PollEnrollment to wait for a terminal KYC status.
func (s *CardCustomersService) Create(ctx context.Context, params CardCustomerCreateParams) (*CardCustomer, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := s.client.request(ctx, "POST", PathCardCustomers, body, nil)
	if err != nil {
		return nil, err
	}

	var result cardCustomerCreateResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return &result.Customer, nil
}

// List returns the merchant's card customers, optionally filtered.
func (s *CardCustomersService) List(ctx context.Context, params CardCustomerListParams) (*CardCustomerListResult, error) {
	queryParams := make(map[string]string)
	if params.Status != "" {
		queryParams["status"] = params.Status
	}
	if params.Search != "" {
		queryParams["search"] = params.Search
	}

	respBody, err := s.client.request(ctx, "GET", PathCardCustomers, nil, queryParams)
	if err != nil {
		return nil, err
	}

	var direct CardCustomerListResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Get returns one card customer by ID.
func (s *CardCustomersService) Get(ctx context.Context, customerID string) (*CardCustomer, error) {
	if customerID == "" {
		return nil, errors.New("[hrpay] Customer ID is required")
	}

	path := fmt.Sprintf("%s/%s", PathCardCustomers, customerID)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct CardCustomer
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Cards returns the virtual cards issued to a customer.
func (s *CardCustomersService) Cards(ctx context.Context, customerID string) (*CardCustomerCardsResult, error) {
	if customerID == "" {
		return nil, errors.New("[hrpay] Customer ID is required")
	}

	path := fmt.Sprintf("%s/%s/cards", PathCardCustomers, customerID)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct CardCustomerCardsResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// Transactions returns this customer's card transaction history. For a test
// customer, or one not yet linked to Cartevo, the server always returns an
// empty result rather than an error.
func (s *CardCustomersService) Transactions(ctx context.Context, customerID string) (*VirtualCardTransactionsResult, error) {
	if customerID == "" {
		return nil, errors.New("[hrpay] Customer ID is required")
	}

	path := fmt.Sprintf("%s/%s/transactions", PathCardCustomers, customerID)
	respBody, err := s.client.request(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var direct VirtualCardTransactionsResult
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return &direct, nil
}

// PollEnrollment fetches a card customer periodically until its KYC status
// reaches a terminal state (ENROLLED, REJECTED_LOCAL, or REJECTED_PROVIDER).
// KYC review is manual and can take hours or longer — opts.Interval defaults
// to DefaultKYCPollInterval (5 minutes), not the payment-tuned
// DefaultPollInterval (3 seconds); do not poll this endpoint every few
// seconds.
func (s *CardCustomersService) PollEnrollment(ctx context.Context, customerID string, opts PollOptions) (*CardCustomer, error) {
	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultKYCPollInterval
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultPollTimeout
	}

	deadline := time.Now().Add(timeout)
	attempt := 0

	for {
		attempt++

		customer, err := s.Get(ctx, customerID)
		if err != nil {
			return nil, err
		}

		if opts.OnStatus != nil {
			opts.OnStatus(string(customer.KYCStatus), attempt)
		}

		if TerminalKYCStatuses[customer.KYCStatus] {
			return customer, nil
		}

		if opts.MaxAttempts > 0 && attempt >= opts.MaxAttempts {
			return nil, &ApiError{SDKError: SDKError{
				StatusCode: 408,
				Code:       "POLL_MAX_ATTEMPTS_REACHED",
				Message:    fmt.Sprintf("Polling stopped after %d attempts. Last status: %s", attempt, customer.KYCStatus),
			}}
		}

		if time.Now().Add(interval).After(deadline) {
			return nil, &ApiError{SDKError: SDKError{
				StatusCode: 408,
				Code:       "POLL_TIMEOUT",
				Message:    fmt.Sprintf("Polling timed out after %v. Last status: %s", timeout, customer.KYCStatus),
			}}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
