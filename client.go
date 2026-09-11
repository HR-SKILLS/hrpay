package hrpay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hr-skills/hrpay/internal/breaker"
	"github.com/hr-skills/hrpay/internal/transport"
)

type Client struct {
	config          Config
	transportClient *transport.TransportClient
	pipeline        *MiddlewarePipeline
	authManager     *AuthManager
	circuitBreaker  *breaker.CircuitBreaker

	// Callbacks
	onRequest  func(*MiddlewareContext)
	onResponse func(*transport.Response)
	onError    func(error, *MiddlewareContext)

	// Services
	Auth         *AuthService
	CashIn       *CashInService
	CashOut      *CashOutService
	Transactions *TransactionsService
	Wallet       *WalletService
	Fees         *FeesService
	Airtime      *AirtimeService
	Data         *DataService
	Bills        *BillsService
	Commissions  *CommissionsService
	Payroll      *PayrollService
	Cards        *CardsService
	Analytics    *AnalyticsService
	PaymentLinks *PaymentLinksService
	Webhooks     *WebhooksService
}

func NewClient(opts ...Option) (*Client, error) {
	config := Config{
		Timeout:    DefaultTimeout,
		MaxRetries: DefaultMaxRetries,
	}

	for _, opt := range opts {
		opt(&config)
	}

	if config.PublicKey == "" || config.SecretKey == "" {
		return nil, errors.New("[hrpay] Both publicKey and secretKey are required")
	}

	// Sandbox (hrsk_pk_test_) and production (hrsk_pk_live_) share the same
	// base URL — the environment is determined by the key, not the host.
	// In sandbox, even amounts settle to SUCCESS and odd amounts to FAILED.
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}

	if config.TokenCache == nil {
		config.TokenCache = NewInMemoryTokenCache()
	}

	tc := transport.NewTransportClient(config.HTTPClient)
	pipeline := NewMiddlewarePipeline()

	authMgr := NewAuthManager(AuthManagerOptions{
		PublicKey:  config.PublicKey,
		SecretKey:  config.SecretKey,
		BaseURL:    config.BaseURL,
		Timeout:    config.Timeout,
		TokenCache: config.TokenCache,
		HTTPClient: config.HTTPClient,
	})

	cb := breaker.NewCircuitBreaker(breaker.Config{
		FailureThreshold: 5,
		ResetTimeout:     15 * time.Second,
	})

	c := &Client{
		config:          config,
		transportClient: tc,
		pipeline:        pipeline,
		authManager:     authMgr,
		circuitBreaker:  cb,
		onRequest:       config.OnRequest,
		onResponse:      config.OnResponse,
		onError:         config.OnError,
	}

	// Register built-in middlewares
	// 1. Logger
	if config.Logger.Requests || config.Logger.Responses || config.Logger.Errors {
		pipeline.Use(createLoggerMiddleware(config.Logger))
	}

	// 2. Throttle
	if config.Throttle.Enabled {
		pipeline.Use(createThrottleMiddleware(config.Throttle.MinDelay))
	}

	// 3. Retry
	pipeline.Use(createRetryMiddleware(config.MaxRetries))

	// 4. Auth
	pipeline.Use(createAuthMiddleware(config.SecretKey, authMgr))

	// 5. Idempotence
	pipeline.Use(createIdempotencyMiddleware())

	// 6. Callback events
	pipeline.Use(createEventMiddleware(c))

	// Initialize public services
	c.Auth = &AuthService{authManager: authMgr}
	c.CashIn = &CashInService{client: c}
	c.CashOut = &CashOutService{client: c}
	c.Transactions = &TransactionsService{client: c}
	c.Wallet = &WalletService{client: c}
	c.Airtime = &AirtimeService{client: c}
	c.Data = &DataService{client: c}
	c.Bills = &BillsService{
		Eneo:      &EneoModule{client: c},
		Camwater:  &CamwaterModule{client: c},
		CanalPlus: &CanalPlusModule{client: c},
		Customs:   &CustomsModule{client: c},
	}
	c.Commissions = &CommissionsService{client: c}
	c.Payroll = &PayrollService{client: c}
	c.Cards = &CardsService{
		Customers: &CardCustomersService{client: c},
		Wallet:    &CardWalletService{client: c},
		Virtual:   &VirtualCardsService{client: c},
	}
	c.Analytics = &AnalyticsService{client: c}
	c.PaymentLinks = &PaymentLinksService{client: c}
	c.Webhooks = &WebhooksService{client: c}
	c.Fees = &FeesService{client: c}

	return c, nil
}

// Register custom middleware to the pipeline.
func (c *Client) Use(middleware Middleware) {
	c.pipeline.Use(middleware)
}

// requestOptions carries per-call deviations from the default request
// behavior. The zero value (authModeSecretAndToken) is what nearly every
// service method wants, so request() below stays the common-case entry point.
type requestOptions struct {
	AuthMode authMode
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, params map[string]string) (resBody []byte, finalErr error) {
	return c.requestWithOptions(ctx, method, path, body, params, requestOptions{})
}

func (c *Client) requestWithOptions(ctx context.Context, method, path string, body []byte, params map[string]string, opts requestOptions) (resBody []byte, finalErr error) {
	defer func() {
		if r := recover(); r != nil {
			finalErr = &UnknownError{
				SDKError: SDKError{
					Message: fmt.Sprintf("Panic recovered in request flow: %v", r),
				},
				Cause: fmt.Errorf("%v", r),
			}
		}
	}()

	// Construct full URL
	baseURL := strings.TrimSuffix(c.config.BaseURL, "/")
	cleanPath := path
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	// Simple query params injection
	var queryStr string
	if len(params) > 0 {
		var parts []string
		for k, v := range params {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		queryStr = "?" + strings.Join(parts, "&")
	}
	fullURL := baseURL + cleanPath + queryStr

	headers := map[string]string{
		HeaderContentType: MimeJSON,
		HeaderUserAgent:   "HRSkills-Go/1.0.0 Go/1.22",
	}

	mctx := &MiddlewareContext{
		Method:   method,
		URL:      fullURL,
		Headers:  headers,
		Body:     body,
		Meta:     make(map[string]interface{}),
		AuthMode: opts.AuthMode,
	}

	// Composed pipeline execution with transport client inside circuit breaker
	execute := c.pipeline.Compose(func(cx context.Context, mc *MiddlewareContext) error {
		opts := transport.RequestOptions{
			Method:  mc.Method,
			URL:     mc.URL,
			Headers: mc.Headers,
			Body:    mc.Body,
			Timeout: c.config.Timeout,
		}

		err := c.circuitBreaker.Execute(func() error {
			resp, err := c.transportClient.Execute(cx, opts)
			if err != nil {
				return err
			}
			mc.Response = resp
			return nil
		}, func(e error) bool {
			// Decide system failures for Circuit Breaker
			if e == nil {
				return false
			}
			if e == breaker.ErrCircuitOpen {
				return false
			}
			// Check network/timeout or 5xx server status codes
			if strings.Contains(strings.ToLower(e.Error()), "timeout") ||
				strings.Contains(strings.ToLower(e.Error()), "connection") ||
				strings.Contains(strings.ToLower(e.Error()), "refused") ||
				strings.Contains(strings.ToLower(e.Error()), "dial") {
				return true
			}
			// If we got a response but it's 5xx
			if mctx.Response != nil && mctx.Response.StatusCode >= 500 {
				return true
			}
			return false
		})

		return err
	})

	err := execute(ctx, mctx)
	if err != nil {
		if err == breaker.ErrCircuitOpen {
			return nil, &CircuitBreakerOpenError{}
		}
		// Wrap standard context timeouts/cancels as TimeoutError/SDKError
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &TimeoutError{TimeoutMs: c.config.Timeout.Milliseconds()}
		}
		if errors.Is(err, context.Canceled) {
			return nil, &NetworkError{SDKError: SDKError{Message: "Request cancelled"}}
		}
		return nil, &NetworkError{SDKError: SDKError{Message: "Request failed"}, Cause: err}
	}

	if mctx.Response == nil {
		return nil, &UnknownError{SDKError: SDKError{Message: "Empty response from transport pipeline"}}
	}

	// Check if status is not success
	if mctx.Response.StatusCode < 200 || mctx.Response.StatusCode >= 300 {
		return nil, ParseApiError(mctx.Response.StatusCode, mctx.Response.Body)
	}

	return mctx.Response.Body, nil
}
