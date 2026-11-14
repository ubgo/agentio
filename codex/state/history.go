package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ubgo/agentio"
)

// Tables of thread_history_1.sqlite (upstream state/thread_history_migrations).
const (
	tableTurns = "thread_turns"
	tableItems = "thread_items"
)

// TurnStatus is thread_turns.status (thread-store thread_history.rs
// turn_status).
type TurnStatus string

// TurnStatus values.
const (
	TurnCompleted   TurnStatus = "completed"
	TurnFailed      TurnStatus = "failed"
	TurnInterrupted TurnStatus = "interrupted"
	TurnInProgress  TurnStatus = "inProgress"
)

// IsKnown reports whether s is a known turn status. Pure.
func (s TurnStatus) IsKnown() bool {
	switch s {
	case TurnCompleted, TurnFailed, TurnInterrupted, TurnInProgress:
		return true
	}
	return false
}

// Turn is one row of thread_turns: a user prompt and the agent's work on
// it, with the byte range it occupies in the rollout file.
type Turn struct {
	ThreadID          string
	TurnID            string
	RootTurnID        *string // 0008+
	Status            TurnStatus
	Error             agentio.Raw // error_json, shape vendor-defined
	Started           time.Time   // started_at (seconds)
	Completed         time.Time   // completed_at (seconds)
	Duration          time.Duration
	FirstUserItemID   *string
	FinalAgentItemID  *string
	RolloutOrdinal    int64
	RolloutByteOffset *int64 // 0003+: where the turn starts in the rollout
	RolloutEndOrdinal *int64
	RolloutEndOffset  *int64
}

// turnColumns is the SELECT layout scanned by Turns.
func turnColumns() []string {
	return []string{
		"thread_id", "turn_id", "root_turn_id", "status", "error_json", "started_at", "completed_at",
		"duration_ms", "first_user_item_id", "final_agent_item_id", "rollout_ordinal",
		"rollout_byte_offset", "rollout_end_ordinal", "rollout_end_byte_offset",
	}
}

// Turns lists a thread's turns in rollout order. Only paginated threads
// (Thread.HistoryMode) have rows; a legacy thread returns none. Disk · Read
// (thread_history_1.sqlite).
func (d *DB) Turns(ctx context.Context, threadID string) ([]Turn, error) {
	c, err := d.ready(ctx, FileThreadHistory)
	if err != nil {
		return nil, err
	}
	have, err := d.columns(ctx, FileThreadHistory, tableTurns)
	if err != nil {
		return nil, err
	}
	q := "SELECT " + selectList(have, "", turnColumns()) + " FROM " + tableTurns +
		" WHERE thread_id = ? ORDER BY rollout_ordinal LIMIT ?"
	rows, err := c.QueryContext(ctx, q, threadID, maxLimit)
	if err != nil {
		return nil, fmt.Errorf("codex/state: turns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Turn
	for rows.Next() {
		var t Turn
		var root, status, errJSON, firstUser, finalAgent sql.NullString
		var started, completed, durMs, ord, off, endOrd, endOff sql.NullInt64
		if err := rows.Scan(&t.ThreadID, &t.TurnID, &root, &status, &errJSON, &started, &completed,
			&durMs, &firstUser, &finalAgent, &ord, &off, &endOrd, &endOff); err != nil {
			return nil, fmt.Errorf("codex/state: turns: %w", err)
		}
		t.RootTurnID = strPtr(root)
		t.Status = TurnStatus(status.String)
		if errJSON.String != "" {
			t.Error = agentio.Raw(errJSON.String)
		}
		t.Started, t.Completed = fromSeconds(started), fromSeconds(completed)
		t.Duration = time.Duration(durMs.Int64) * time.Millisecond
		t.FirstUserItemID, t.FinalAgentItemID = strPtr(firstUser), strPtr(finalAgent)
		t.RolloutOrdinal = ord.Int64
		t.RolloutByteOffset, t.RolloutEndOrdinal, t.RolloutEndOffset = int64Ptr(off), int64Ptr(endOrd), int64Ptr(endOff)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ItemType is thread_items.item_type (camelCase upstream ThreadItem tag).
type ItemType string

// ItemType values seen on the sample home (CODEX_API §9).
const (
	ItemUserMessage       ItemType = "userMessage"
	ItemAgentMessage      ItemType = "agentMessage"
	ItemReasoning         ItemType = "reasoning"
	ItemCommandExecution  ItemType = "commandExecution"
	ItemFileChange        ItemType = "fileChange"
	ItemMCPToolCall       ItemType = "mcpToolCall"
	ItemWebSearch         ItemType = "webSearch"
	ItemContextCompaction ItemType = "contextCompaction"
)

// IsKnown reports whether t is a known item type. Pure.
func (t ItemType) IsKnown() bool {
	switch t {
	case ItemUserMessage, ItemAgentMessage, ItemReasoning, ItemCommandExecution, ItemFileChange,
		ItemMCPToolCall, ItemWebSearch, ItemContextCompaction:
		return true
	}
	return false
}

// Item is one row of thread_items: a typed header plus the vendor JSON,
// which stays raw (its schema is upstream's ThreadItem and moves fast).
type Item struct {
	ThreadID       string
	TurnID         string
	ItemID         string
	Type           ItemType
	RolloutOrdinal int64
	Created        time.Time
	Started        *time.Time // 0007+
	Completed      *time.Time // 0007+
	JSON           agentio.Raw
}

// itemTypeKey is the JSON key holding the item type, used when the
// item_type column predates migration 0002.
const itemTypeKey = "type"

// Items pages through a thread's items in rollout order: those with
// rollout_ordinal > after, at most limit (0 = 200, capped at 10000).
// Disk · Read (thread_history_1.sqlite).
func (d *DB) Items(ctx context.Context, threadID string, after int64, limit int) ([]Item, error) {
	c, err := d.ready(ctx, FileThreadHistory)
	if err != nil {
		return nil, err
	}
	have, err := d.columns(ctx, FileThreadHistory, tableItems)
	if err != nil {
		return nil, err
	}
	cols := []string{"thread_id", "turn_id", "item_id", "item_type", "rollout_ordinal", "created_at_ms", "started_at_ms", "completed_at_ms", "item_json"}
	q := "SELECT " + selectList(have, "", cols) + " FROM " + tableItems +
		" WHERE thread_id = ? AND rollout_ordinal > ? ORDER BY rollout_ordinal LIMIT ?"
	rows, err := c.QueryContext(ctx, q, threadID, after, clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("codex/state: items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Item
	for rows.Next() {
		var it Item
		var typ, body sql.NullString
		var created, started, completed sql.NullInt64
		if err := rows.Scan(&it.ThreadID, &it.TurnID, &it.ItemID, &typ, &it.RolloutOrdinal, &created, &started, &completed, &body); err != nil {
			return nil, fmt.Errorf("codex/state: items: %w", err)
		}
		it.Type = ItemType(typ.String)
		if it.Type == "" && body.String != "" {
			var head map[string]json.RawMessage
			if json.Unmarshal([]byte(body.String), &head) == nil {
				var s string
				if json.Unmarshal(head[itemTypeKey], &s) == nil {
					it.Type = ItemType(s)
				}
			}
		}
		it.Created = fromMillis(created)
		it.Started, it.Completed = timePtr(fromMillis(started)), timePtr(fromMillis(completed))
		it.JSON = agentio.Raw(body.String)
		out = append(out, it)
	}
	return out, rows.Err()
}
