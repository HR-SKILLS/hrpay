package hrpay

import (
	"time"

	"github.com/hr-skills/hrpay/internal/transport"
)

// LoggerConfig defines outgoing and incoming HTTP logging parameters.
type LoggerConfig struct {
	Requests  bool
	Responses bool
	Errors    bool
	Log       func(string)
}

// ThrottleConfig controls the rate of outgoing requests.
type ThrottleConfig struct {
	Enabled    bool
	MinDelay   time.Duration
}

// Config wraps all customization parameters.
type Config struct {
	PublicKey  string
	SecretKey  string
	BaseURL    string
	Timeout    time.Duration
	Logger     LoggerConfig
	Throttle   ThrottleConfig
	MaxRetries int
	TokenCache TokenCache
	HTTPClient transport.HTTPClient

	// Callbacks
	OnRequest  func(*MiddlewareContext)
	OnResponse func(*transport.Response)
	OnError    func(error, *MiddlewareContext)
}

type Option func(*Config)

// WithAPIKeys configures the required credentials.
func WithAPIKeys(publicKey, secretKey string) Option {
	return func(c *Config) {
		c.PublicKey = publicKey
		c.SecretKey = secretKey
	}
}

// WithTimeout sets a custom request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithMaxRetries sets the max number of automatic retries.
func WithMaxRetries(maxRetries int) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
	}
}

// WithLogger configures logging.
func WithLogger(logger LoggerConfig) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithThrottle enables request rate limiting.
func WithThrottle(minDelay time.Duration) Option {
	return func(c *Config) {
		c.Throttle = ThrottleConfig{
			Enabled:  true,
			MinDelay: minDelay,
		}
	}
}

// WithTokenCache configures the persistent cache storage for transaction tokens.
func WithTokenCache(cache TokenCache) Option {
	return func(c *Config) {
		c.TokenCache = cache
	}
}

// WithHTTPClient allows dependency injection of custom transport.
func WithHTTPClient(client transport.HTTPClient) Option {
	return func(c *Config) {
		c.HTTPClient = client
	}
}

// WithOnRequest sets a callback triggered before sending requests.
func WithOnRequest(cb func(*MiddlewareContext)) Option {
	return func(c *Config) {
		c.OnRequest = cb
	}
}

// WithOnResponse sets a callback triggered upon receiving responses.
func WithOnResponse(cb func(*transport.Response)) Option {
	return func(c *Config) {
		c.OnResponse = cb
	}
}

// WithOnError sets a callback triggered on request failures.
func WithOnError(cb func(error, *MiddlewareContext)) Option {
	return func(c *Config) {
		c.OnError = cb
	}
}

// WithBaseURL overrides the production endpoint (used for sandbox or testing).
func WithBaseURL(url string) Option {
	return func(c *Config) {
		c.BaseURL = url
	}
}
