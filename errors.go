package hrpay

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Sentinel business errors — codes as returned in the "error" field of the
// API error envelope: {"error": "CODE", "message": "...", "details": {...}}
var (
	ErrValidation                = errors.New("VALIDATION_ERROR")
	ErrMissingRequiredField      = errors.New("MISSING_REQUIRED_FIELD")
	ErrMissingAPIKey             = errors.New("MISSING_API_KEY")
	ErrInvalidAPIKey             = errors.New("INVALID_API_KEY")
	ErrMissingTransactionToken   = errors.New("MISSING_TRANSACTION_TOKEN")
	ErrInvalidTransactionToken   = errors.New("INVALID_TRANSACTION_TOKEN")
	ErrWalletBalanceInsufficient = errors.New("WALLET_BALANCE_INSUFFICIENT")
	ErrKycNotApproved            = errors.New("KYC_NOT_APPROVED")
	ErrWalletFrozen              = errors.New("WALLET_FROZEN")
	ErrIdempotencyKeyConflict    = errors.New("IDEMPOTENCY_KEY_CONFLICT")
	ErrOperatorNotAvailable      = errors.New("OPERATOR_NOT_AVAILABLE")
	ErrCurrencyMismatch          = errors.New("CURRENCY_MISMATCH")
	ErrRateLimitExceeded         = errors.New("RATE_LIMIT_EXCEEDED")
	ErrInternalError             = errors.New("INTERNAL_ERROR")
	ErrProviderNotConfigured     = errors.New("PROVIDER_NOT_CONFIGURED")
	ErrProviderTimeout           = errors.New("PROVIDER_TIMEOUT")
)

// Deprecated aliases kept for backwards compatibility with earlier SDK versions.
// They resolve to the canonical API error codes above.
var (
	ErrInvalidRequest       = ErrValidation
	ErrIdempotencyConflict  = ErrIdempotencyKeyConflict
	ErrDuplicateTransaction = ErrIdempotencyKeyConflict
	ErrKycRequired          = ErrKycNotApproved
	ErrInsufficientBalance  = ErrWalletBalanceInsufficient
	ErrOperatorUnavailable  = ErrOperatorNotAvailable
	ErrServiceUnavailable   = ErrProviderNotConfigured

	// Not part of the documented v1 error table; retained so existing callers
	// continue to compile.
	ErrMerchantInactive    = errors.New("MERCHANT_INACTIVE")
	ErrSandboxKeyRequired  = errors.New("SANDBOX_KEY_REQUIRED")
	ErrSandboxPathRequired = errors.New("SANDBOX_PATH_REQUIRED")
	ErrInvalidPhone        = errors.New("INVALID_PHONE")
)

// Base SDK Error
type SDKError struct {
	StatusCode int
	Code       string
	Message    string
	// Details carries the contextual "details" object of the error envelope
	// (e.g. "shortfall" on a 402 WALLET_BALANCE_INSUFFICIENT).
	Details map[string]interface{}
	RawBody []byte
}

func (e *SDKError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("[hrpay] %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("[hrpay] HTTP %d: %s", e.StatusCode, e.Message)
}

// Is implements compatibility with the standard errors.Is function for sentinels.
// The comparison is case-insensitive so that both the canonical uppercase codes
// and legacy lowercase codes resolve to the same sentinel.
func (e *SDKError) Is(target error) bool {
	if target == nil {
		return false
	}
	return strings.EqualFold(e.Code, target.Error())
}

// Authentication Error (401/403)
type AuthenticationError struct {
	SDKError
}

func (e *AuthenticationError) Error() string {
	return "[hrpay] Authentication error: " + e.Message
}

// Validation Error (400)
type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	SDKError
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("[hrpay] Validation error: %s (Issues: %v)", e.Message, e.Issues)
}

// Conflict Error (409)
type ConflictError struct {
	SDKError
}

func (e *ConflictError) Error() string {
	return "[hrpay] Conflict error: " + e.Message
}

// Generic API Error
type ApiError struct {
	SDKError
}

// Network Error
type NetworkError struct {
	SDKError
	Cause error
}

func (e *NetworkError) Error() string {
	if e.Cause != nil {
		return "[hrpay] Network error: " + e.Message + " (Cause: " + e.Cause.Error() + ")"
	}
	return "[hrpay] Network error: " + e.Message
}

// Timeout Error (408)
type TimeoutError struct {
	SDKError
	TimeoutMs int64
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("[hrpay] Request timed out after %dms", e.TimeoutMs)
}

// Rate Limit Error (429)
type RateLimitError struct {
	SDKError
	RetryAfterSeconds int
}

func (e *RateLimitError) Error() string {
	if e.RetryAfterSeconds > 0 {
		return fmt.Sprintf("[hrpay] Rate limit exceeded. Retry after %ds", e.RetryAfterSeconds)
	}
	return "[hrpay] Rate limit exceeded"
}

// Wallet Error (402)
type WalletError struct {
	SDKError
}

func (e *WalletError) Error() string {
	return "[hrpay] Wallet error: " + e.Message
}

// Circuit Breaker Error
type CircuitBreakerOpenError struct {
	SDKError
}

func (e *CircuitBreakerOpenError) Error() string {
	return "[hrpay] Circuit breaker is open. Request blocked to prevent cascading failures."
}

// Webhook Signature Error
type WebhookSignatureError struct {
	SDKError
}

func (e *WebhookSignatureError) Error() string {
	return "[hrpay] Webhook signature verification failed: " + e.Message
}

// Unknown Error
type UnknownError struct {
	SDKError
	Cause error
}

func (e *UnknownError) Error() string {
	if e.Cause != nil {
		return "[hrpay] Unknown error: " + e.Message + " (Cause: " + e.Cause.Error() + ")"
	}
	return "[hrpay] Unknown error: " + e.Message
}

// Helper structure to parse API error payloads.
// The documented envelope is {"error": "CODE", "message": "...", "details": {...}}.
// "code" is accepted as a legacy fallback.
type apiErrorPayload struct {
	Error   string                 `json:"error"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details"`
	Code    string                 `json:"code"`
}

// legacyCodeAliases maps error codes emitted by pre-v1 backends onto the
// canonical codes documented in the v1 error table.
var legacyCodeAliases = map[string]string{
	"invalid_request":            "VALIDATION_ERROR",
	"insufficient_balance":       "WALLET_BALANCE_INSUFFICIENT",
	"duplicate_transaction":      "IDEMPOTENCY_KEY_CONFLICT",
	"idempotency_conflict":       "IDEMPOTENCY_KEY_CONFLICT",
	"kyc_required":               "KYC_NOT_APPROVED",
	"operator_unavailable":       "OPERATOR_NOT_AVAILABLE",
	"service_unavailable":        "PROVIDER_NOT_CONFIGURED",
	"missing_api_key":            "MISSING_API_KEY",
	"invalid_api_key":            "INVALID_API_KEY",
	"missing_transaction_token":  "MISSING_TRANSACTION_TOKEN",
	"invalid_transaction_token":  "INVALID_TRANSACTION_TOKEN",
	"rate_limit_exceeded":        "RATE_LIMIT_EXCEEDED",
	"internal_error":             "INTERNAL_ERROR",
}

// normalizeCode resolves a legacy lowercase code to its canonical form.
func normalizeCode(code string) string {
	if canonical, ok := legacyCodeAliases[strings.ToLower(code)]; ok {
		return canonical
	}
	return code
}

func ParseApiError(statusCode int, body []byte) error {
	var payload apiErrorPayload
	_ = json.Unmarshal(body, &payload)

	// The machine-readable code lives in "error"; fall back to legacy "code".
	code := payload.Error
	if code == "" {
		code = payload.Code
	}
	code = normalizeCode(code)

	message := payload.Message
	if message == "" {
		message = fmt.Sprintf("HTTP error status %d", statusCode)
	}

	base := SDKError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Details:    payload.Details,
		RawBody:    body,
	}

	// Default the code from the status when the server omitted it.
	if base.Code == "" {
		base.Code = defaultCodeForStatus(statusCode)
	}

	switch statusCode {
	case 401, 403:
		return &AuthenticationError{SDKError: base}
	case 402:
		return &WalletError{SDKError: base}
	case 409:
		return &ConflictError{SDKError: base}
	case 429:
		return &RateLimitError{SDKError: base}
	case 400, 422:
		// Try to parse validation issues if any exist in raw body
		type issueWrapper struct {
			Issues []ValidationIssue `json:"issues"`
		}
		var wrapper issueWrapper
		_ = json.Unmarshal(body, &wrapper)
		return &ValidationError{
			SDKError: base,
			Issues:   wrapper.Issues,
		}
	default:
		return &ApiError{SDKError: base}
	}
}

// defaultCodeForStatus maps an HTTP status to the documented error code used
// when the response body carries no explicit code.
func defaultCodeForStatus(statusCode int) string {
	switch statusCode {
	case 400:
		return ErrValidation.Error()
	case 401:
		return ErrInvalidAPIKey.Error()
	case 402:
		return ErrWalletBalanceInsufficient.Error()
	case 403:
		return ErrKycNotApproved.Error()
	case 409:
		return ErrIdempotencyKeyConflict.Error()
	case 422:
		return ErrOperatorNotAvailable.Error()
	case 429:
		return ErrRateLimitExceeded.Error()
	case 500:
		return ErrInternalError.Error()
	case 503:
		return ErrProviderNotConfigured.Error()
	case 504:
		return ErrProviderTimeout.Error()
	default:
		return "API_ERROR"
	}
}
