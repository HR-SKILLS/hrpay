package hrpay

import "time"

// Configuration defaults
const (
	DefaultBaseURL           = "https://api.hrskills-pay.com"
	DefaultTimeout           = 30 * time.Second
	DefaultMaxRetries        = 3
	TokenTTL                 = 45 * time.Minute
	TokenExpiryMargin        = 60 * time.Second
	DefaultPollInterval      = 3 * time.Second
	DefaultPollTimeout       = 10 * time.Minute
	HeaderAuthorization      = "Authorization"
	HeaderTransactionToken   = "X-Transaction-Token"
	HeaderIdempotencyKey     = "Idempotency-Key"
	HeaderContentType        = "Content-Type"
	HeaderUserAgent          = "User-Agent"
	MimeJSON                 = "application/json"
)

// Transaction statuses
const (
	StatusPending  = "PENDING"  // awaiting customer confirmation or network processing
	StatusSuccess  = "SUCCESS"  // confirmed — wallet credited or debited
	StatusFailed   = "FAILED"   // failure or 10 min timeout
	StatusHold     = "HOLD"     // blocked by AML, under manual review
	StatusRefunded = "REFUNDED" // refunded (Cash-In only)
)

// Terminal transaction statuses. HOLD is intentionally excluded: it denotes an
// AML review still in progress, so polling must keep waiting.
var TerminalStatuses = map[string]bool{
	StatusSuccess:  true,
	StatusFailed:   true,
	StatusRefunded: true,
}

// Webhook event types.
const (
	EventPaymentSucceeded = "payment.succeeded"
	EventPaymentFailed    = "payment.failed"
	EventPaymentHold      = "payment.hold"
	EventPaymentRefunded  = "payment.refunded"
)

// Transaction fees, as a percentage of the amount.
const (
	FeePercentCashIn  = 1.5
	FeePercentCashOut = 1.0
)

// Minimum Cash-In amount, in local currency.
const MinCashInAmount = 100

// Funds credited by a Cash-In stay in balance.held for this duration.
const CashInHoldDuration = 48 * time.Hour

// Maximum page size accepted by list endpoints.
const MaxPageLimit = 100

// Default reseller commission rates per VAS service, as a percentage.
var DefaultCommissionRates = map[VasService]float64{
	VasServiceAirtime:   3.0,
	VasServiceData:      3.0,
	VasServiceEneo:      1.5,
	VasServiceCamwater:  1.5,
	VasServiceCanalPlus: 2.0,
	VasServiceCustoms:   1.0,
}

// Operators supported across the 16 covered countries.
type Operator string

const (
	OperatorOrange    Operator = "ORANGE"
	OperatorMtn       Operator = "MTN"
	OperatorMoov      Operator = "MOOV"
	OperatorAirtel    Operator = "AIRTEL"
	OperatorMpesa     Operator = "MPESA"
	OperatorWave      Operator = "WAVE"
	OperatorFree      Operator = "FREE"
	OperatorTmoney    Operator = "TMONEY"
	OperatorAfrimoney Operator = "AFRIMONEY"
	OperatorCamtel    Operator = "CAMTEL"
	OperatorNexttel   Operator = "NEXTTEL"
	OperatorCoris     Operator = "CORIS"
	OperatorExpresso  Operator = "EXPRESSO"
	OperatorFlooz     Operator = "FLOOZ"
	OperatorQmoney    Operator = "QMONEY"
)

// Currencies used by Mobile Money across covered countries.
// USD/EUR remain available for virtual cards.
type Currency string

const (
	CurrencyXaf Currency = "XAF" // CM, GA
	CurrencyXof Currency = "XOF" // SN, CI, ML, BF, TG, BJ
	CurrencyCdf Currency = "CDF" // CD
	CurrencyGnf Currency = "GNF" // GN
	CurrencyGmd Currency = "GMD" // GM
	CurrencyUsd Currency = "USD"
	CurrencyEur Currency = "EUR"
)

// Countries (ISO 3166-1 alpha-2) covered by the API.
type Country string

const (
	CountryCm Country = "CM" // Cameroun
	CountrySn Country = "SN" // Sénégal
	CountryCi Country = "CI" // Côte d'Ivoire
	CountryGa Country = "GA" // Gabon
	CountryCd Country = "CD" // RD Congo
	CountryMl Country = "ML" // Mali
	CountryBf Country = "BF" // Burkina Faso
	CountryTg Country = "TG" // Togo
	CountryBj Country = "BJ" // Bénin
	CountryGn Country = "GN" // Guinée
	CountryGm Country = "GM" // Gambie
)

// CurrencyForCountry maps a country code to its local Mobile Money currency.
// Used to derive the currency when the caller omits it.
var CurrencyForCountry = map[Country]Currency{
	CountryCm: CurrencyXaf,
	CountryGa: CurrencyXaf,
	CountrySn: CurrencyXof,
	CountryCi: CurrencyXof,
	CountryMl: CurrencyXof,
	CountryBf: CurrencyXof,
	CountryTg: CurrencyXof,
	CountryBj: CurrencyXof,
	CountryCd: CurrencyCdf,
	CountryGn: CurrencyGnf,
	CountryGm: CurrencyGmd,
}

// currencyForCountry returns the local currency for a country, defaulting to
// XAF when the country is unknown.
func currencyForCountry(country Country) Currency {
	if cur, ok := CurrencyForCountry[country]; ok {
		return cur
	}
	return CurrencyXaf
}

// API Path Constants
const (
	// Auth
	PathAuthToken = "/v1/auth/transaction-token"

	// Cash In
	PathCashInMobileMoney = "/api/v1/payin/mobile-money"
	PathCashInInitiate    = "/v1/payments/initiate"

	// Cash Out
	PathCashOutMobileMoney = "/api/v1/payout/mobile-money"

	// Payments / Transactions
	PathPaymentStatus    = "/v1/payments"
	PathTransactionsList = "/v1/transactions"
	PathTransactionDetail = "/v1/transactions"

	// Wallet
	PathWalletBalance   = "/v1/balance"
	PathWalletsAlias    = "/api/v1/wallets" // alias of /v1/balance
	PathWalletMovements = "/v1/wallet/movements"

	// Airtime
	PathAirtimeRecharge = "/api/v1/airtime/recharge"
	PathAirtimeBatch    = "/api/v1/airtime/batch"
	PathAirtimeOffers   = "/api/v1/airtime/offers"

	// Data
	PathDataPackages = "/api/v1/data/packages"
	PathDataSend     = "/api/v1/data/send"

	// Bills — ENEO
	PathBillsEneoInvoice  = "/api/v1/bills/eneo/invoice"
	PathBillsEneoPrepaid  = "/api/v1/bills/eneo/prepaid"
	PathBillsEneoPostpaid = "/api/v1/bills/eneo/postpaid"

	// Bills — CAMWATER
	PathBillsCamwaterInvoice = "/api/v1/bills/camwater/invoice"
	PathBillsCamwaterPay     = "/api/v1/services/camwater"

	// Bills — Canal+
	PathBillsCanalPlusPay = "/api/v1/services/canalplus"

	// Bills — Customs (Douanes)
	PathBillsCustomsGet = "/api/v1/bills/customs"
	PathBillsCustomsPay = "/api/v1/bills/customs/pay"

	// VAS Commissions
	PathVasRates              = "/v1/vas/rates"
	PathVasCommissions        = "/v1/vas/commissions"
	PathVasCommissionsSummary = "/v1/vas/commissions/summary"

	// Payroll
	PathPayrollImport   = "/api/v1/payroll/import"
	PathPayrollBatch    = "/api/v1/payroll/batch"
	PathPayrollBatches  = "/api/v1/payroll/batches"

	// Virtual Cards
	PathVirtualCards = "/api/v1/virtual-cards"

	// Analytics
	PathStatsSummary      = "/api/v1/stats/summary"
	PathStatsTransactions = "/api/v1/stats/transactions"
	PathStatsRevenue      = "/api/v1/stats/revenue"

	// Payment Links
	PathPaymentLinks = "/api/v1/payment-links"

	// Webhooks
	PathWebhooksEvents = "/v1/webhooks/events"
)
