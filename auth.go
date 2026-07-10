package hrpay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hr-skills/hrpay/internal/transport"
)

// TokenCache defines persistence contract for transaction tokens.
type TokenCache interface {
	Get(ctx context.Context, key string) (string, time.Time, error)
	Set(ctx context.Context, key string, token string, expiresAt time.Time) error
	Delete(ctx context.Context, key string) error
}

// ─── InMemoryTokenCache ─────────────────────────────────────────────

type cacheEntry struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type InMemoryTokenCache struct {
	mu    sync.RWMutex
	cache map[string]cacheEntry
}

func NewInMemoryTokenCache() *InMemoryTokenCache {
	return &InMemoryTokenCache{
		cache: make(map[string]cacheEntry),
	}
}

func (c *InMemoryTokenCache) Get(ctx context.Context, key string) (string, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		return "", time.Time{}, errors.New("token cache miss")
	}
	return entry.Token, entry.ExpiresAt, nil
}

func (c *InMemoryTokenCache) Set(ctx context.Context, key string, token string, expiresAt time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = cacheEntry{
		Token:     token,
		ExpiresAt: expiresAt,
	}
	return nil
}

func (c *InMemoryTokenCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, key)
	return nil
}

// ─── FileTokenCache ─────────────────────────────────────────────────

type FileTokenCache struct {
	mu       sync.RWMutex
	filePath string
}

func NewFileTokenCache(filePath string) *FileTokenCache {
	return &FileTokenCache{filePath: filePath}
}

func (c *FileTokenCache) Get(ctx context.Context, key string) (string, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return "", time.Time{}, err
	}

	var cache map[string]cacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return "", time.Time{}, err
	}

	entry, exists := cache[key]
	if !exists {
		return "", time.Time{}, errors.New("token cache miss")
	}

	return entry.Token, entry.ExpiresAt, nil
}

func (c *FileTokenCache) Set(ctx context.Context, key string, token string, expiresAt time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var cache map[string]cacheEntry

	data, err := os.ReadFile(c.filePath)
	if err == nil {
		_ = json.Unmarshal(data, &cache)
	}

	if cache == nil {
		cache = make(map[string]cacheEntry)
	}

	cache[key] = cacheEntry{
		Token:     token,
		ExpiresAt: expiresAt,
	}

	updatedData, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return os.WriteFile(c.filePath, updatedData, 0644)
}

func (c *FileTokenCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cache map[string]cacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return err
	}

	delete(cache, key)

	updatedData, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return os.WriteFile(c.filePath, updatedData, 0644)
}

// ─── AuthManager ───────────────────────────────────────────────────

type AuthManagerOptions struct {
	PublicKey  string
	SecretKey  string
	BaseURL    string
	Timeout    time.Duration
	TokenCache TokenCache
	HTTPClient transport.HTTPClient
}

type AuthManager struct {
	publicKey      string
	secretKey      string
	baseURL        string
	timeout        time.Duration
	cache          TokenCache
	tc             *transport.TransportClient
	mu             sync.Mutex
	refreshChannel chan struct{}

	// Populated from the most recent /auth/transaction-token response.
	metaMu      sync.RWMutex
	merchantID  string
	environment string
}

func NewAuthManager(opts AuthManagerOptions) *AuthManager {
	return &AuthManager{
		publicKey: opts.PublicKey,
		secretKey: opts.SecretKey,
		baseURL:   opts.BaseURL,
		timeout:   opts.Timeout,
		cache:     opts.TokenCache,
		tc:        transport.NewTransportClient(opts.HTTPClient),
	}
}

type tokenResponse struct {
	TransactionToken string `json:"transaction_token"`
	ExpiresIn        int    `json:"expires_in"`
	MerchantID       string `json:"merchant_id"`
	Environment      string `json:"environment"` // LIVE or TEST
}

func (am *AuthManager) cacheKey() string {
	return "tx_token_" + am.publicKey
}

// GetToken returns a valid token from cache or fetches a new one.
// Uses a sync.Mutex logic to prevent concurrent refresh storm.
func (am *AuthManager) GetToken(ctx context.Context) (string, error) {
	key := am.cacheKey()
	token, expiresAt, err := am.cache.Get(ctx, key)
	if err == nil && time.Now().Before(expiresAt) {
		return token, nil
	}

	am.mu.Lock()
	// Double check after lock acquisition
	token, expiresAt, err = am.cache.Get(ctx, key)
	if err == nil && time.Now().Before(expiresAt) {
		am.mu.Unlock()
		return token, nil
	}

	// Fetch new token
	newToken, err := am.fetchNewToken(ctx)
	if err != nil {
		am.mu.Unlock()
		return "", err
	}

	am.mu.Unlock()
	return newToken, nil
}

func (am *AuthManager) Refresh(ctx context.Context) (string, error) {
	_ = am.cache.Delete(ctx, am.cacheKey())
	return am.GetToken(ctx)
}

func (am *AuthManager) ClearCache(ctx context.Context) error {
	return am.cache.Delete(ctx, am.cacheKey())
}

func (am *AuthManager) IsTokenValid(ctx context.Context) bool {
	token, expiresAt, err := am.cache.Get(ctx, am.cacheKey())
	return err == nil && token != "" && time.Now().Before(expiresAt)
}

// MerchantID returns the merchant id reported by the last token exchange.
// Empty until a token has been fetched.
func (am *AuthManager) MerchantID() string {
	am.metaMu.RLock()
	defer am.metaMu.RUnlock()
	return am.merchantID
}

// Environment returns "LIVE" or "TEST" as reported by the last token exchange.
// Empty until a token has been fetched.
func (am *AuthManager) Environment() string {
	am.metaMu.RLock()
	defer am.metaMu.RUnlock()
	return am.environment
}

func (am *AuthManager) GetTokenExpiry(ctx context.Context) (time.Time, error) {
	_, expiresAt, err := am.cache.Get(ctx, am.cacheKey())
	if err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}

func (am *AuthManager) fetchNewToken(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s%s", strings.TrimSuffix(am.baseURL, "/"), PathAuthToken)

	bodyData, err := json.Marshal(map[string]string{
		"api_secret": am.secretKey,
	})
	if err != nil {
		return "", err
	}

	headers := map[string]string{
		HeaderAuthorization: fmt.Sprintf("Bearer %s", am.publicKey),
		HeaderContentType:   MimeJSON,
	}

	opts := transport.RequestOptions{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    bodyData,
		Timeout: am.timeout,
	}

	resp, err := am.tc.Execute(ctx, opts)
	if err != nil {
		return "", &AuthenticationError{SDKError: SDKError{
			StatusCode: 0,
			Message:    "Network error while fetching transaction token: " + err.Error(),
		}}
	}

	if resp.StatusCode != http.StatusOK {
		return "", &AuthenticationError{SDKError: SDKError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("Failed to obtain transaction token: HTTP %s. %s", resp.Status, string(resp.Body)),
			RawBody:    resp.Body,
		}}
	}

	var data tokenResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return "", &AuthenticationError{SDKError: SDKError{
			StatusCode: resp.StatusCode,
			Message:    "Failed to decode token response JSON: " + err.Error(),
			RawBody:    resp.Body,
		}}
	}

	if data.TransactionToken == "" {
		return "", &AuthenticationError{SDKError: SDKError{
			StatusCode: resp.StatusCode,
			Message:    "Invalid token response: missing transaction_token field.",
			RawBody:    resp.Body,
		}}
	}

	am.metaMu.Lock()
	am.merchantID = data.MerchantID
	am.environment = data.Environment
	am.metaMu.Unlock()

	ttl := TokenTTL
	if data.ExpiresIn > 0 {
		ttl = time.Duration(data.ExpiresIn) * time.Second
	}

	// Apply margin to ensure token is refreshed before it expires
	expiresAt := time.Now().Add(ttl - TokenExpiryMargin)

	err = am.cache.Set(ctx, am.cacheKey(), data.TransactionToken, expiresAt)
	if err != nil {
		// Non-fatal, just continue with the token in memory
	}

	return data.TransactionToken, nil
}

// ─── AuthService (Public Endpoint Wrapper) ─────────────────────────

type AuthService struct {
	authManager *AuthManager
}

func (s *AuthService) GetToken(ctx context.Context) (string, error) {
	return s.authManager.GetToken(ctx)
}

func (s *AuthService) Refresh(ctx context.Context) (string, error) {
	return s.authManager.Refresh(ctx)
}

func (s *AuthService) IsTokenValid(ctx context.Context) bool {
	return s.authManager.IsTokenValid(ctx)
}

func (s *AuthService) ClearCache(ctx context.Context) error {
	return s.authManager.ClearCache(ctx)
}

func (s *AuthService) GetTokenExpiry(ctx context.Context) (time.Time, error) {
	return s.authManager.GetTokenExpiry(ctx)
}

// MerchantID returns the merchant id from the last token exchange.
func (s *AuthService) MerchantID() string {
	return s.authManager.MerchantID()
}

// Environment returns "LIVE" or "TEST" from the last token exchange.
func (s *AuthService) Environment() string {
	return s.authManager.Environment()
}
