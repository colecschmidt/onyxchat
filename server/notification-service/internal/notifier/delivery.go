package notifier

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	maxAttempts    = 3
	initialBackoff = time.Second
)

// deliverWithRetry attempts notification delivery up to maxAttempts times,
// doubling the backoff on each failure. Respects ctx cancellation.
func deliverWithRetry(ctx context.Context, ev MessageCreatedEvent, pushToken string, log *zap.Logger) error {
	backoff := initialBackoff

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := deliver(ctx, ev, pushToken)
		if err == nil {
			return nil
		}

		log.Warn("delivery attempt failed",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", maxAttempts),
			zap.Int64("message_id", ev.MessageID),
			zap.Error(err),
		)

		if attempt == maxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return fmt.Errorf("delivery failed after %d attempts", maxAttempts)
}

// deliver is the stub notification backend. Replace the body with FCM/APNs/
// web-push calls when real push is wired up. The function signature and retry
// contract stay the same regardless of backend.
func deliver(ctx context.Context, ev MessageCreatedEvent, pushToken string) error {
	// Simulate work so the duration histogram has something to observe.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Millisecond):
	}

	// In production: pass pushToken to FCM, APNs, or web-push.
	// Return a non-nil error to trigger the retry loop.
	_, _ = ev, pushToken
	return nil
}
