package hrpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuthManager_GetToken(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		if r.URL.Path != PathAuthToken {
			t.Errorf("expected path %s, got %s", PathAuthToken, r.URL.Path)
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Errorf("expected Bearer authorization header, got %s", authHeader)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"transaction_token": "mock_token_xyz_12345",
			"expires_in": 2700
		}`))
	}))
	defer server.Close()

	ctx := context.Background()
	am := NewAuthManager(AuthManagerOptions{
		PublicKey:  "hrsk_pk_test_PUBLIC_KEY",
		SecretKey:  "hrsk_sk_test_SECRET_KEY",
		BaseURL:    server.URL,
		Timeout:    5 * time.Second,
		TokenCache: NewInMemoryTokenCache(),
		HTTPClient: server.Client(),
	})

	// First call - should hit the server
	token, err := am.GetToken(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "mock_token_xyz_12345" {
		t.Errorf("expected mock_token_xyz_12345, got %s", token)
	}

	// Second call - should hit cache
	token, err = am.GetToken(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "mock_token_xyz_12345" {
		t.Errorf("expected mock_token_xyz_12345, got %s", token)
	}

	mu.Lock()
	actualCalls := callCount
	mu.Unlock()

	if actualCalls != 1 {
		t.Errorf("expected exactly 1 API call, got %d", actualCalls)
	}
}

func TestClient_MiddlewarePipeline(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock token endpoint
		if r.URL.Path == PathAuthToken {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"transaction_token": "token_123", "expires_in": 2700}`))
			return
		}

		// Mock wallet balance endpoint
		if r.URL.Path == PathWalletBalance {
			auth := r.Header.Get("Authorization")
			token := r.Header.Get("X-Transaction-Token")
			if auth != "Bearer hrsk_pk_test_KEY" || token != "token_123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {
					"account_type": "test",
					"balance": {
						"available": 1000,
						"held": 0,
						"total": 1000
					},
					"currency": "XAF",
					"environment": "TEST"
				}
			}`))
			return
		}
	}))
	defer server.Close()

	client, err := NewClient(
		WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var customMiddlewareRun bool
	client.Use(func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		customMiddlewareRun = true
		return next(ctx, mctx)
	})

	ctx := context.Background()
	balance, err := client.Wallet.Balance(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if balance.Balance.Available != 1000 {
		t.Errorf("expected available balance 1000, got %f", balance.Balance.Available)
	}

	if !customMiddlewareRun {
		t.Error("custom middleware was not executed")
	}
}

func TestCashIn_MobileMoney(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathAuthToken {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"transaction_token": "token_123", "expires_in": 2700}`))
			return
		}

		if r.URL.Path == PathCashInMobileMoney {
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)

			if payload["operator"] != "ORANGE" || payload["phone_number"] != "237655500393" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			// Exact 202 payload from the API documentation (§3).
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {
					"transaction_id": "a959b6ca-a5e6-4485-8f92-0747a574b54e",
					"reference": "ref_d5b40df948dc52cc",
					"status": "PENDING",
					"type": "CASHIN",
					"amount": 5000,
					"fee": 75,
					"fee_percent": 1.5,
					"net_amount": 4925,
					"currency": "XAF",
					"operator": "ORANGE",
					"phone_number": "237655500393",
					"initiated_at": "2026-06-07T17:00:59Z",
					"pending_action": "GET /v1/payments/ref_d5b40df948dc52cc"
				}
			}`))
			return
		}
	}))
	defer server.Close()

	client, err := NewClient(
		WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	res, err := client.CashIn.MobileMoney(ctx, CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    OperatorOrange,
		Amount:      5000,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Reference != "ref_d5b40df948dc52cc" {
		t.Errorf("expected reference ref_d5b40df948dc52cc, got %s", res.Reference)
	}
	if res.Status != StatusPending {
		t.Errorf("expected status PENDING, got %s", res.Status)
	}
	// Regression: the field is "fee", not "fees". The old tag never unmarshalled.
	if res.Fee != 75 {
		t.Errorf("expected fee 75, got %f", res.Fee)
	}
	if res.FeePercent != FeePercentCashIn {
		t.Errorf("expected fee_percent 1.5, got %f", res.FeePercent)
	}
	if res.NetAmount != 4925 {
		t.Errorf("expected net_amount 4925, got %f", res.NetAmount)
	}
	if res.TransactionID != "a959b6ca-a5e6-4485-8f92-0747a574b54e" {
		t.Errorf("unexpected transaction_id %q", res.TransactionID)
	}
	if res.Type != "CASHIN" {
		t.Errorf("expected type CASHIN, got %s", res.Type)
	}
}

func TestWebhooks_VerifySignature(t *testing.T) {
	client, _ := NewClient(
		WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"),
	)

	payload := `{"id":"evt_123","type":"payment.succeeded"}`
	secret := "webhook_secret_key"

	// Compute the expected digest the same way the API does.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	digest := hex.EncodeToString(mac.Sum(nil))

	t.Run("accepts the documented sha256= prefixed header", func(t *testing.T) {
		if !client.Webhooks.VerifySignature(payload, "sha256="+digest, secret) {
			t.Error("expected prefixed signature to verify")
		}
	})

	t.Run("accepts a bare hex digest", func(t *testing.T) {
		if !client.Webhooks.VerifySignature(payload, digest, secret) {
			t.Error("expected bare digest to verify")
		}
	})

	t.Run("rejects a tampered payload", func(t *testing.T) {
		if client.Webhooks.VerifySignature(`{"id":"evt_999"}`, "sha256="+digest, secret) {
			t.Error("expected tampered payload to be rejected")
		}
	})

	t.Run("rejects a wrong secret", func(t *testing.T) {
		if client.Webhooks.VerifySignature(payload, "sha256="+digest, "wrong_secret") {
			t.Error("expected wrong secret to be rejected")
		}
	})
}

func TestWebhooks_ConstructEvent(t *testing.T) {
	client, _ := NewClient(WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"))

	payload := `{"id":"evt_123","type":"payment.succeeded","created_at":"2026-06-07T17:00:59Z"}`
	secret := "whsec_test"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	event, err := client.Webhooks.ConstructEvent(payload, sig, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != EventPaymentSucceeded {
		t.Errorf("expected type %s, got %s", EventPaymentSucceeded, event.Type)
	}

	// A bad signature must surface as *WebhookSignatureError.
	_, err = client.Webhooks.ConstructEvent(payload, "sha256=deadbeef", secret)
	var sigErr *WebhookSignatureError
	if !errors.As(err, &sigErr) {
		t.Errorf("expected *WebhookSignatureError, got %T (%v)", err, err)
	}
}

func TestClient_BusinessErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathAuthToken {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"transaction_token": "token_123", "expires_in": 2700}`))
			return
		}

		if r.URL.Path == PathWalletBalance {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired) // 402
			// Documented envelope: {"error": "CODE", "message": "...", "details": {...}}
			_, _ = w.Write([]byte(`{
				"error": "WALLET_BALANCE_INSUFFICIENT",
				"message": "Solde insuffisant pour cette opération",
				"details": {"shortfall": 2500}
			}`))
			return
		}
	}))
	defer server.Close()

	client, err := NewClient(
		WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	_, err = client.Wallet.Balance(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// The canonical sentinel must match.
	if !errors.Is(err, ErrWalletBalanceInsufficient) {
		t.Errorf("expected ErrWalletBalanceInsufficient, got %v", err)
	}
	// The deprecated alias must keep working.
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("expected legacy alias ErrInsufficientBalance to match, got %v", err)
	}

	// details must be surfaced (e.g. shortfall on a 402).
	var walletErr *WalletError
	if !errors.As(err, &walletErr) {
		t.Fatalf("expected *WalletError, got %T", err)
	}
	if got := walletErr.Details["shortfall"]; got != float64(2500) {
		t.Errorf("expected details.shortfall = 2500, got %v", got)
	}
}

// The code must be read from the "error" field of the envelope, not "code".
// Each case deliberately uses a code that differs from the status default, so
// that a regression cannot be masked by defaultCodeForStatus.
func TestParseApiError_ReadsCodeFromErrorField(t *testing.T) {
	cases := []struct {
		status   int
		body     string
		sentinel error
		name     string
	}{
		// 403 defaults to KYC_NOT_APPROVED — WALLET_FROZEN proves "error" is read.
		{403, `{"error":"WALLET_FROZEN","message":"gelé"}`, ErrWalletFrozen, "wallet frozen"},
		// 422 defaults to OPERATOR_NOT_AVAILABLE — CURRENCY_MISMATCH proves it too.
		{422, `{"error":"CURRENCY_MISMATCH","message":"devise"}`, ErrCurrencyMismatch, "currency mismatch"},
		// 400 defaults to VALIDATION_ERROR.
		{400, `{"error":"MISSING_REQUIRED_FIELD","message":"champ"}`, ErrMissingRequiredField, "missing field"},
		// 401 defaults to INVALID_API_KEY.
		{401, `{"error":"MISSING_TRANSACTION_TOKEN","message":"token"}`, ErrMissingTransactionToken, "missing token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ParseApiError(tc.status, []byte(tc.body))
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("expected %v, got %v", tc.sentinel, err)
			}
		})
	}
}

// The legacy "code" field is still honoured as a fallback when "error" is absent.
func TestParseApiError_LegacyCodeFieldFallback(t *testing.T) {
	// 400 defaults to VALIDATION_ERROR, so OPERATOR_NOT_AVAILABLE can only come
	// from the "code" field being read.
	err := ParseApiError(400, []byte(`{"code":"OPERATOR_NOT_AVAILABLE","message":"nope"}`))
	if !errors.Is(err, ErrOperatorNotAvailable) {
		t.Errorf("legacy code field should be honoured, got %v", err)
	}
}

// The pre-v1 backend emitted lowercase codes. They must still resolve.
// 400 defaults to VALIDATION_ERROR, so a match on OPERATOR_NOT_AVAILABLE proves
// the lowercase value was both read and normalized.
func TestParseApiError_LegacyCodeNormalization(t *testing.T) {
	err := ParseApiError(400, []byte(`{"error":"operator_unavailable","message":"nope"}`))
	if !errors.Is(err, ErrOperatorNotAvailable) {
		t.Errorf("legacy lowercase code should normalize, got %v", err)
	}

	err = ParseApiError(409, []byte(`{"error":"duplicate_transaction","message":"dup"}`))
	if !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Errorf("duplicate_transaction should normalize, got %v", err)
	}
}

// When the body carries no code, it is derived from the HTTP status.
func TestParseApiError_DefaultsCodeFromStatus(t *testing.T) {
	cases := map[int]error{
		401: ErrInvalidAPIKey,
		402: ErrWalletBalanceInsufficient,
		403: ErrKycNotApproved,
		409: ErrIdempotencyKeyConflict,
		422: ErrOperatorNotAvailable,
		429: ErrRateLimitExceeded,
		503: ErrProviderNotConfigured,
		504: ErrProviderTimeout,
	}
	for status, sentinel := range cases {
		err := ParseApiError(status, []byte(`{}`))
		if !errors.Is(err, sentinel) {
			t.Errorf("status %d: expected %v, got %v", status, sentinel, err)
		}
	}
}

// A 503 on a VAS endpoint is the documented "deploy requis" state.
func TestParseApiError_ProviderNotConfigured(t *testing.T) {
	err := ParseApiError(503, []byte(`{"error":"PROVIDER_NOT_CONFIGURED","message":"Service VAS non disponible"}`))
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("expected ErrProviderNotConfigured, got %v", err)
	}
}

func TestClient_PanicRecovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathAuthToken {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"transaction_token": "token_123", "expires_in": 2700}`))
			return
		}
		if r.URL.Path == PathWalletBalance {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"available": 10}}`))
			return
		}
	}))
	defer server.Close()

	client, err := NewClient(
		WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Custom middleware that panics
	client.Use(func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		panic("something went critically wrong in middleware")
	})

	ctx := context.Background()
	_, err = client.Wallet.Balance(ctx)
	if err == nil {
		t.Fatal("expected error from panic recovery, got nil")
	}

	unknownErr, ok := err.(*UnknownError)
	if !ok {
		t.Fatalf("expected error to be of type *UnknownError, got %T (%v)", err, err)
	}

	if !strings.Contains(unknownErr.Message, "Panic recovered") {
		t.Errorf("expected message to mention Panic recovered, got: %s", unknownErr.Message)
	}
}
