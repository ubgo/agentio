package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/opencode"
)

// messageOrder is the stable order of a session's messages: creation time,
// then id (the message_session_time_created_id_idx index opencode keeps).
const messageOrder = " ORDER BY time_created, id"

// Messages reads every message of a session with its parts (part-id
// order), oldest first. It reads the whole session: call it for one
// session, not in a listing loop. cost: Disk · Read.
func (d *DB) Messages(ctx context.Context, sessionID string) ([]opencode.MessageRecord, error) {
	tx, err := d.readTx(ctx)
	if err != nil {
		return nil, err
	}
	// Read-only transaction: rollback only ends the snapshot.
	defer func() { _ = tx.Rollback() }()
	return readMessages(ctx, tx, sessionID, 0)
}

// readTx opens a read-only transaction, so the message count, the
// messages and their parts come from one WAL snapshot even while opencode
// writes. Disk · Read.
func (d *DB) readTx(ctx context.Context) (*sql.Tx, error) {
	c, err := d.db()
	if err != nil {
		return nil, err
	}
	for _, t := range []string{tableMessage, tablePart} {
		if _, err := d.columns(ctx, t); err != nil {
			return nil, err
		}
	}
	tx, err := c.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("opencode/store: begin read: %w", err)
	}
	return tx, nil
}

// readMessages reads the messages of a session from position from on
// (messageOrder) with their parts. Disk · Read.
func readMessages(ctx context.Context, tx *sql.Tx, sessionID string, from agentio.Offset) ([]opencode.MessageRecord, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, data FROM "+tableMessage+" WHERE session_id = ?"+messageOrder+" LIMIT -1 OFFSET ?", sessionID, int64(from))
	if err != nil {
		return nil, fmt.Errorf("opencode/store: messages: %w", err)
	}
	var out []opencode.MessageRecord
	index := map[string]int{}
	err = func() error {
		// Read-only rows: a close error loses nothing.
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id, data string
			if err := rows.Scan(&id, &data); err != nil {
				return err
			}
			m, err := opencode.DecodeMessage([]byte(data))
			if err != nil {
				return fmt.Errorf("message %s: %w", id, err)
			}
			index[id] = len(out)
			out = append(out, opencode.MessageRecord{ID: id, SessionID: sessionID, Info: m, Raw: agentio.Raw(data)})
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, fmt.Errorf("opencode/store: messages: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	if err := readParts(ctx, tx, sessionID, from, out, index); err != nil {
		return nil, err
	}
	return out, nil
}

// readParts attaches the parts of the messages at position from on to
// out, in part-id order. Disk · Read.
func readParts(ctx context.Context, tx *sql.Tx, sessionID string, from agentio.Offset, out []opencode.MessageRecord, index map[string]int) error {
	rows, err := tx.QueryContext(ctx, "SELECT p.message_id, p.id, p.data FROM "+tablePart+" p JOIN (SELECT id FROM "+tableMessage+
		" WHERE session_id = ?"+messageOrder+" LIMIT -1 OFFSET ?) m ON m.id = p.message_id ORDER BY p.message_id, p.id", sessionID, int64(from))
	if err != nil {
		return fmt.Errorf("opencode/store: parts: %w", err)
	}
	// Read-only rows: a close error loses nothing.
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var msgID, id, data string
		if err := rows.Scan(&msgID, &id, &data); err != nil {
			return fmt.Errorf("opencode/store: parts: %w", err)
		}
		p, err := opencode.DecodePart([]byte(data))
		if err != nil {
			return fmt.Errorf("opencode/store: part %s: %w", id, err)
		}
		// The data column leaves the ids to their own columns.
		p.ID, p.MessageID, p.SessionID = id, msgID, sessionID
		if i, ok := index[msgID]; ok {
			out[i].Parts = append(out[i].Parts, p)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("opencode/store: parts: %w", err)
	}
	return nil
}

// Turns reads the user and assistant messages of a session from cursor
// from (0 = the start), calling fn per message, and returns the new
// cursor. Turns follow opencode.MessageRecord.Turn: text parts only
// (synthetic text skipped), Turn.ID the message id, tokens from the
// message's tokens with reasoning separate, Model "providerID/modelID",
// Interim on tool-call steps and compaction summaries.
//
// The cursor is the number of the session's messages already passed, in
// the stable order time_created then id. Reading stops before the newest
// message while opencode is still writing it (an assistant message without
// time.completed or an error, a user message without parts), so its text
// and usage are reported once. A cursor past the end (messages deleted by
// a revert) emits nothing and returns the current count; a caller that
// must not miss a message dedups on Turn.ID and re-reads from 0. The
// count, messages and parts come from one read transaction.
// cost: Disk · Read (only the messages past from).
func (d *DB) Turns(ctx context.Context, sessionID string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	from = max(from, 0)
	s, err := d.Session(ctx, sessionID)
	if err != nil {
		return from, err
	}
	tx, err := d.readTx(ctx)
	if err != nil {
		return from, err
	}
	// Read-only transaction: rollback only ends the snapshot.
	defer func() { _ = tx.Rollback() }()
	var n int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableMessage+" WHERE session_id = ?", sessionID).Scan(&n); err != nil {
		return from, fmt.Errorf("opencode/store: count messages: %w", err)
	}
	if int64(from) > n {
		return agentio.Offset(n), nil
	}
	recs, err := readMessages(ctx, tx, sessionID, from)
	if err != nil {
		return from, err
	}
	return opencode.EmitTurns(recs, from, opencode.TurnContext{Dir: s.Directory, Version: s.Version}, fn)
}
