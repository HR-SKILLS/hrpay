package hrpay

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hr-skills/hrpay/internal/transport"
)

// authMode selects which credentials createAuthMiddleware attaches to a request.
type authMode int

const (
	// authModeSecretAndToken sends Authorization: Bearer <SecretKey> plus
	// X-Transaction-Token. This is the mode nearly every payment/resource
	// endpoint requires, and the zero value so existing call sites need no change.
	authModeSecretAndToken authMode = iota
	// authModeSecretOnly sends Authorization: Bearer <SecretKey> with no
	// X-Transaction-Token header at all (e.g. GET /v1/wallet/balance).
	authModeSecretOnly
)

type MiddlewareContext struct {
	Method   string
	URL      string
	Headers  map[string]string
	Body     []byte
	Response *transport.Response
	Meta     map[string]interface{}
	AuthMode authMode
}

type Next func(ctx context.Context, mctx *MiddlewareContext) error
type Middleware func(ctx context.Context, mctx *MiddlewareContext, next Next) error

type MiddlewarePipeline struct {
	mu          sync.RWMutex
	middlewares []Middleware
}

func NewMiddlewarePipeline() *MiddlewarePipeline {
	return &MiddlewarePipeline{}
}

func (p *MiddlewarePipeline) Use(m Middleware) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.middlewares = append(p.middlewares, m)
}

func (p *MiddlewarePipeline) GetMiddlewares() []Middleware {
	p.mu.RLock()
	defer p.mu.RUnlock()
	copied := make([]Middleware, len(p.middlewares))
	copy(copied, p.middlewares)
	return copied
}

func (p *MiddlewarePipeline) Compose(handler func(ctx context.Context, mctx *MiddlewareContext) error) func(ctx context.Context, mctx *MiddlewareContext) error {
	p.mu.RLock()
	middlewares := p.middlewares
	p.mu.RUnlock()

	return func(ctx context.Context, mctx *MiddlewareContext) error {
		var dispatch func(int) error
		dispatch = func(i int) error {
			if i < len(middlewares) {
				return middlewares[i](ctx, mctx, func(c context.Context, mc *MiddlewareContext) error {
					return dispatch(i + 1)
				})
			}
			return handler(ctx, mctx)
		}
		return dispatch(0)
	}
}

// ─── Built-in Middlewares ──────────────────────────────────────────

func redactSensitive(text string) string {
	// Redact keys like hrsk_pk_test_... and hrsk_sk_test_...
	re := regexp.MustCompile(`(hrsk_(?:pk|sk)_(?:live|test)_\w{4})\w+(\w{4})`)
	return re.ReplaceAllString(text, "$1...$2")
}

func createLoggerMiddleware(config LoggerConfig) Middleware {
	logFn := config.Log
	if logFn == nil {
		logFn = func(msg string) {
			fmt.Println(msg)
		}
	}

	return func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		start := time.Now()
		redactedURL := redactSensitive(mctx.URL)

		if config.Requests {
			logFn(fmt.Sprintf("[HRSkillsPay] → %s %s", mctx.Method, redactedURL))
		}

		err := next(ctx, mctx)
		duration := time.Since(start)

		if err != nil {
			if config.Errors {
				redactedErr := redactSensitive(err.Error())
				logFn(fmt.Sprintf("[HRSkillsPay] ✖ %s %s FAILED [%v]: %s", mctx.Method, redactedURL, duration, redactedErr))
			}
			return err
		}

		if config.Responses && mctx.Response != nil {
			logFn(fmt.Sprintf("[HRSkillsPay] ← %d %s [%v]", mctx.Response.StatusCode, mctx.Response.Status, duration))
		}

		return nil
	}
}

func createThrottleMiddleware(minDelay time.Duration) Middleware {
	var mu sync.Mutex
	var lastRequestTime time.Time

	return func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		mu.Lock()
		now := time.Now()
		timeSinceLast := now.Sub(lastRequestTime)

		if timeSinceLast < minDelay {
			delay := minDelay - timeSinceLast
			mu.Unlock() // Unlock before sleep to not block other concurrent requests from queuing up
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			mu.Lock()
		}

		lastRequestTime = time.Now()
		mu.Unlock()

		return next(ctx, mctx)
	}
}

func createRetryMiddleware(maxRetries int) Middleware {
	retryableStatuses := map[int]bool{
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusServiceUnavailable:  true,
		http.StatusGatewayTimeout:      true,
	}

	retryDelays := []time.Duration{
		500 * time.Millisecond,
		1000 * time.Millisecond,
		2000 * time.Millisecond,
		4000 * time.Millisecond,
	}

	return func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		attempt := 0

		for {
			err := next(ctx, mctx)

			// Determine if response requires a retry
			status := 0
			if mctx.Response != nil {
				status = mctx.Response.StatusCode
			}

			isRetryableStatus := status != 0 && retryableStatuses[status]
			isRetryableErr := err != nil && isRetryableError(err)

			if (isRetryableStatus || isRetryableErr) && attempt < maxRetries {
				delay := retryDelays[attempt]
				if attempt >= len(retryDelays) {
					delay = retryDelays[len(retryDelays)-1]
				}

				if status == http.StatusTooManyRequests && mctx.Response != nil {
					retryAfterHeader := mctx.Response.Headers["Retry-After"]
					if retryAfterHeader != "" {
						if seconds, err := strconv.Atoi(retryAfterHeader); err == nil {
							delay = time.Duration(seconds) * time.Second
						}
					}
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}

				// Clear response and retry
				mctx.Response = nil
				attempt++
				continue
			}

			return err
		}
	}
}

func isRetryableError(err error) bool {
	if _, ok := err.(*RateLimitError); ok {
		return true
	}
	// Also retry on connection errors
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "fetch") ||
		strings.Contains(errStr, "dial")
}

// createAuthMiddleware attaches merchant credentials to every request.
// secretKey (Clé B) always goes in Authorization — payment/resource endpoints
// require the secret key here, never the public key (Clé A is only used to
// obtain the transaction token itself, in auth.go, which bypasses this
// middleware entirely). X-Transaction-Token is added unless the call opted
// into authModeSecretOnly (e.g. wallet balance).
func createAuthMiddleware(secretKey string, authManager *AuthManager) Middleware {
	return func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		mctx.Headers[HeaderAuthorization] = fmt.Sprintf("Bearer %s", secretKey)

		if mctx.AuthMode == authModeSecretOnly {
			return next(ctx, mctx)
		}

		token, err := authManager.GetToken(ctx)
		if err != nil {
			return err
		}
		mctx.Headers[HeaderTransactionToken] = token
		return next(ctx, mctx)
	}
}

func createIdempotencyMiddleware() Middleware {
	mutatingMethods := map[string]bool{
		"POST":  true,
		"PATCH": true,
		"PUT":   true,
	}

	return func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		method := strings.ToUpper(mctx.Method)
		if mutatingMethods[method] {
			if _, exists := mctx.Headers[HeaderIdempotencyKey]; !exists {
				if key, ok := idempotencyKeyFromContext(ctx); ok {
					mctx.Headers[HeaderIdempotencyKey] = key
				} else {
					mctx.Headers[HeaderIdempotencyKey] = uuid.New().String()
				}
			}
		}
		return next(ctx, mctx)
	}
}

func createEventMiddleware(client *Client) Middleware {
	return func(ctx context.Context, mctx *MiddlewareContext, next Next) error {
		if client.onRequest != nil {
			client.onRequest(mctx)
		}

		err := next(ctx, mctx)

		if err != nil {
			if client.onError != nil {
				client.onError(err, mctx)
			}
			return err
		}

		if client.onResponse != nil && mctx.Response != nil {
			client.onResponse(mctx.Response)
		}

		return nil
	}
}
