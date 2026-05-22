package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/cole/onyxchat-server/internal/store"
)

const (
	maxUsernameLen  = 32
	maxMessageLen   = 8192 // bumped: ciphertext is larger than plaintext
	defaultPageSize = 50
	maxPageSize     = 100
)

// ─────────────────────────────────────────────────────────────
// POST /api/v1/messages
// Body (plaintext):  {"recipientUsername":"bob","body":"hello"}
// Body (encrypted):  {"recipientUsername":"bob","body":"<ciphertext>","iv":"<nonce>","encrypted":true}
// ─────────────────────────────────────────────────────────────

type SendMessageRequest struct {
	RecipientUsername string `json:"recipientUsername"`
	Body              string `json:"body"`
	IV                string `json:"iv"`
	Encrypted         bool   `json:"encrypted"`
	ClientMessageID   string `json:"clientMessageId"`
}

type MessageCreatedEvent struct {
	MessageID       int64  `json:"messageId"`
	SenderID        int64  `json:"senderId"`
	RecipientID     int64  `json:"recipientId"`
	ClientMessageID string `json:"clientMessageId"`
}

type MessageDeletedEvent struct {
	MessageID   int64 `json:"messageId"`
	SenderID    int64 `json:"senderId"`
	RecipientID int64 `json:"recipientId"`
}

type RedisPublisher struct {
	Client *redis.Client
}

func (p *RedisPublisher) PublishMessageCreated(ctx context.Context, event MessageCreatedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.Client.Publish(ctx, "message.created", payload).Err()
}

func (p *RedisPublisher) PublishMessageDeleted(ctx context.Context, event MessageDeletedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.Client.Publish(ctx, "message.deleted", payload).Err()
}

func (r *SendMessageRequest) Validate() string {
	if r.RecipientUsername == "" || r.Body == "" || r.ClientMessageID == "" {
		return "recipientUsername, body, and clientMessageId required"
	}
	if len(r.RecipientUsername) > maxUsernameLen {
		return "recipientUsername too long"
	}
	if len(r.Body) > maxMessageLen {
		return "message body too long"
	}
	if len(r.ClientMessageID) > 128 {
		return "clientMessageId too long"
	}
	if r.Encrypted {
		if r.IV == "" {
			return "iv required when encrypted is true"
		}
		// The frontend encodes the IV with btoa() (standard base64). AES-GCM
		// requires a 96-bit nonce: 12 bytes encodes to exactly 16 base64 characters
		// (12 is divisible by 3, so no padding). Reject anything else so a
		// malformed IV doesn't reach the DB and silently break decryption.
		ivBytes, err := base64.StdEncoding.DecodeString(r.IV)
		if err != nil || len(ivBytes) != 12 {
			return "iv must be a base64-encoded 96-bit (12-byte) AES-GCM nonce"
		}
	}
	return ""
}

func StartMessageSubscriber(
	ctx context.Context,
	rdb *redis.Client,
	msgStore messageStorer,
	hub *Hub,
	log *zap.Logger,
) error {
	sub := rdb.Subscribe(ctx, "message.created", "message.deleted")
	defer sub.Close()
	ch := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return fmt.Errorf("subscription channel closed")
			}
			switch msg.Channel {
			case "message.created":
				var ev MessageCreatedEvent
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
					log.Error("[RedisSub] invalid message.created payload", zap.Error(err))
					continue
				}
				fullMsg, err := msgStore.GetByID(ev.MessageID)
				if err != nil {
					log.Error("[RedisSub] failed to load message", zap.Int64("message_id", ev.MessageID), zap.Error(err))
					continue
				}
				hub.SendMessageToUser(fullMsg.SenderID, fullMsg)
				hub.SendMessageToUser(fullMsg.RecipientID, fullMsg)

			case "message.deleted":
				var ev MessageDeletedEvent
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
					log.Error("[RedisSub] invalid message.deleted payload", zap.Error(err))
					continue
				}
				hub.SendMessageDeletedToUsers(ev.SenderID, ev.RecipientID, ev.MessageID)
			}
		}
	}
}

type EventPublisher interface {
	PublishMessageCreated(ctx context.Context, event MessageCreatedEvent) error
	PublishMessageDeleted(ctx context.Context, event MessageDeletedEvent) error
}

func SendMessageHandler(
	userStore userStorer,
	msgStore messageStorer,
	hub *Hub,
	publisher EventPublisher,
	log *zap.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		currentUser := CurrentUser(r)
		if currentUser == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if msg := req.Validate(); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}

		recipient, err := userStore.GetByUsername(req.RecipientUsername)
		if err != nil || recipient == nil {
			http.Error(w, "recipient not found", http.StatusBadRequest)
			return
		}

		dbStart := time.Now()
		saved, inserted, err := msgStore.CreateOrGetExisting(
			currentUser.ID,
			recipient.ID,
			req.Body,
			req.IV,
			req.Encrypted,
			req.ClientMessageID,
		)
		ObserveDBQuery("message_create", dbStart)
		if err != nil {
			log.Error("[SendMessage] failed to save message", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to save message")
			return
		}

		if inserted {
			MessagesSent.Inc()
		} else {
			MessageDeduplicated.Inc()
		}

		w.Header().Set("Content-Type", "application/json")
		if inserted {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		if err := json.NewEncoder(w).Encode(saved); err != nil {
			log.Error("[SendMessage] failed to write response", zap.Error(err))
			return
		}

		event := MessageCreatedEvent{
			MessageID:       saved.ID,
			SenderID:        saved.SenderID,
			RecipientID:     saved.RecipientID,
			ClientMessageID: req.ClientMessageID,
		}

		go func(ev MessageCreatedEvent) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			if err := publisher.PublishMessageCreated(ctx, ev); err != nil {
				log.Error("[SendMessage] failed to publish event", zap.Error(err))
				MessagePublishFailures.Inc()
			}
		}(event)
	}
}

// ─────────────────────────────────────────────────────────────
// GET /api/v1/messages?peer=<username>&sinceId=<id>
// ─────────────────────────────────────────────────────────────

type ListMessagesResponse struct {
	Messages []store.Message `json:"messages"`
	HasMore  bool            `json:"hasMore"`
}

func ListMessagesHandler(userStore userStorer, msgStore messageStorer, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentUser := CurrentUser(r)
		if currentUser == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		peer := r.URL.Query().Get("peer")
		if peer == "" {
			http.Error(w, "missing peer", http.StatusBadRequest)
			return
		}

		limit := defaultPageSize
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
				limit = v
			}
		}
		if limit > maxPageSize {
			limit = maxPageSize
		}

		peerUser, err := userStore.GetByUsername(peer)
		if err != nil || peerUser == nil {
			http.Error(w, "peer not found", http.StatusNotFound)
			return
		}

		q := r.URL.Query()
		beforeStr := q.Get("beforeId")
		sinceStr := q.Get("sinceId")

		var msgs []store.Message
		var hasMore bool
		dbStart := time.Now()

		switch {
		case beforeStr != "":
			beforeID, parseErr := strconv.ParseInt(beforeStr, 10, 64)
			if parseErr != nil || beforeID <= 0 {
				http.Error(w, "beforeId must be a positive integer", http.StatusBadRequest)
				return
			}
			msgs, hasMore, err = msgStore.ListConversationBefore(currentUser.ID, peerUser.ID, beforeID, limit)

		default:
			var sinceID int64
			if sinceStr != "" {
				sinceID, err = strconv.ParseInt(sinceStr, 10, 64)
				if err != nil || sinceID < 0 {
					http.Error(w, "sinceId must be a non-negative integer", http.StatusBadRequest)
					return
				}
			}
			msgs, hasMore, err = msgStore.ListConversationSince(currentUser.ID, peerUser.ID, sinceID, limit)
		}

		ObserveDBQuery("message_list", dbStart)
		if err != nil {
			log.Error("[ListMessages] failed to fetch messages", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to fetch messages")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ListMessagesResponse{Messages: msgs, HasMore: hasMore}); err != nil {
			log.Error("[ListMessages] failed to write response", zap.Error(err))
		}
	}
}

// ─────────────────────────────────────────────────────────────
// POST /api/v1/messages/read
// Body: {"peerUsername":"bob"}
// Marks all unread messages from bob → me as read.
// Fires a message_read WS event back to bob.
// ─────────────────────────────────────────────────────────────

type MarkReadRequest struct {
	PeerUsername string `json:"peerUsername"`
}

func MarkReadHandler(userStore userStorer, msgStore messageStorer, hub *Hub, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentUser := CurrentUser(r)
		if currentUser == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req MarkReadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PeerUsername == "" {
			http.Error(w, "peerUsername required", http.StatusBadRequest)
			return
		}

		peer, err := userStore.GetByUsername(req.PeerUsername)
		if err != nil || peer == nil {
			http.Error(w, "peer not found", http.StatusNotFound)
			return
		}

		dbStart := time.Now()
		ids, err := msgStore.MarkRead(currentUser.ID, peer.ID)
		ObserveDBQuery("message_mark_read", dbStart)
		if err != nil {
			log.Error("[MarkRead] db error", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to mark read")
			return
		}

		if len(ids) > 0 {
			hub.SendReadReceiptToUser(peer.ID, currentUser.ID, ids)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"markedCount": len(ids)})
	}
}

// ─────────────────────────────────────────────────────────────
// DELETE /api/v1/messages/{id}
// Only the original sender may delete. Publishes to Redis so all server
// instances broadcast the message_deleted WS event to both parties.
// ─────────────────────────────────────────────────────────────

func DeleteMessageHandler(
	msgStore messageStorer,
	publisher EventPublisher,
	log *zap.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentUser := CurrentUser(r)
		if currentUser == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		messageID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
		if err != nil || messageID <= 0 {
			http.Error(w, "invalid message id", http.StatusBadRequest)
			return
		}

		dbStart := time.Now()
		deleted, err := msgStore.SoftDelete(messageID, currentUser.ID)
		ObserveDBQuery("message_delete", dbStart)
		if errors.Is(err, store.ErrMessageNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Error("[DeleteMessage] db error", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to delete message")
			return
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := publisher.PublishMessageDeleted(ctx, MessageDeletedEvent{
				MessageID:   deleted.ID,
				SenderID:    deleted.SenderID,
				RecipientID: deleted.RecipientID,
			}); err != nil {
				log.Error("[DeleteMessage] failed to publish event", zap.Error(err))
			}
		}()

		w.WriteHeader(http.StatusNoContent)
	}
}
