package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	if auth := headers.Get(HeaderAuthorization); auth != "Bearer hrsk_pk_test_KEY" {
		t.Errorf("unexpected Authorization header %q", auth)
	}
}

// ─── §3 Cash-In ────────────────────────────────────────────────────

func TestCashIn_MinimumAmountIs100(t *testing.T) {
	client, _, _ := captureBody(t, 202, `{"success":true,"data":{"reference":"ref_1"}}`)

	_, err := client.CashIn.MobileMoney(context.Background(), CashInMobileMoneyParams{
		PhoneNumber: "237655500393",
		Operator:    OperatorOrange,
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
		Amount:      100,
	}); err != nil {
		t.Errorf("amount of exactly 100 should be valid, got %v", err)
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
		{CountrySn, CurrencyXof},
		{CountryCi, CurrencyXof},
		{CountryMl, CurrencyXof},
		{CountryBf, CurrencyXof},
		{CountryTg, CurrencyXof},
		{CountryBj, CurrencyXof},
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
		PhoneNumber: "237655500393", Operator: OperatorOrange, Amount: 5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if even.Status != StatusSuccess {
		t.Errorf("even amount should settle to SUCCESS, got %s", even.Status)
	}

	odd, err := client.CashIn.MobileMoney(ctx, CashInMobileMoneyParams{
		PhoneNumber: "237655500393", Operator: OperatorOrange, Amount: 5001,
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
