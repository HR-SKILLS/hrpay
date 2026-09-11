package hrpay

import (
	"context"
	"encoding/json"

	"github.com/hr-skills/hrpay/internal/crypto"
)

type WebhookEventDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type WebhookEvent struct {
	ID         string          `json:"id"`
	Type       string          `json:"event"` // wire field is "event", e.g. "payment.succeeded"
	MerchantID string          `json:"merchant_id,omitempty"`
	CreatedAt  string          `json:"created_at"`
	Data       json.RawMessage `json:"data"`
}

type WebhooksService struct {
	client *Client
}

// Events lists available webhook event types.
func (s *WebhooksService) Events(ctx context.Context) ([]WebhookEventDefinition, error) {
	respBody, err := s.client.request(ctx, "GET", PathWebhooksEvents, nil, nil)
	if err != nil {
		return nil, err
	}

	var envelope ApiResponse[[]WebhookEventDefinition]
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Success {
		return envelope.Data, nil
	}

	var direct []WebhookEventDefinition
	if err := json.Unmarshal(respBody, &direct); err != nil {
		return nil, err
	}
	return direct, nil
}

// VerifySignature validates an X-Hub-Signature header using HMAC-SHA256.
// The header is formatted as "sha256=<hmac>". A bare hex digest (without the
// "sha256=" prefix) is also accepted for convenience.
func (s *WebhooksService) VerifySignature(payload string, signature string, secret string) bool {
	expected := crypto.ComputeHmacSha256(payload, secret)
	// Prefixed form, matching the documented header value.
	if crypto.SafeCompare("sha256="+expected, signature) {
		return true
	}
	// Bare hex digest fallback.
	return crypto.SafeCompare(expected, signature)
}

// ConstructEvent verifies signature and decodes JSON payload.
func (s *WebhooksService) ConstructEvent(payload string, signature string, secret string) (*WebhookEvent, error) {
	if !s.VerifySignature(payload, signature, secret) {
		return nil, &WebhookSignatureError{
			SDKError: SDKError{
				Message: "Webhook signature mismatch. Ensure you are using the raw body and correct secret.",
			},
		}
	}

	var event WebhookEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, &WebhookSignatureError{
			SDKError: SDKError{
				Message: "Webhook payload is not valid JSON: " + err.Error(),
			},
		}
	}

	return &event, nil
}
