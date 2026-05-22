package store

import (
	"database/sql"
	"errors"
)

var ErrMessageNotFound = errors.New("message not found")

type MessageStore struct {
	db *sql.DB
}

func NewMessageStore(db *sql.DB) *MessageStore {
	return &MessageStore{db: db}
}

// scanMsg reads a message from any Scan-compatible source.
// Column order must be: id, sender_id, recipient_id, body, iv, encrypted,
// client_message_id, created_at, read_at, deleted_at.
func scanMsg(scan func(dest ...any) error) (Message, error) {
	var m Message
	var iv, clientMsgID sql.NullString
	var readAt, deletedAt sql.NullTime
	err := scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Body, &iv,
		&m.Encrypted, &clientMsgID, &m.CreatedAt, &readAt, &deletedAt)
	if err != nil {
		return Message{}, err
	}
	m.IV = iv.String
	m.ClientMessageID = clientMsgID.String
	if readAt.Valid {
		m.ReadAt = &readAt.Time
	}
	if deletedAt.Valid {
		m.DeletedAt = &deletedAt.Time
	}
	return m, nil
}

// selectCols is the SELECT column list for display queries.
// Deleted messages have their body and IV cleared so clients can show a
// "message deleted" placeholder without exposing ciphertext.
const selectCols = `
    id, sender_id, recipient_id,
    CASE WHEN deleted_at IS NOT NULL THEN '' ELSE body END,
    CASE WHEN deleted_at IS NOT NULL THEN '' ELSE iv   END,
    encrypted, client_message_id, created_at, read_at, deleted_at`

// rawCols is used in RETURNING clauses where CASE expressions aren't needed
// (newly inserted/updated rows always have the real values we want).
const rawCols = `id, sender_id, recipient_id, body, iv, encrypted, client_message_id, created_at, read_at, deleted_at`

// CreateOrGetExisting inserts a new message, or returns the existing one when
// the (sender_id, client_message_id) pair already exists (idempotent send).
// Returns (message, inserted, error).
func (s *MessageStore) CreateOrGetExisting(
	senderID, recipientID int64,
	body, iv string,
	encrypted bool,
	clientMessageID string,
) (*Message, bool, error) {
	const insertQ = `
		INSERT INTO messages (sender_id, recipient_id, body, iv, encrypted, client_message_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (sender_id, client_message_id) DO NOTHING
		RETURNING ` + rawCols

	m, err := scanMsg(s.db.QueryRow(insertQ, senderID, recipientID, body, iv, encrypted, clientMessageID).Scan)
	if err == nil {
		return &m, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	const fetchQ = `SELECT ` + rawCols + ` FROM messages WHERE sender_id = $1 AND client_message_id = $2`
	m, err = scanMsg(s.db.QueryRow(fetchQ, senderID, clientMessageID).Scan)
	if err != nil {
		return nil, false, err
	}
	return &m, false, nil
}

// GetByID returns a single message by primary key.
func (s *MessageStore) GetByID(id int64) (*Message, error) {
	const q = `SELECT ` + selectCols + ` FROM messages WHERE id = $1`
	m, err := scanMsg(s.db.QueryRow(q, id).Scan)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListConversationSince returns up to limit messages between two users with
// id > sinceID, ordered oldest-first. Fetches limit+1 rows to detect hasMore.
func (s *MessageStore) ListConversationSince(userID, peerID, sinceID int64, limit int) ([]Message, bool, error) {
	const q = `
		SELECT ` + selectCols + `
		FROM messages
		WHERE ((sender_id = $1 AND recipient_id = $2) OR (sender_id = $2 AND recipient_id = $1))
		  AND id > $3
		ORDER BY id ASC
		LIMIT $4`

	return s.queryMessages(q, userID, peerID, sinceID, limit+1, limit, false)
}

// ListConversationBefore returns up to limit messages between two users with
// id < beforeID, ordered oldest-first. Fetches limit+1 rows to detect hasMore.
// Used for loading older history when scrolling up.
func (s *MessageStore) ListConversationBefore(userID, peerID, beforeID int64, limit int) ([]Message, bool, error) {
	const q = `
		SELECT ` + selectCols + `
		FROM messages
		WHERE ((sender_id = $1 AND recipient_id = $2) OR (sender_id = $2 AND recipient_id = $1))
		  AND id < $3
		ORDER BY id DESC
		LIMIT $4`

	msgs, hasMore, err := s.queryMessages(q, userID, peerID, beforeID, limit+1, limit, false)
	if err != nil {
		return nil, false, err
	}
	// Reverse so results are oldest-first, consistent with ListConversationSince.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, hasMore, nil
}

// GetUnreadForUser returns messages delivered to userID with id > sinceID,
// excluding deleted messages. Used to replay missed messages on WS reconnect.
func (s *MessageStore) GetUnreadForUser(userID, sinceID int64) ([]Message, error) {
	const q = `
		SELECT ` + selectCols + `
		FROM messages
		WHERE recipient_id = $1 AND id > $2 AND deleted_at IS NULL
		ORDER BY id ASC`

	rows, err := s.db.Query(q, userID, sinceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		m, err := scanMsg(rows.Scan)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// MarkRead sets read_at = NOW() on all unread messages sent from senderID to
// recipientID. Returns the IDs of the messages that were updated.
func (s *MessageStore) MarkRead(recipientID, senderID int64) ([]int64, error) {
	const q = `
		UPDATE messages
		SET read_at = NOW()
		WHERE recipient_id = $1
		  AND sender_id    = $2
		  AND read_at      IS NULL
		  AND deleted_at   IS NULL
		RETURNING id`

	rows, err := s.db.Query(q, recipientID, senderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SoftDelete marks a message as deleted. Only the original sender may delete.
// Returns ErrMessageNotFound if the message doesn't exist, is already deleted,
// or belongs to a different sender.
func (s *MessageStore) SoftDelete(messageID, senderID int64) (*Message, error) {
	const q = `
		UPDATE messages
		SET deleted_at = NOW()
		WHERE id = $1 AND sender_id = $2 AND deleted_at IS NULL
		RETURNING ` + rawCols

	m, err := scanMsg(s.db.QueryRow(q, messageID, senderID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// queryMessages is a shared helper for list queries that accept (userID, peerID,
// cursor, fetchLimit) and return at most retainLimit messages + hasMore.
func (s *MessageStore) queryMessages(q string, userID, peerID, cursor int64, fetchLimit, retainLimit int, _ bool) ([]Message, bool, error) {
	rows, err := s.db.Query(q, userID, peerID, cursor, fetchLimit)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	msgs := make([]Message, 0, retainLimit)
	for rows.Next() {
		m, err := scanMsg(rows.Scan)
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	if len(msgs) > retainLimit {
		return msgs[:retainLimit], true, nil
	}
	return msgs, false, nil
}
