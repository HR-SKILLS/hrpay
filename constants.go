package hrpay

import "time"

// Configuration defaults
const (
	DefaultBaseURL      = "https://api.hrskills-pay.com"
	DefaultTimeout      = 30 * time.Second
	DefaultMaxRetries   = 3
	TokenTTL            = 45 * time.Minute
	TokenExpiryMargin   = 60 * time.Second
	DefaultPollInterval = 3 * time.Second
	DefaultPollTimeout  = 10 * time.Minute
	// DefaultKYCPollInterval is used by CardCustomers.PollEnrollment. KYC is
	// reviewed manually and is never instant — polling every few seconds like
	// DefaultPollInterval would hammer the API for no benefit.
	DefaultKYCPollInterval = 5 * time.Minute
	HeaderAuthorization    = "Authorization"
	HeaderTransactionToken = "X-Transaction-Token"
	HeaderIdempotencyKey   = "Idempotency-Key"
	HeaderContentType      = "Content-Type"
	HeaderUserAgent        = "User-Agent"
	MimeJSON               = "application/json"
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

	// Virtual card events. Never fired for is_test:true resources.
	EventCardCreated             = "card.created"
	EventCardFunded              = "card.funded"
	EventCardWithdrawn           = "card.withdrawn"
	EventCardTerminated          = "card.terminated"
	EventCardTransactionApproved = "card.transaction.approved"
	EventCardTransactionDeclined = "card.transaction.declined"
)

// Numeric Mobile Money provider refusal codes surfaced on a CASHOUT as HTTP
// 422 "cashout_refused". These are reference constants only — read the
// humanized SDKError.Message rather than switching on the raw numeric code,
// which arrives (if at all) in SDKError.Details.
const (
	CashoutRefusalClientConfirmationTimeout    = 703201
	CashoutRefusalClientRejected               = 703202
	CashoutRefusalInvalidPIN                   = 703203
	CashoutRefusalInsufficientBeneficiaryFunds = 703108
	CashoutRefusalAccountNotActivated          = 703117
	CashoutRefusalAmountAboveThreshold         = 702103
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

// Operators supported across the 16 covered Mobile Money countries, plus a
// handful of additional telco brands (Camtel, Nexttel, Flooz, Qmoney) used
// only by the Airtime/Data/Payroll (VAS) domains — NOT valid Mobile Money
// cashin/cashout operators. See MobileMoneyOperatorsByCountry for the set
// that is actually valid per country for cashin/cashout.
type Operator string

const (
	OperatorOrange     Operator = "ORANGE"
	OperatorMtn        Operator = "MTN"
	OperatorMoov       Operator = "MOOV"
	OperatorAirtel     Operator = "AIRTEL"
	OperatorMpesa      Operator = "MPESA"
	OperatorWave       Operator = "WAVE"
	OperatorFree       Operator = "FREE"
	OperatorTmoney     Operator = "TMONEY"
	OperatorAfrimoney  Operator = "AFRIMONEY"
	OperatorWligdicash Operator = "WLIGDICASH"
	OperatorCeltiis    Operator = "CELTIIS"
	OperatorCoris      Operator = "CORIS"
	OperatorExpresso   Operator = "EXPRESSO"

	// Not part of the Mobile Money cashin/cashout vocabulary — used by
	// airtime.go/data.go/payroll.go only.
	OperatorCamtel  Operator = "CAMTEL"
	OperatorNexttel Operator = "NEXTTEL"
	OperatorFlooz   Operator = "FLOOZ"
	OperatorQmoney  Operator = "QMONEY"
)

// Currencies used by Mobile Money across covered countries.
// USD/EUR remain available for virtual cards.
type Currency string

const (
	CurrencyXaf Currency = "XAF" // CM, GA, CG, TD, CF
	CurrencyXof Currency = "XOF" // SN, CI, ML, BF, TG, BJ, NE, GW
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
	CountryGa Country = "GA" // Gabon
	CountryCg Country = "CG" // Congo Brazzaville
	CountryTd Country = "TD" // Tchad
	CountryCf Country = "CF" // République Centrafricaine
	CountryCi Country = "CI" // Côte d'Ivoire
	CountrySn Country = "SN" // Sénégal
	CountryMl Country = "ML" // Mali
	CountryBf Country = "BF" // Burkina Faso
	CountryTg Country = "TG" // Togo
	CountryBj Country = "BJ" // Bénin
	CountryNe Country = "NE" // Niger
	CountryGw Country = "GW" // Guinée-Bissau
	CountryCd Country = "CD" // RD Congo
	CountryGn Country = "GN" // Guinée Conakry
	CountryGm Country = "GM" // Gambie
)

// CurrencyForCountry maps a country code to its local Mobile Money currency.
// Used to derive the currency when the caller omits it.
var CurrencyForCountry = map[Country]Currency{
	CountryCm: CurrencyXaf,
	CountryGa: CurrencyXaf,
	CountryCg: CurrencyXaf,
	CountryTd: CurrencyXaf,
	CountryCf: CurrencyXaf,
	CountryCi: CurrencyXof,
	CountrySn: CurrencyXof,
	CountryMl: CurrencyXof,
	CountryBf: CurrencyXof,
	CountryTg: CurrencyXof,
	CountryBj: CurrencyXof,
	CountryNe: CurrencyXof,
	CountryGw: CurrencyXof,
	CountryCd: CurrencyCdf,
	CountryGn: CurrencyGnf,
	CountryGm: CurrencyGmd,
}

// MobileMoneyOperatorsByCountry documents, per country, the operators valid
// for cashin/cashout as of this writing. This is a reference/UX convenience
// (e.g. building a picker) — the server remains the source of truth, and the
// live catalogue (including per-merchant activation) is only available via
// the dashboard-JWT-authenticated GET /v1/countries/supported, which this
// SDK does not call. Do not treat this map as a hard client-side gate.
var MobileMoneyOperatorsByCountry = map[Country][]Operator{
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

// OtpRequiredOperators documents, per country, which operators additionally
// require the customer to confirm via OTP. Reference/UX only — see the note
// on MobileMoneyOperatorsByCountry above.
var OtpRequiredOperators = map[Country][]Operator{
	CountryCi: {OperatorOrange},
	CountrySn: {OperatorOrange},
	CountryBf: {OperatorOrange, OperatorWligdicash},
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

	// Cash Out
	PathCashOutMobileMoney = "/api/v1/payout/mobile-money"

	// Payments / Transactions
	// Refund hits PathPaymentStatus + "/" + reference + "/refund".
	PathPaymentStatus     = "/v1/payments"
	PathPaymentFees       = "/v1/payments/fees"
	PathTransactionsList  = "/v1/transactions"
	PathTransactionDetail = "/v1/transactions"

	// Wallet
	PathWalletBalance   = "/v1/wallet/balance"
	PathWalletsAlias    = "/api/v1/wallets" // legacy/unverified alias, unused by any service method today
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
	PathPayrollImport  = "/api/v1/payroll/import"
	PathPayrollBatch   = "/api/v1/payroll/batch"
	PathPayrollBatches = "/api/v1/payroll/batches"

	// Virtual Cards
	PathVirtualCards       = "/api/v1/virtual-cards"
	PathCardCustomers      = "/api/v1/card-customers"
	PathCardWallet         = "/api/v1/card-wallet"
	PathCardWalletQuote    = "/api/v1/card-wallet/quote"
	PathCardWalletFund     = "/api/v1/card-wallet/fund"
	PathCardWalletWithdraw = "/api/v1/card-wallet/withdraw"
	PathCardPricing        = "/api/v1/card-pricing"

	// Analytics
	PathStatsSummary      = "/api/v1/stats/summary"
	PathStatsTransactions = "/api/v1/stats/transactions"
	PathStatsRevenue      = "/api/v1/stats/revenue"

	// Payment Links
	PathPaymentLinks = "/api/v1/payment-links"

	// Webhooks
	PathWebhooksEvents = "/v1/webhooks/events"
)

// ─── Virtual Cards enums ────────────────────────────────────────────

// CardBrand identifies the card network for a virtual card.
type CardBrand string

const (
	CardBrandVisa       CardBrand = "VISA"
	CardBrandMastercard CardBrand = "MASTERCARD"
)

// IDDocumentType is the kind of identity document supplied for card
// customer KYC.
type IDDocumentType string

const (
	IDDocumentNIN            IDDocumentType = "NIN"
	IDDocumentPassport       IDDocumentType = "PASSPORT"
	IDDocumentVotersCard     IDDocumentType = "VOTERS_CARD"
	IDDocumentDriversLicense IDDocumentType = "DRIVERS_LICENSE"
)

// KYCStatus tracks a card customer through manual KYC review and Cartevo
// enrollment.
type KYCStatus string

const (
	KYCStatusPendingReview    KYCStatus = "PENDING_REVIEW"
	KYCStatusEnrolling        KYCStatus = "ENROLLING"
	KYCStatusEnrolled         KYCStatus = "ENROLLED" // only status allowing card issuance
	KYCStatusRejectedProvider KYCStatus = "REJECTED_PROVIDER"
	KYCStatusRejectedLocal    KYCStatus = "REJECTED_LOCAL"
)

// TerminalKYCStatuses are the states CardCustomers.PollEnrollment stops at.
var TerminalKYCStatuses = map[KYCStatus]bool{
	KYCStatusEnrolled:         true,
	KYCStatusRejectedProvider: true,
	KYCStatusRejectedLocal:    true,
}

// CardStatus is the lifecycle state of a virtual card.
type CardStatus string

const (
	CardStatusPending    CardStatus = "PENDING"
	CardStatusActive     CardStatus = "ACTIVE"
	CardStatusFrozen     CardStatus = "FROZEN"
	CardStatusSuspended  CardStatus = "SUSPENDED" // Cartevo-initiated, read-only from this API
	CardStatusTerminated CardStatus = "TERMINATED"
	CardStatusFailed     CardStatus = "FAILED"
)

// CardTransactionCategory is always "CARD" today; kept typed for clarity.
type CardTransactionCategory string

const CardTransactionCategoryCard CardTransactionCategory = "CARD"

// CardTransactionType enumerates the kinds of entries returned by
// VirtualCardsService.Transactions.
type CardTransactionType string

const (
	CardTxCreate        CardTransactionType = "CREATE" // local/sandbox log only
	CardTxAuthorization CardTransactionType = "AUTHORIZATION"
	CardTxSettlement    CardTransactionType = "SETTLEMENT"
	CardTxFunding       CardTransactionType = "FUNDING"
	CardTxWithdrawal    CardTransactionType = "WITHDRAWAL"
	CardTxDecline       CardTransactionType = "DECLINE"
	CardTxReversal      CardTransactionType = "REVERSAL"
	CardTxRefund        CardTransactionType = "REFUND"
	CardTxCrossBorder   CardTransactionType = "CROSS-BORDER" // literal hyphenated wire value
	CardTxTermination   CardTransactionType = "TERMINATION"
)
