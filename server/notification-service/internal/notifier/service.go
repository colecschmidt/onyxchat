package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
)

// MessageCreatedEvent mirrors the struct published by onyxchat-server.
// Services own their own type definitions — the contract is the JSON shape.
type MessageCreatedEvent struct {
	MessageID       int64             `json:"messageId"`
	SenderID        int64             `json:"senderId"`
	RecipientID     int64             `json:"recipientId"`
	ClientMessageID string            `json:"clientMessageId"`
	TraceContext    map[string]string `json:"traceContext,omitempty"`
}

type Service struct {
	rdb         *redis.Client
	log         *zap.Logger
	pushTokens  *PushTokenClient
}

func New(rdb *redis.Client, pushTokens *PushTokenClient, log *zap.Logger) *Service {
	return &Service{rdb: rdb, pushTokens: pushTokens, log: log}
}

// Run subscribes to message.created and processes events until ctx is cancelled.
// Returns a non-nil error only when the subscription channel closes unexpectedly,
// so the caller's restart loop can reconnect.
func (s *Service) Run(ctx context.Context) error {
	sub := s.rdb.Subscribe(ctx, "message.created")
	defer sub.Close()

	s.log.Info("notification service listening", zap.String("channel", "message.created"))

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-sub.Channel():
			if !ok {
				return fmt.Errorf("subscription channel closed")
			}
			var ev MessageCreatedEvent
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				s.log.Error("invalid message.created payload", zap.Error(err))
				continue
			}
			s.log.Info("received message.created event",
				zap.Int64("message_id", ev.MessageID),
				zap.Int64("recipient_id", ev.RecipientID),
			)
			// Handle each event in its own goroutine so a slow delivery doesn't
			// stall the subscription loop.
			go s.handle(ev)
		}
	}
}

func (s *Service) handle(ev MessageCreatedEvent) {
	// Re-attach to the originating trace so this span shows up as a child of
	// the HTTP request that triggered the send, linking both services in Tempo.
	carrier := propagation.MapCarrier(ev.TraceContext)
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)

	tracer := otel.Tracer("notification-service")
	ctx, span := tracer.Start(ctx, "notification.handle")
	defer span.End()

	span.SetAttributes(
		attribute.Int64("message.id", ev.MessageID),
		attribute.Int64("recipient.id", ev.RecipientID),
	)

	start := time.Now()
	defer func() { NotificationDuration.Observe(time.Since(start).Seconds()) }()

	online, err := s.isOnline(ctx, ev.RecipientID)
	if err != nil {
		s.log.Warn("presence check failed, assuming offline", zap.Int64("recipient", ev.RecipientID), zap.Error(err))
	}

	if online {
		NotificationsSkipped.Inc()
		span.SetAttributes(attribute.String("skip_reason", "recipient_online"))
		s.log.Debug("skipping notification: recipient is online", zap.Int64("recipient", ev.RecipientID))
		return
	}

	// Fetch the recipient's push token via authenticated call to the main server.
	pushToken, err := s.pushTokens.GetPushToken(ctx, ev.RecipientID)
	if err != nil {
		s.log.Warn("failed to fetch push token", zap.Int64("recipient", ev.RecipientID), zap.Error(err))
	}
	if pushToken == "" {
		s.log.Debug("recipient has no push token, skipping", zap.Int64("recipient", ev.RecipientID))
		NotificationsSkipped.Inc()
		return
	}

	if err := deliverWithRetry(ctx, ev, pushToken, s.log); err != nil {
		NotificationErrors.Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error("notification delivery failed", zap.Int64("message_id", ev.MessageID), zap.Error(err))
		return
	}

	NotificationsSent.Inc()
	s.log.Info("notification delivered",
		zap.Int64("message_id", ev.MessageID),
		zap.Int64("recipient_id", ev.RecipientID),
	)
}

// isOnline checks the distributed presence store. The key format must match
// onyxchat-server's presence.go: "presence:conns:<userID>".
func (s *Service) isOnline(ctx context.Context, userID int64) (bool, error) {
	key := "presence:conns:" + strconv.FormatInt(userID, 10)
	val, err := s.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val > 0, nil
}
