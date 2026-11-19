package opencode

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Millis is Unix milliseconds, the epoch form opencode writes in every
// time object and SQLite time column.
type Millis int64

// Time converts to UTC; 0 (unset) stays the zero time. cost: Pure.
func (m Millis) Time() time.Time {
	if m <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(m)).UTC()
}

// MessageTime is a message's time object. Completed is set on an assistant
// message once its last step finished; 0 while it streams, and on a
// message opencode abandoned without an error.
type MessageTime struct {
	Created   Millis `json:"created"`
	Completed Millis `json:"completed,omitempty"`
}

// ModelRef is the model a user message asked for (1.1.14 and later):
// {"providerID":"openai","modelID":"gpt-5-nano"}; Variant is the
// provider-specific reasoning effort when one was chosen ("high").
type ModelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
	Variant    string `json:"variant,omitempty"`
}

// MessagePath is where an assistant message ran.
type MessagePath struct {
	Cwd  string `json:"cwd"`
	Root string `json:"root"`
}

// CacheTokens are the prompt-cache counts of a message.
type CacheTokens struct {
	Read  int64 `json:"read"`
	Write int64 `json:"write"`
}

// Tokens is an assistant message's token use as opencode records it.
// Total is written by newer releases only (seen from 1.3.13); 0 when
// absent.
type Tokens struct {
	Input     int64       `json:"input"`
	Output    int64       `json:"output"`
	Reasoning int64       `json:"reasoning"`
	Cache     CacheTokens `json:"cache"`
	Total     int64       `json:"total,omitempty"`
}

// Neutral maps the counts onto agentio.TokenCounts: cache read is
// CachedInput, cache write is CacheWrite, reasoning stays separate. The
// numbers are copied as opencode records them; whether Output already
// includes Reasoning depends on the provider. cost: Pure.
func (t Tokens) Neutral() agentio.TokenCounts {
	return agentio.TokenCounts{
		Input: t.Input, CachedInput: t.Cache.Read, CacheWrite: t.Cache.Write,
		Output: t.Output, Reasoning: t.Reasoning,
	}
}

// MessageError is the error an assistant message ended with
// ({"name":"MessageAbortedError","data":{"message":"Aborted"}}). Data
// stays raw: its keys depend on Name (APIError carries a status code and
// headers).
type MessageError struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Message is one message's info object: a legacy storage/message file, or
// the data column of an opencode.db message row (which leaves id and
// sessionID to their own columns). User messages carry Agent and Model;
// assistant messages carry the provider, model, mode, path, cost, tokens,
// Finish and Error. Keys not modelled here are kept in Unknown.
type Message struct {
	ID        string      `json:"id,omitempty"`
	SessionID string      `json:"sessionID,omitempty"`
	Role      string      `json:"role"`
	Time      MessageTime `json:"time"`
	ParentID  string      `json:"parentID,omitempty"`
	Agent     string      `json:"agent,omitempty"`
	// Model is the model a user message asked for.
	Model *ModelRef `json:"model,omitempty"`
	// Summary is an object on a user message ({"diffs":[...]}) and true on
	// the assistant message that summarises a compaction; raw because its
	// type depends on the role.
	Summary    json.RawMessage `json:"summary,omitempty"`
	ModelID    string          `json:"modelID,omitempty"`
	ProviderID string          `json:"providerID,omitempty"`
	Mode       string          `json:"mode,omitempty"`
	// System is the system prompt an assistant message ran with (0.x).
	System []string      `json:"system,omitempty"`
	Path   *MessagePath  `json:"path,omitempty"`
	Cost   float64       `json:"cost,omitempty"`
	Tokens *Tokens       `json:"tokens,omitempty"`
	Finish string        `json:"finish,omitempty"`
	Error  *MessageError `json:"error,omitempty"`
	// Unknown are the keys not modelled above, raw.
	Unknown agentio.Unknown `json:"-"`
}

// IsCompactionSummary reports the assistant message that summarises a
// compacted conversation (mode "compaction", summary true). cost: Pure.
func (m Message) IsCompactionSummary() bool {
	return m.Role == roleAssistant && (m.Mode == modeCompaction || strings.TrimSpace(string(m.Summary)) == "true")
}

// ModelName is "providerID/modelID": an assistant message's own model,
// else the model a user message asked for; "" when neither is recorded.
// cost: Pure.
func (m Message) ModelName() string {
	switch {
	case m.ModelID != "":
		return joinModel(m.ProviderID, m.ModelID)
	case m.Model != nil && m.Model.ModelID != "":
		return joinModel(m.Model.ProviderID, m.Model.ModelID)
	}
	return ""
}

// joinModel is "provider/model", or the model alone without a provider.
// cost: Pure.
func joinModel(provider, model string) string {
	if provider == "" {
		return model
	}
	return provider + "/" + model
}

// Part is one part of a message. Only text parts feed turns; every other
// type (reasoning, tool, step-start, step-finish, compaction, file, ...)
// is kept with its keys in Unknown.
type Part struct {
	ID        string `json:"id,omitempty"`
	SessionID string `json:"sessionID,omitempty"`
	MessageID string `json:"messageID,omitempty"`
	Type      string `json:"type"`
	// Text is the text of a text or reasoning part.
	Text string `json:"text,omitempty"`
	// Synthetic marks text opencode inserted itself (a plan-mode reminder,
	// a compaction continue prompt), not typed by the user or the model.
	Synthetic bool `json:"synthetic,omitempty"`
	// Unknown are the keys not modelled above, raw.
	Unknown agentio.Unknown `json:"-"`
}

// DecodeMessage decodes a message info object, keeping unknown keys.
// cost: Pure.
func DecodeMessage(data []byte) (Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return Message{}, fmt.Errorf("opencode: message: %w", err)
	}
	u, err := agentio.ExtractUnknownFor(data, reflect.TypeFor[Message]())
	if err != nil {
		return Message{}, fmt.Errorf("opencode: message: %w", err)
	}
	m.Unknown = u
	return m, nil
}

// DecodePart decodes a part object, keeping unknown keys. cost: Pure.
func DecodePart(data []byte) (Part, error) {
	var p Part
	if err := json.Unmarshal(data, &p); err != nil {
		return Part{}, fmt.Errorf("opencode: part: %w", err)
	}
	u, err := agentio.ExtractUnknownFor(data, reflect.TypeFor[Part]())
	if err != nil {
		return Part{}, fmt.Errorf("opencode: part: %w", err)
	}
	p.Unknown = u
	return p, nil
}

// MessageRecord is one message with its parts in part-id order, as either
// store holds it. ID and SessionID are the record's own (the legacy file
// name or the database columns), so they are set even where the info
// object omits them.
type MessageRecord struct {
	ID, SessionID string
	Info          Message
	Parts         []Part
	// Raw is the message info JSON as stored.
	Raw agentio.Raw
}

// TurnContext is what a turn takes from its session.
type TurnContext struct {
	// Dir is the session directory; when empty an assistant message's
	// path.cwd is used.
	Dir string
	// Version is the opencode version that created the session.
	Version string
}

// Complete reports whether opencode has finished writing r, so a cursor
// may pass it. A message followed by another (last false) is complete: a
// new message means opencode moved on, even past an assistant message it
// abandoned without completing it. The last message is complete when it
// is an assistant message with time.completed or an error, or a user
// message with at least one part. cost: Pure.
func (r MessageRecord) Complete(last bool) bool {
	if !last {
		return true
	}
	if r.Info.Role == roleAssistant {
		return r.Info.Time.Completed > 0 || r.Info.Error != nil
	}
	return len(r.Parts) > 0
}

// Text joins the message's text parts with newlines, skipping synthetic
// text opencode inserted itself. cost: Pure.
func (r MessageRecord) Text() string {
	var texts []string
	for _, p := range r.Parts {
		if p.Type == partText && !p.Synthetic && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// Turn converts r to a neutral turn: user and assistant messages only,
// Turn.ID the message id, Text the non-synthetic text parts, Model
// "providerID/modelID", StopReason the finish reason, Tokens from an
// assistant message's tokens (reasoning separate). Interim is set on an
// assistant message whose step ended in tool calls (finish "tool-calls":
// the agent kept working after it) and on a compaction summary, so a
// caller that stores answers keeps only the final message of a prompt.
// cost: Pure.
func (r MessageRecord) Turn(s TurnContext) (agentio.Turn, bool) {
	m := r.Info
	t := agentio.Turn{
		ID: r.ID, ConversationID: r.SessionID, Dir: s.Dir, AgentVersion: s.Version,
		Time: m.Time.Created.Time(), Model: m.ModelName(), StopReason: m.Finish,
		Text: r.Text(), Raw: r.Raw,
	}
	switch m.Role {
	case roleUser:
		t.Role = agentio.RoleUser
	case roleAssistant:
		t.Role = agentio.RoleAssistant
		t.Interim = m.Finish == finishToolCalls || m.IsCompactionSummary()
		if m.Tokens != nil {
			n := m.Tokens.Neutral()
			t.Tokens = &n
		}
		if t.Dir == "" && m.Path != nil {
			t.Dir = m.Path.Cwd
		}
	default:
		return agentio.Turn{}, false
	}
	return t, true
}

// EmitTurns is the cursor rule both stores share. The cursor is the number
// of messages of the session already passed, in the store's stable order
// (oldest first). recs are the messages from position from onward, the
// last of them being the session's newest message. Each complete record is
// passed to fn as a turn; reading stops before the first incomplete one,
// so its usage is reported once it is written. Returns from plus the
// records passed. cost: Pure (fn may do more).
func EmitTurns(recs []MessageRecord, from agentio.Offset, s TurnContext, fn func(agentio.Turn) error) (agentio.Offset, error) {
	off := from
	for i, r := range recs {
		if !r.Complete(i == len(recs)-1) {
			break
		}
		off++
		t, ok := r.Turn(s)
		if !ok {
			continue
		}
		if err := fn(t); err != nil {
			return off, err
		}
	}
	return off, nil
}
