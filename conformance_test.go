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
	"testing"
	"time"
)

// newTestServer spins up a mock API that always serves the token endpoint and
// delegates everything else to the supplied handler.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathAuthToken {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"transaction_token": "token_123",
				"expires_in": 2700,
				"merchant_id": "e6af1e82-0000-0000-0000-000000000000",
				"environment": "TEST"
			}`))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(
		WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return server, client
}

// captureBody records the JSON body of the first non-auth request.
func captureBody(t *testing.T, status int, response string) (*Client, *map[string]interface{}, *http.Header) {
	t.Helper()
	captured := make(map[string]interface{})
	var headers http.Header

	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	})
	return client, &captured, &headers
}

// ─── §2 Authentification ───────────────────────────────────────────

// Sandbox and production share the same host; only the key differs.
func TestSandboxUsesSameBaseURLAsLive(t *testing.T) {
	testClient, err := NewClient(WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	liveClient, err := NewClient(WithAPIKeys("hrsk_pk_live_KEY", "hrsk_sk_live_SECRET"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if testClient.config.BaseURL != DefaultBaseURL {
		t.Errorf("sandbox base URL should be %s, got %s", DefaultBaseURL, testClient.config.BaseURL)
	}
	if liveClient.config.BaseURL != DefaultBaseURL {
		t.Errorf("live base URL should be %s, got %s", DefaultBaseURL, liveClient.config.BaseURL)
	}
	if strings.Contains(testClient.config.BaseURL, "/sandbox") {
		t.Error("sandbox must not use a /sandbox path segment")
	}
}

func TestAuth_ExposesMerchantIDAndEnvironment(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := client.Auth.GetToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := client.Auth.MerchantID(); got != "e6af1e82-0000-0000-0000-000000000000" {
		t.Errorf("unexpected merchant_id %q", got)
	}
	if got := client.Auth.Environment(); got != "TEST" {
		t.Errorf("expected environment TEST, got %q", got)
	}
}

// Idempotency-Key is mandatory on POST.
func TestIdempotencyKeySentOnPost(t *testing.T) {
	client, _, headers := captureBody(t, 202, `{"success":true,"data":{"reference":"ref_1","status":"PENDING"}}`)

	_, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    OperatorOrange,
		Country:     CountryCm,
		Amount:      5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key := headers.Get(HeaderIdempotencyKey); key == "" {
		t.Error("expected an Idempotency-Key header on POST")
	}
	if tok := headers.Get(HeaderTransactionToken); tok != "token_123" {
		t.Errorf("expected X-Transaction-Token header, got %q", tok)
	}
	// Payment endpoints authenticate with the SECRET key (Clé B), never the
	// public key — this used to assert the public key, encoding a real bug.
	if auth := headers.Get(HeaderAuthorization); auth != "Bearer hrsk_sk_test_SECRET" {
		t.Errorf("unexpected Authorization header %q", auth)
	}
}

// ─── §3 Cash-In ────────────────────────────────────────────────────

func TestCashIn_MinimumAmountIs100(t *testing.T) {
	client, _, _ := captureBody(t, 202, `{"success":true,"data":{"reference":"ref_1"}}`)

	_, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    OperatorOrange,
		Country:     CountryCm,
		Amount:      99,
	})

	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError for amount < 100, got %T (%v)", err, err)
	}

	// 100 exactly must be accepted.
	if _, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    OperatorOrange,
		Country:     CountryCm,
		Amount:      100,
	}); err != nil {
		t.Errorf("amount of exactly 100 should be valid, got %v", err)
	}
}

// Country is required — the SDK must never silently default it, since
// currency is always derived server-side from the country.
func TestCashIn_CountryIsRequired(t *testing.T) {
	client, _, _ := captureBody(t, 202, `{"success":true,"data":{"reference":"ref_1"}}`)

	_, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    OperatorOrange,
		Amount:      5000,
	})
	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError when Country is omitted, got %T (%v)", err, err)
	}
}

// ─── §13 Opérateurs & Pays ─────────────────────────────────────────

// The currency must be derived from the country when omitted.
func TestCurrencyDerivedFromCountry(t *testing.T) {
	cases := []struct {
		country  Country
		expected Currency
	}{
		{CountryCm, CurrencyXaf},
		{CountryGa, CurrencyXaf},
		{CountryCg, CurrencyXaf},
		{CountryTd, CurrencyXaf},
		{CountryCf, CurrencyXaf},
		{CountrySn, CurrencyXof},
		{CountryCi, CurrencyXof},
		{CountryMl, CurrencyXof},
		{CountryBf, CurrencyXof},
		{CountryTg, CurrencyXof},
		{CountryBj, CurrencyXof},
		{CountryNe, CurrencyXof},
		{CountryGw, CurrencyXof},
		{CountryCd, CurrencyCdf},
		{CountryGn, CurrencyGnf},
		{CountryGm, CurrencyGmd},
	}

	for _, tc := range cases {
		if got := currencyForCountry(tc.country); got != tc.expected {
			t.Errorf("country %s: expected %s, got %s", tc.country, tc.expected, got)
		}
	}

	// Unknown country falls back to XAF.
	if got := currencyForCountry(Country("ZZ")); got != CurrencyXaf {
		t.Errorf("unknown country should fall back to XAF, got %s", got)
	}
}

func TestCashIn_SendsDerivedCurrencyForSenegal(t *testing.T) {
	client, body, _ := captureBody(t, 202, `{"success":true,"data":{"reference":"ref_sn"}}`)

	_, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "221771234567",
		Operator:    OperatorWave,
		Country:     CountrySn,
		Amount:      5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if (*body)["currency"] != "XOF" {
		t.Errorf("expected XOF for Senegal, got %v", (*body)["currency"])
	}
	if (*body)["operator"] != "WAVE" {
		t.Errorf("expected WAVE operator, got %v", (*body)["operator"])
	}
	if (*body)["country"] != "SN" {
		t.Errorf("expected SN, got %v", (*body)["country"])
	}
}

// An explicit currency must not be overridden.
func TestCashIn_ExplicitCurrencyWins(t *testing.T) {
	client, body, _ := captureBody(t, 202, `{"success":true,"data":{"reference":"ref_x"}}`)

	_, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    OperatorMtn,
		Country:     CountryCm,
		Currency:    CurrencyXaf,
		Amount:      1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*body)["currency"] != "XAF" {
		t.Errorf("expected XAF, got %v", (*body)["currency"])
	}
}

// ─── §5 Statuts ────────────────────────────────────────────────────

// HOLD means "AML review in progress" and must NOT terminate polling.
func TestHoldIsNotATerminalStatus(t *testing.T) {
	if TerminalStatuses[StatusHold] {
		t.Error("HOLD must not be terminal — it is an in-progress AML review")
	}
	for _, s := range []string{StatusSuccess, StatusFailed, StatusRefunded} {
		if !TerminalStatuses[s] {
			t.Errorf("%s must be terminal", s)
		}
	}
	if TerminalStatuses[StatusPending] {
		t.Error("PENDING must not be terminal")
	}
}

// limit is capped at 100 by the API.
func TestTransactions_ListLimitCappedAt100(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"meta":{"total":0}}`))
	})

	_, err := client.Transactions.List(context.Background(), TransactionListParams{Limit: 101})
	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError for limit > 100, got %T (%v)", err, err)
	}

	if _, err := client.Transactions.List(context.Background(), TransactionListParams{Limit: 100}); err != nil {
		t.Errorf("limit of exactly 100 should be valid, got %v", err)
	}
}

// ─── §6 Solde ──────────────────────────────────────────────────────

func TestWallet_ParsesHolds(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Exact shape from the documentation (§6).
		_, _ = w.Write([]byte(`{
			"success": true,
			"data": {
				"balance": {"available": 45000, "held": 5000, "total": 50000},
				"holds": [{"amount": 5000, "available_at": "2026-06-09T17:01:26Z"}],
				"stats_today": {"cashin_count": 3, "cashin_volume": 30000, "cashout_count": 1, "cashout_volume": 10000},
				"limits": {"daily_cashin_limit": 500000, "daily_cashout_limit": 200000}
			}
		}`))
	})

	balance, err := client.Wallet.Balance(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if balance.Balance.Available != 45000 || balance.Balance.Held != 5000 || balance.Balance.Total != 50000 {
		t.Errorf("unexpected balance %+v", balance.Balance)
	}
	if len(balance.Holds) != 1 {
		t.Fatalf("expected 1 hold, got %d", len(balance.Holds))
	}
	if balance.Holds[0].Amount != 5000 {
		t.Errorf("expected hold amount 5000, got %f", balance.Holds[0].Amount)
	}
	if balance.Holds[0].AvailableAt != "2026-06-09T17:01:26Z" {
		t.Errorf("unexpected available_at %q", balance.Holds[0].AvailableAt)
	}
	if balance.Limits == nil || balance.Limits.DailyCashInLimit != 500000 {
		t.Error("expected limits.daily_cashin_limit = 500000")
	}
}

// ─── §7 Services VAS ───────────────────────────────────────────────

// The doc is explicit: field is "meter", NOT "meter_number".
func TestEneo_SendsMeterNotMeterNumber(t *testing.T) {
	client, body, _ := captureBody(t, 200, `{"success":true,"data":{"reference":"ref_eneo"}}`)

	_, err := client.Bills.Eneo.Prepaid(context.Background(), EneoPrepaidParams{
		Meter:  "12345678",
		Amount: 5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if (*body)["meter"] != "12345678" {
		t.Errorf("expected meter field, got %v", (*body)["meter"])
	}
	if _, present := (*body)["meter_number"]; present {
		t.Error("meter_number must not be sent — the documented field is meter")
	}
	// customer_phone is optional and must be omitted when empty.
	if _, present := (*body)["customer_phone"]; present {
		t.Error("customer_phone must be omitted when not supplied")
	}
}

// customer_phone is optional on every bill endpoint.
func TestBills_CustomerPhoneIsOptional(t *testing.T) {
	client, _, _ := captureBody(t, 200, `{"success":true,"data":{"reference":"r"}}`)
	ctx := context.Background()

	if _, err := client.Bills.Eneo.Prepaid(ctx, EneoPrepaidParams{Meter: "123456", Amount: 1000}); err != nil {
		t.Errorf("ENEO prepaid without phone should be valid: %v", err)
	}
	if _, err := client.Bills.Eneo.Postpaid(ctx, EneoPostpaidParams{Meter: "123456", Amount: 1000}); err != nil {
		t.Errorf("ENEO postpaid without phone should be valid: %v", err)
	}
	if _, err := client.Bills.Camwater.Pay(ctx, CamwaterPayParams{Meter: "CW123", Amount: 1000}); err != nil {
		t.Errorf("CAMWATER without phone should be valid: %v", err)
	}
	if _, err := client.Bills.CanalPlus.Pay(ctx, CanalPlusPayParams{DecoderNumber: "12345678", Amount: 1000}); err != nil {
		t.Errorf("CanalPlus without phone should be valid: %v", err)
	}
	if _, err := client.Bills.Customs.Pay(ctx, CustomsPayParams{DeclarationRef: "DGD-2026-001234", Amount: 1000}); err != nil {
		t.Errorf("Customs without phone should be valid: %v", err)
	}
}

// An invalid phone, when supplied, must still be rejected.
func TestBills_InvalidCustomerPhoneStillRejected(t *testing.T) {
	client, _, _ := captureBody(t, 200, `{"success":true,"data":{}}`)

	_, err := client.Bills.Eneo.Prepaid(context.Background(), EneoPrepaidParams{
		Meter:         "123456",
		Amount:        1000,
		CustomerPhone: "abc",
	})
	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Errorf("expected *ValidationError for a malformed phone, got %T (%v)", err, err)
	}
}

// decoder_number must be 8 to 12 digits.
func TestCanalPlus_DecoderNumberLength(t *testing.T) {
	client, _, _ := captureBody(t, 200, `{"success":true,"data":{}}`)
	ctx := context.Background()

	for _, bad := range []string{"1234567", "1234567890123", "ABCD1234"} {
		_, err := client.Bills.CanalPlus.Pay(ctx, CanalPlusPayParams{DecoderNumber: bad, Amount: 1000})
		var valErr *ValidationError
		if !errors.As(err, &valErr) {
			t.Errorf("decoder %q should be rejected, got %v", bad, err)
		}
	}

	if _, err := client.Bills.CanalPlus.Pay(ctx, CanalPlusPayParams{DecoderNumber: "12345678", Amount: 1000}); err != nil {
		t.Errorf("8-digit decoder should be valid, got %v", err)
	}
}

// offer_code is not part of the v1 contract and must not be sent unless set.
func TestCanalPlus_OmitsOfferCodeByDefault(t *testing.T) {
	client, body, _ := captureBody(t, 200, `{"success":true,"data":{}}`)

	_, err := client.Bills.CanalPlus.Pay(context.Background(), CanalPlusPayParams{
		DecoderNumber: "12345678",
		Amount:        10000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := (*body)["offer_code"]; present {
		t.Error("offer_code must be omitted when unset")
	}
}

// ─── §8 Commissions ────────────────────────────────────────────────

func TestDefaultCommissionRates(t *testing.T) {
	expected := map[VasService]float64{
		VasServiceAirtime:   3.0,
		VasServiceData:      3.0,
		VasServiceEneo:      1.5,
		VasServiceCamwater:  1.5,
		VasServiceCanalPlus: 2.0,
		VasServiceCustoms:   1.0,
	}
	for svc, rate := range expected {
		if got := DefaultCommissionRates[svc]; got != rate {
			t.Errorf("%s: expected %.1f%%, got %.1f%%", svc, rate, got)
		}
	}
}

// ─── §9 Payroll ────────────────────────────────────────────────────

func TestPayroll_ImportSendsRecipients(t *testing.T) {
	client, body, _ := captureBody(t, 200, `{"success":true,"data":{"batch_id":"batch_123"}}`)

	res, err := client.Payroll.Import(context.Background(), PayrollImportParams{
		Label:    "Salaires Juin 2026",
		Currency: CurrencyXaf,
		Recipients: []PayrollRecipient{
			{PhoneNumber: "237670000001", Operator: OperatorMtn, Amount: 150000, Name: "Jean Dupont"},
			{PhoneNumber: "237655000002", Operator: OperatorOrange, Amount: 200000, Name: "Marie Martin"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.BatchID != "batch_123" {
		t.Errorf("expected batch_123, got %s", res.BatchID)
	}

	recipients, ok := (*body)["recipients"].([]interface{})
	if !ok || len(recipients) != 2 {
		t.Fatalf("expected 2 recipients in body, got %v", (*body)["recipients"])
	}
	if (*body)["label"] != "Salaires Juin 2026" {
		t.Errorf("unexpected label %v", (*body)["label"])
	}
}

// ─── Sandbox behaviour: even → SUCCESS, odd → FAILED ───────────────

// In sandbox the outcome is driven by the parity of the amount. This models the
// documented behaviour so integrators can assert against it.
func TestSandbox_AmountParityDrivesOutcome(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Amount float64 `json:"amount"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)

		status := StatusSuccess
		if int64(payload.Amount)%2 != 0 {
			status = StatusFailed
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"success":true,"data":{"reference":"ref_sbx","status":"` + status + `"}}`))
	})

	ctx := context.Background()

	even, err := client.CashIn.MobileMoney(ctx, CashInMobileMoneyParams{
		PhoneNumber: "237655500393", Operator: OperatorOrange, Country: CountryCm, Amount: 5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if even.Status != StatusSuccess {
		t.Errorf("even amount should settle to SUCCESS, got %s", even.Status)
	}

	odd, err := client.CashIn.MobileMoney(ctx, CashInMobileMoneyParams{
		PhoneNumber: "237655500393", Operator: OperatorOrange, Country: CountryCm, Amount: 5001,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if odd.Status != StatusFailed {
		t.Errorf("odd amount should settle to FAILED, got %s", odd.Status)
	}
}

// ─── §12 Webhooks ──────────────────────────────────────────────────

func TestWebhookEventConstants(t *testing.T) {
	expected := map[string]string{
		EventPaymentSucceeded: "payment.succeeded",
		EventPaymentFailed:    "payment.failed",
		EventPaymentHold:      "payment.hold",
		EventPaymentRefunded:  "payment.refunded",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	}
}

func TestCardWebhookEventConstants(t *testing.T) {
	expected := map[string]string{
		EventCardCreated:             "card.created",
		EventCardFunded:              "card.funded",
		EventCardWithdrawn:           "card.withdrawn",
		EventCardTerminated:          "card.terminated",
		EventCardTransactionApproved: "card.transaction.approved",
		EventCardTransactionDeclined: "card.transaction.declined",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("expected %s, got %s", want, got)
		}
	}
}

// ─── §2/§4/§5 Mobile Money auth-mode fix + new endpoints ───────────

// GET /v1/wallet/balance authenticates with the secret key ALONE — no
// X-Transaction-Token — and hits /v1/wallet/balance, not /v1/balance.
func TestWalletBalance_SecretKeyOnlyNoTransactionToken(t *testing.T) {
	var gotPath, auth, token string
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		auth = r.Header.Get(HeaderAuthorization)
		token = r.Header.Get(HeaderTransactionToken)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"currency":"XAF","balance":{"available":1,"held":0,"total":1}}}`))
	})

	if _, err := client.Wallet.Balance(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != PathWalletBalance {
		t.Errorf("expected path %s, got %s", PathWalletBalance, gotPath)
	}
	if auth != "Bearer hrsk_sk_test_SECRET" {
		t.Errorf("expected secret-key Authorization, got %q", auth)
	}
	if token != "" {
		t.Errorf("expected no X-Transaction-Token, got %q", token)
	}
}

func TestCashOutMobileMoney_UsesSecretKeyAndTransactionToken(t *testing.T) {
	client, body, headers := captureBody(t, 200, `{"success":true,"data":{"reference":"ref_co","status":"SUCCESS","type":"CASHOUT"}}`)

	_, err := client.CashOut.MobileMoney(context.Background(), CashOutMobileMoneyParams{
		PhoneNumber: "2250700000001",
		Operator:    OperatorOrange,
		Country:     CountryCi,
		Amount:      25000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers.Get(HeaderAuthorization) != "Bearer hrsk_sk_test_SECRET" {
		t.Errorf("unexpected Authorization header %q", headers.Get(HeaderAuthorization))
	}
	if headers.Get(HeaderTransactionToken) != "token_123" {
		t.Errorf("expected X-Transaction-Token to be present")
	}
	if (*body)["country"] != "CI" {
		t.Errorf("expected country CI, got %v", (*body)["country"])
	}
}

// Response field completeness: every documented field must round-trip.
func TestCashInMobileMoney_ResponseFieldCompleteness(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"message": "CASHIN initié",
			"data": {
				"transaction_id": "b6e2c1a0-0000-0000-0000-000000000000",
				"external_id": "COLL-1757600000-a1b2c3d4",
				"reference": "commande-42891",
				"status": "SUCCESS",
				"type": "CASHIN",
				"amount": 10000,
				"fee": 200,
				"fee_percent": 2,
				"fee_fixed": 0,
				"net_amount": 9800,
				"currency": "XAF",
				"operator": "MTN",
				"country": "CM",
				"phone_number": "+237690000001",
				"otp_required": false,
				"initiated_at": "2026-09-11T10:15:00Z",
				"wallet_balance_before": 150000,
				"wallet_balance_after": 159800,
				"provider": "CARTEVO",
				"provider_ref": "sandbox-a1b2c3d4"
			}
		}`))
	})

	tx, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "690000001", Operator: OperatorMtn, Country: CountryCm, Amount: 10000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tx.ExternalID != "COLL-1757600000-a1b2c3d4" {
		t.Errorf("unexpected external_id %q", tx.ExternalID)
	}
	if tx.Provider != "CARTEVO" || tx.ProviderRef != "sandbox-a1b2c3d4" {
		t.Errorf("unexpected provider fields: %q / %q", tx.Provider, tx.ProviderRef)
	}
	if tx.WalletBalanceBefore != 150000 || tx.WalletBalanceAfter != 159800 {
		t.Errorf("unexpected wallet balances: %f / %f", tx.WalletBalanceBefore, tx.WalletBalanceAfter)
	}
	if tx.OtpRequired {
		t.Error("expected otp_required=false")
	}
	if tx.Country != CountryCm {
		t.Errorf("unexpected country %q", tx.Country)
	}
}

func TestTransactionsStatus_IncludesCompletionFields(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"transaction_id": "b6e2c1a0-0000-0000-0000-000000000000",
			"reference": "commande-42891",
			"type": "CASHIN",
			"status": "FAILED",
			"amount": 10000,
			"fee": 200,
			"net_amount": 9800,
			"currency": "XAF",
			"operator": "MTN",
			"country": "CM",
			"phone_number": "+237690000001",
			"initiated_at": "2026-09-11T10:15:00Z",
			"completed_at": "2026-09-11T10:15:12Z",
			"error_message": "Le client a refusé la demande de paiement.",
			"error_code": "703202",
			"failure_reason_category": "customer_rejected",
			"wallet_balance_before": 150000,
			"wallet_balance_after": 150000
		}`))
	})

	tx, err := client.Transactions.Status(context.Background(), "commande-42891")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.CompletedAt == "" || tx.ErrorMessage == "" || tx.ErrorCode != "703202" || tx.FailureReasonCategory != "customer_rejected" {
		t.Errorf("unexpected completion fields: %+v", tx)
	}
}

func TestTransactionsRefund_SendsIdempotencyKeyAndCorrectAuth(t *testing.T) {
	var headers http.Header
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		if r.URL.Path != PathPaymentStatus+"/commande-42891/refund" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"reference":"commande-42891","status":"REFUNDED"}}`))
	})

	tx, err := client.Transactions.Refund(context.Background(), "commande-42891")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.Status != StatusRefunded {
		t.Errorf("expected REFUNDED, got %s", tx.Status)
	}
	if headers.Get(HeaderIdempotencyKey) == "" {
		t.Error("expected an Idempotency-Key header on refund")
	}
	if headers.Get(HeaderAuthorization) != "Bearer hrsk_sk_test_SECRET" {
		t.Errorf("unexpected Authorization header %q", headers.Get(HeaderAuthorization))
	}
	if headers.Get(HeaderTransactionToken) != "token_123" {
		t.Error("expected X-Transaction-Token on refund")
	}
}

// WithIdempotencyKey lets a caller override the auto-generated UUID, for a
// safe client-side retry.
func TestWithIdempotencyKey_OverridesAutoGenerated(t *testing.T) {
	client, _, headers := captureBody(t, 202, `{"success":true,"data":{"reference":"ref_1","status":"PENDING"}}`)

	ctx := WithIdempotencyKey(context.Background(), "my-custom-retry-key")
	_, err := client.CashIn.MobileMoney(ctx, CashInMobileMoneyParams{
		PhoneNumber: "237655500393", Operator: OperatorOrange, Country: CountryCm, Amount: 5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := headers.Get(HeaderIdempotencyKey); got != "my-custom-retry-key" {
		t.Errorf("expected custom idempotency key, got %q", got)
	}
}

func TestFeesList_ParsesScheduleArray(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PathPaymentFees {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"currency": "XAF",
			"fees": [
				{"country":"CM","country_name":"Cameroon","currency":"XAF","direction":"CASHIN","fee_percent":0.02,"fee_pct":2,"fee_fixed":0,"default_fee_pct":2},
				{"country":"CM","country_name":"Cameroon","currency":"XAF","direction":"CASHOUT","fee_percent":0.02,"fee_pct":2,"fee_fixed":0,"default_fee_pct":2}
			]
		}`))
	})

	res, err := client.Fees.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Fees) != 2 {
		t.Fatalf("expected 2 fee rows, got %d", len(res.Fees))
	}
	if res.Fees[0].FeePct != 2 || res.Fees[0].FeePercent != 0.02 {
		t.Errorf("unexpected fee row %+v", res.Fees[0])
	}
}

// The webhook envelope field is "event", not "type".
func TestWebhookEvent_EnvelopeUsesEventFieldNotType(t *testing.T) {
	client, _ := NewClient(WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"))

	payload := `{"id":"wh_1","event":"payment.succeeded","merchant_id":"m1","created_at":"2026-09-11T10:15:12Z","data":{}}`
	secret := "whsec"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	event, err := client.Webhooks.ConstructEvent(payload, sig, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != "payment.succeeded" {
		t.Errorf("expected Type to decode from the \"event\" field, got %q", event.Type)
	}
	if event.MerchantID != "m1" {
		t.Errorf("expected merchant_id to decode, got %q", event.MerchantID)
	}
}

// Locks in the full 16-country Mobile Money operator table.
func TestMobileMoneyOperatorsByCountry_Covers16Countries(t *testing.T) {
	expected := map[Country][]Operator{
		CountryCm: {OperatorMtn, OperatorOrange},
		CountryGa: {OperatorAirtel, OperatorMoov},
		CountryCg: {OperatorAirtel, OperatorMtn},
		CountryTd: {OperatorAirtel, OperatorMoov},
		CountryCf: {OperatorOrange},
		CountryCi: {OperatorMoov, OperatorMtn, OperatorOrange, OperatorWave},
		CountrySn: {OperatorExpresso, OperatorFree, OperatorOrange, OperatorWave},
		CountryMl: {OperatorMoov, OperatorOrange},
		CountryBf: {OperatorMoov, OperatorOrange, OperatorWligdicash},
		CountryTg: {OperatorMoov, OperatorTmoney},
		CountryBj: {OperatorMoov, OperatorMtn, OperatorCeltiis, OperatorCoris},
		CountryNe: {OperatorAirtel},
		CountryGw: {OperatorOrange},
		CountryCd: {OperatorAirtel, OperatorMpesa, OperatorOrange, OperatorAfrimoney},
		CountryGn: {OperatorMtn, OperatorOrange},
		CountryGm: {OperatorAfrimoney},
	}

	if len(MobileMoneyOperatorsByCountry) != 16 {
		t.Fatalf("expected 16 countries, got %d", len(MobileMoneyOperatorsByCountry))
	}
	for country, ops := range expected {
		got, ok := MobileMoneyOperatorsByCountry[country]
		if !ok {
			t.Errorf("missing country %s", country)
			continue
		}
		if len(got) != len(ops) {
			t.Errorf("%s: expected %v, got %v", country, ops, got)
			continue
		}
		for i := range ops {
			if got[i] != ops[i] {
				t.Errorf("%s: expected %v, got %v", country, ops, got)
				break
			}
		}
	}
}

// Table-driven error-mapping coverage for the new sentinels.
func TestParseApiError_NewMobileMoneySentinels(t *testing.T) {
	cases := []struct {
		status   int
		code     string
		sentinel error
	}{
		{422, "INVALID_AMOUNT", ErrInvalidAmount},
		{402, "WALLET_NOT_FOUND", ErrWalletNotFound},
		{403, "COUNTRY_NOT_ACTIVATED", ErrCountryNotActivated},
		{429, "PLAN_TX_LIMIT_REACHED", ErrPlanTxLimitReached},
		{503, "PROVIDER_UNAVAILABLE", ErrProviderUnavailable},
		{422, "cashout_refused", ErrCashoutRefused},
		{409, "DUPLICATE_REFERENCE", ErrDuplicateReference},
	}
	for _, tc := range cases {
		err := ParseApiError(tc.status, []byte(`{"error":"`+tc.code+`","message":"m"}`))
		if !errors.Is(err, tc.sentinel) {
			t.Errorf("%s: expected %v, got %v", tc.code, tc.sentinel, err)
		}
	}
}

// ─── Virtual Cards — Card Customers (KYC) ──────────────────────────

func TestCardCustomerCreate_ValidatesRequiredFields(t *testing.T) {
	valid := CardCustomerCreateParams{
		FirstName: "Jean", LastName: "Dupont", Email: "jean.dupont@client.cm",
		Country: "Cameroon", CountryIsoCode: "CM", CountryPhoneCode: "+237",
		PhoneNumber: "690001234", Street: "Rue 1.234, Bonanjo", City: "Douala",
		State: "Littoral", PostalCode: "00237", IdentificationNumber: "123456789",
		IDDocumentType: IDDocumentNIN, DateOfBirth: "1990-04-12",
		IDDocumentFront: "data:image/jpeg;base64,AAAA", IDDocumentBack: "data:image/jpeg;base64,AAAA",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected the fully-populated params to be valid, got %v", err)
	}

	mutate := func(f func(*CardCustomerCreateParams)) CardCustomerCreateParams {
		p := valid
		f(&p)
		return p
	}
	badCases := map[string]CardCustomerCreateParams{
		"short first name":     mutate(func(p *CardCustomerCreateParams) { p.FirstName = "Jo" }),
		"email without @":      mutate(func(p *CardCustomerCreateParams) { p.Email = "not-an-email" }),
		"iso code too long":    mutate(func(p *CardCustomerCreateParams) { p.CountryIsoCode = "CMR" }),
		"phone code no plus":   mutate(func(p *CardCustomerCreateParams) { p.CountryPhoneCode = "237" }),
		"phone repeats prefix": mutate(func(p *CardCustomerCreateParams) { p.PhoneNumber = "237690001234" }),
		"bad doc type":         mutate(func(p *CardCustomerCreateParams) { p.IDDocumentType = "SSN" }),
		"bad dob format":       mutate(func(p *CardCustomerCreateParams) { p.DateOfBirth = "12/04/1990" }),
		"under 18":             mutate(func(p *CardCustomerCreateParams) { p.DateOfBirth = "2015-01-01" }),
		"missing front doc":    mutate(func(p *CardCustomerCreateParams) { p.IDDocumentFront = "" }),
	}
	for name, p := range badCases {
		if err := p.Validate(); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestCardCustomerCreate_ParsesPendingReviewResponse(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"customer": {
				"id": "uuid-1", "first_name": "Jean", "last_name": "Dupont",
				"email": "jean.dupont@client.cm", "country": "Cameroon",
				"country_iso_code": "CM", "country_phone_code": "+237",
				"phone_number": "690001234", "street": "Rue 1.234, Bonanjo",
				"city": "Douala", "state": "Littoral", "postal_code": "00237",
				"identification_number": "123456789", "id_document_type": "NIN",
				"date_of_birth": "1990-04-12", "kyc_status": "PENDING_REVIEW",
				"rejection_reason": "", "is_active": true, "is_test": false,
				"enrolled_at": null, "reviewed_at": null,
				"created_at": "2026-05-16T09:00:00+01:00"
			},
			"message": "Client soumis — en attente de validation KYC"
		}`))
	})

	customer, err := client.Cards.Customers.Create(context.Background(), CardCustomerCreateParams{
		FirstName: "Jean", LastName: "Dupont", Email: "jean.dupont@client.cm",
		Country: "Cameroon", CountryIsoCode: "CM", CountryPhoneCode: "+237",
		PhoneNumber: "690001234", Street: "Rue 1.234, Bonanjo", City: "Douala",
		State: "Littoral", PostalCode: "00237", IdentificationNumber: "123456789",
		IDDocumentType: IDDocumentNIN, DateOfBirth: "1990-04-12",
		IDDocumentFront: "data:image/jpeg;base64,AAAA", IDDocumentBack: "data:image/jpeg;base64,AAAA",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if customer.KYCStatus != KYCStatusPendingReview {
		t.Errorf("expected PENDING_REVIEW, got %s", customer.KYCStatus)
	}
	if customer.EnrolledAt != "" || customer.ReviewedAt != "" {
		t.Errorf("expected null enrolled_at/reviewed_at to decode as empty strings")
	}
}

func TestCardCustomers_PollEnrollment_StopsAtTerminalStatus(t *testing.T) {
	for _, terminal := range []KYCStatus{KYCStatusEnrolled, KYCStatusRejectedLocal, KYCStatusRejectedProvider} {
		t.Run(string(terminal), func(t *testing.T) {
			attempts := 0
			_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				attempts++
				status := KYCStatusPendingReview
				if attempts >= 2 {
					status = terminal
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"c1","kyc_status":"` + string(status) + `"}`))
			})

			var statuses []string
			customer, err := client.Cards.Customers.PollEnrollment(context.Background(), "c1", PollOptions{
				Interval: time.Millisecond,
				OnStatus: func(status string, attempt int) { statuses = append(statuses, status) },
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if customer.KYCStatus != terminal {
				t.Errorf("expected %s, got %s", terminal, customer.KYCStatus)
			}
			if len(statuses) != 2 {
				t.Errorf("expected 2 OnStatus calls, got %d (%v)", len(statuses), statuses)
			}
		})
	}
}

func TestTerminalKYCStatuses_ExcludesInProgressStates(t *testing.T) {
	for _, s := range []KYCStatus{KYCStatusPendingReview, KYCStatusEnrolling} {
		if TerminalKYCStatuses[s] {
			t.Errorf("%s must not be terminal", s)
		}
	}
	for _, s := range []KYCStatus{KYCStatusEnrolled, KYCStatusRejectedLocal, KYCStatusRejectedProvider} {
		if !TerminalKYCStatuses[s] {
			t.Errorf("%s must be terminal", s)
		}
	}
}

// ─── Virtual Cards — Wallet & Pricing ───────────────────────────────

func TestCardWallet_ParsesWalletAndPricingExactly(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"wallet": {"id":"uuid","currency":"USD","balance":42.50,"held":0},
			"pricing": {
				"id":"uuid","name":"standard","label":"Forfait standard",
				"is_default":true,"is_active":true,"creation_fee_xaf":1000,
				"fund_fee_fixed_usd":0,"fund_fee_percent":0.01,
				"withdraw_fee_fixed_usd":0,"withdraw_fee_percent":0.01,
				"fx_margin_percent":0.03,"decline_fee_fixed_usd":0.60,
				"decline_fee_margin_percent":0.03,"x_border_fee_fixed_usd":0.60,
				"x_border_fee_margin_percent":0.03,"max_cards_per_customer":3,
				"min_fund_usd":1,"max_fund_usd":2000
			}
		}`))
	})

	res, err := client.Cards.Wallet.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Wallet.Balance != 42.50 {
		t.Errorf("unexpected balance %f", res.Wallet.Balance)
	}
	if res.Pricing.FxMarginPercent != 0.03 || res.Pricing.MaxCardsPerCustomer != 3 {
		t.Errorf("unexpected pricing %+v", res.Pricing)
	}
}

func TestCardWalletQuote_AmbiguousAmountRejectedClientSide(t *testing.T) {
	client, _ := NewClient(WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"))

	_, err := client.Cards.Wallet.Quote(context.Background(), CardWalletQuoteParams{})
	if err == nil {
		t.Fatal("expected an error when neither AmountUSD nor AmountSource is set")
	}
	_, err = client.Cards.Wallet.Quote(context.Background(), CardWalletQuoteParams{AmountUSD: 10, AmountSource: 6490})
	if err == nil {
		t.Fatal("expected an error when both AmountUSD and AmountSource are set")
	}
}

func TestCardWalletFund_NeverSendsAmountXafAlias(t *testing.T) {
	client, body, headers := captureBody(t, 200, `{"amount_usd":10,"amount_xaf":6490,"amount_source":6490,"effective_rate":648.9,"source_currency":"XAF","usd_balance":52.50}`)

	res, err := client.Cards.Wallet.Fund(context.Background(), CardWalletFundParams{AmountUSD: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.USDBalance != 52.50 {
		t.Errorf("unexpected usd_balance %f", res.USDBalance)
	}
	if _, present := (*body)["amount_xaf"]; present {
		t.Error("amount_xaf alias must never be sent — only amount_source")
	}
	if headers.Get(HeaderIdempotencyKey) == "" {
		t.Error("expected an Idempotency-Key header on card-wallet fund")
	}
}

func TestCardPricing_ParsesRateBlock(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"pricing": {"id":"uuid","name":"standard","label":"Forfait standard","is_default":true,"is_active":true,"creation_fee_xaf":1000,"fund_fee_fixed_usd":0,"fund_fee_percent":0.01,"withdraw_fee_fixed_usd":0,"withdraw_fee_percent":0.01,"fx_margin_percent":0.03,"decline_fee_fixed_usd":0.60,"decline_fee_margin_percent":0.03,"x_border_fee_fixed_usd":0.60,"x_border_fee_margin_percent":0.03,"max_cards_per_customer":3,"min_fund_usd":1,"max_fund_usd":2000},
			"rate": {"mid":630,"effective":648.9,"source":"cartevo"}
		}`))
	})

	res, err := client.Cards.Wallet.Pricing(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Rate.Mid != 630 || res.Rate.Effective != 648.9 {
		t.Errorf("unexpected rate %+v", res.Rate)
	}
}

// ─── Virtual Cards — Issuance & PascalCase Decoding ────────────────

// The load-bearing test: the server dumps `card` in PascalCase. Ordinary
// lowercase json tags must still decode it via encoding/json's
// case-insensitive fallback matching.
func TestVirtualCard_DecodesPascalCaseFields(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"card": {
				"ID": "uuid", "MerchantID": "uuid-m", "CustomerID": "uuid-c",
				"FundingWalletAccountID": null, "ExternalID": "cartevo-card-xxxxx",
				"CardNetwork": "VISA", "Last4": "1111", "ExpiryMonth": 6, "ExpiryYear": 2029,
				"Currency": "USD", "Balance": 20, "SpendingLimit": 0, "Status": "ACTIVE",
				"HolderName": "JEAN DUPONT", "HolderEmail": "jean.dupont@client.cm",
				"Label": "Marketing Ads", "IsTeamCard": false, "AssignedToUserID": null,
				"IsTest": false,
				"ProviderData": {"provider":"cartevo","masked_pan":"4111 **** **** 1111","provider_id":"cartevo-card-xxxxx"},
				"FrozenAt": null, "LockedAt": null, "CanceledAt": null,
				"CreatedAt": "2026-05-16T09:00:00+01:00", "UpdatedAt": "2026-05-16T09:00:00+01:00"
			}
		}`))
	})

	res, err := client.Cards.Virtual.Create(context.Background(), VirtualCardCreateParams{CustomerID: "uuid-c", Amount: 20, Label: "Marketing Ads"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pending || res.Card == nil {
		t.Fatal("expected a synchronous card result")
	}
	card := res.Card
	if card.ID != "uuid" || card.CustomerID != "uuid-c" || card.Status != CardStatusActive {
		t.Errorf("unexpected core fields: %+v", card)
	}
	if card.ProviderData == nil || card.ProviderData.MaskedPan != "4111 **** **** 1111" {
		t.Errorf("unexpected provider_data: %+v", card.ProviderData)
	}
	if card.ExpiryMonth != 6 || card.ExpiryYear != 2029 {
		t.Errorf("unexpected expiry: %d/%d", card.ExpiryMonth, card.ExpiryYear)
	}
}

func TestVirtualCardCreate_DefaultsBrandToVisaAndNormalizesMCAlias(t *testing.T) {
	client, body, _ := captureBody(t, 201, `{"card":{"ID":"uuid","Status":"ACTIVE"}}`)

	if _, err := client.Cards.Virtual.Create(context.Background(), VirtualCardCreateParams{CustomerID: "c1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*body)["brand"] != "VISA" {
		t.Errorf("expected default brand VISA, got %v", (*body)["brand"])
	}

	client, body, _ = captureBody(t, 201, `{"card":{"ID":"uuid","Status":"ACTIVE"}}`)
	if _, err := client.Cards.Virtual.Create(context.Background(), VirtualCardCreateParams{CustomerID: "c1", Brand: "MC"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if (*body)["brand"] != "MASTERCARD" {
		t.Errorf("expected MC to normalize to MASTERCARD, got %v", (*body)["brand"])
	}
}

func TestVirtualCardCreate_202PendingFlow(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"card_id":"uuid","status":"PENDING","message":"Création en cours de confirmation"}`))
	})

	res, err := client.Cards.Virtual.Create(context.Background(), VirtualCardCreateParams{CustomerID: "c1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Pending || res.Card != nil || res.CardID != "uuid" {
		t.Errorf("expected a pending result, got %+v", res)
	}
}

func TestVirtualCardTopupAndWithdraw_202PendingFlow(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if strings.HasSuffix(r.URL.Path, "/topup") {
			_, _ = w.Write([]byte(`{"card_id":"uuid","status":"PENDING","message":"Recharge en cours"}`))
		} else {
			_, _ = w.Write([]byte(`{"card_id":"uuid","status":"PENDING","message":"Retrait en cours"}`))
		}
	})

	topup, err := client.Cards.Virtual.Topup(context.Background(), "uuid", VirtualCardTopupParams{Amount: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !topup.Pending {
		t.Error("expected topup Pending=true")
	}

	withdraw, err := client.Cards.Virtual.Withdraw(context.Background(), "uuid", VirtualCardWithdrawParams{Amount: 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !withdraw.Pending {
		t.Error("expected withdraw Pending=true")
	}
}

func TestVirtualCardCreate_CustomerNotEnrolledSurfaces400(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"customer_not_enrolled","message":"Le client n'est pas encore ENROLLED"}`))
	})

	_, err := client.Cards.Virtual.Create(context.Background(), VirtualCardCreateParams{CustomerID: "c1"})
	if !errors.Is(err, ErrCustomerNotEnrolled) {
		t.Errorf("expected ErrCustomerNotEnrolled, got %v", err)
	}
}

// ─── Virtual Cards — Lifecycle & Reveal ─────────────────────────────

func TestVirtualCardGet_RevealAddsSensitiveBlock(t *testing.T) {
	var gotQuery string
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"card":{"ID":"uuid","Status":"ACTIVE"},"sensitive":{"number":"4111111111111111","cvv":"123","expiry_month":6,"expiry_year":2029}}`))
	})

	res, err := client.Cards.Virtual.Get(context.Background(), "uuid", VirtualCardGetParams{Reveal: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "reveal=true") {
		t.Errorf("expected reveal=true in query, got %q", gotQuery)
	}
	if res.Sensitive == nil || res.Sensitive.Number != "4111111111111111" {
		t.Errorf("expected sensitive block to be populated, got %+v", res.Sensitive)
	}
}

func TestVirtualCardCancelAndTerminate_HitDistinctEquivalentPaths(t *testing.T) {
	var gotPath string
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"card_id":"uuid","status":"TERMINATED","refunded":40}`))
	})

	if _, err := client.Cards.Virtual.Cancel(context.Background(), "uuid"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/cancel") {
		t.Errorf("expected Cancel to hit .../cancel, got %s", gotPath)
	}

	if _, err := client.Cards.Virtual.Terminate(context.Background(), "uuid"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/terminate") {
		t.Errorf("expected Terminate to hit .../terminate, got %s", gotPath)
	}
}

// ─── Virtual Cards — 0-Indexed Live Pagination ──────────────────────

func TestVirtualCardTransactions_LocalLogShape(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transactions":[{"id":"uuid","category":"CARD","type":"CREATE","status":"SUCCESS","amount":20,"currency":"USD","description":"Émission carte VISA","created_at":"2026-05-16T09:00:00+01:00"}],"total":1}`))
	})

	res, err := client.Cards.Virtual.Transactions(context.Background(), "uuid", VirtualCardTransactionsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Transactions) != 1 || res.Transactions[0].Card != nil {
		t.Errorf("expected 1 local transaction with no Card ref, got %+v", res.Transactions)
	}
	if res.Page != 0 || res.TotalPages != 0 {
		t.Errorf("local shape should not populate page/total_pages")
	}
}

func TestVirtualCardTransactions_LiveShapeParsesCardAndCustomerRefs(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transactions":[{"id":"txn_uuid","category":"CARD","type":"CROSS-BORDER","status":"SUCCESS","amount":45.00,"currency":"USD","description":"Amazon.com","created_at":"2026-05-16T09:00:00Z","card":{"id":"card_ext_id","masked_pan":"4111 **** **** 1111","brand":"VISA"},"customer":{"id":"cust_ext_id","first_name":"Jean","last_name":"Dupont"}}],"total":12,"page":0,"total_pages":1}`))
	})

	res, err := client.Cards.Virtual.Transactions(context.Background(), "uuid", VirtualCardTransactionsParams{Page: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(res.Transactions))
	}
	tx := res.Transactions[0]
	if tx.Type != CardTxCrossBorder {
		t.Errorf("expected CROSS-BORDER type, got %s", tx.Type)
	}
	if tx.Card == nil || tx.Card.MaskedPan != "4111 **** **** 1111" {
		t.Errorf("expected populated Card ref, got %+v", tx.Card)
	}
	if tx.Customer == nil || tx.Customer.FirstName != "Jean" {
		t.Errorf("expected populated Customer ref, got %+v", tx.Customer)
	}
}

// Regression lock for the documented 0-indexed pagination pitfall: an
// explicit Page: 1 must request page=1, not page=0 — this test exists so a
// future "off-by-one fix" doesn't quietly break live card pagination.
func TestVirtualCardTransactions_PageIsSentVerbatim(t *testing.T) {
	var gotQuery string
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transactions":[],"total":0,"page":1,"total_pages":1}`))
	})

	if _, err := client.Cards.Virtual.Transactions(context.Background(), "uuid", VirtualCardTransactionsParams{Page: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "page=1") {
		t.Errorf("expected page=1 to be sent verbatim, got %q", gotQuery)
	}
}

// ─── Virtual Cards — Error Envelope ──────────────────────────────────

// Card error responses omit "success" entirely — ParseApiError must not
// require it.
func TestParseApiError_CardEnvelopeWithoutSuccessField(t *testing.T) {
	err := ParseApiError(400, []byte(`{"code":"customer_not_enrolled","message":"Le client n'est pas ENROLLED"}`))
	var valErr *ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if !errors.Is(err, ErrCustomerNotEnrolled) {
		t.Errorf("expected ErrCustomerNotEnrolled, got code %q", valErr.Code)
	}
}

// The card domain's 422 "insufficient_balance" intentionally shares the
// mobile-money 402 sentinel — disambiguate via StatusCode, not the sentinel.
func TestParseApiError_CardInsufficientBalanceSharesSentinelWithWallet(t *testing.T) {
	cardErr := ParseApiError(422, []byte(`{"code":"insufficient_balance","message":"wallet USD carte insuffisant"}`))
	if !errors.Is(cardErr, ErrWalletBalanceInsufficient) {
		t.Errorf("expected shared sentinel to match, got %v", cardErr)
	}
	if cardErr.(*ValidationError).StatusCode != 422 {
		t.Errorf("expected StatusCode 422 to disambiguate from the wallet 402 case")
	}

	walletErr := ParseApiError(402, []byte(`{"error":"insufficient_balance","message":"solde XAF insuffisant"}`))
	if !errors.Is(walletErr, ErrWalletBalanceInsufficient) {
		t.Errorf("expected shared sentinel to match, got %v", walletErr)
	}
	if walletErr.(*WalletError).StatusCode != 402 {
		t.Errorf("expected StatusCode 402 for the wallet case")
	}
}

func TestParseApiError_PlanFeatureNotAvailableIsScreamingSnakeCase(t *testing.T) {
	err := ParseApiError(403, []byte(`{"code":"PLAN_FEATURE_NOT_AVAILABLE","message":"non disponible"}`))
	if !errors.Is(err, ErrPlanFeatureNotAvailable) {
		t.Errorf("expected ErrPlanFeatureNotAvailable, got %v", err)
	}
}

func TestWebhooksService_VerifiesCardEventsWithExistingHelper(t *testing.T) {
	client, _ := NewClient(WithAPIKeys("hrsk_pk_test_KEY", "hrsk_sk_test_SECRET"))

	payload := `{"id":"wh_2","event":"card.transaction.declined","created_at":"2026-09-11T10:15:12Z","data":{"card_id":"uuid","amount":45,"currency":"USD","decline_reason":"insufficient_funds"}}`
	secret := "whsec"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	event, err := client.Webhooks.ConstructEvent(payload, sig, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != EventCardTransactionDeclined {
		t.Errorf("expected %s, got %s", EventCardTransactionDeclined, event.Type)
	}
}
