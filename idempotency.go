package hrpay

import "context"

type idempotencyKeyCtxKey struct{}

// WithIdempotencyKey attaches a caller-supplied Idempotency-Key to ctx for the
// next mutating call (payin, payout, refund, card issuance/topup/withdraw,
// card-wallet fund/withdraw). Use it to make a call safely retryable: if the
// first attempt returned a 2xx, the exact same response is replayed verbatim
// for 24h regardless of the request body sent on a retry — never reuse a key
// across two logically different operations. If no key is attached, the SDK
// auto-generates a fresh UUID v4 per request, as before.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, idempotencyKeyCtxKey{}, key)
}

func idempotencyKeyFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(idempotencyKeyCtxKey{}).(string)
	return v, ok && v != ""
}
