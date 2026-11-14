package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/ubgo/agentio"
)

// PromptEntry is one history.jsonl line: {session_id, ts, text}. Codex
// records no directory.
type PromptEntry struct {
	SessionID string
	Time      time.Time
	Text      string
}

type promptWire struct {
	SessionID string `json:"session_id"`
	TS        int64  `json:"ts"` // Unix seconds
	Text      string `json:"text"`
}

// historyChunk is how far back each reverse-scan step reads.
const historyChunk = 64 << 10

// PromptHistory returns prompts newest first, scanning history.jsonl from
// the tail in chunks so a Limit reads only the end of the file.
// q.ConversationID filters by session, q.Since stops the scan; q.Dir
// cannot match (Codex records no dir) and returns nothing when set.
// Disk · Read.
func (a *Agent) PromptHistory(h Home, q agentio.PromptQuery) ([]PromptEntry, error) {
	if q.Dir != "" {
		return nil, nil
	}
	path := a.Paths(h).History
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: open %s: %w", fileHistory, err)
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("codex: stat %s: %w", fileHistory, err)
	}
	var (
		out   []PromptEntry
		carry []byte
		end   = st.Size()
	)
	for end > 0 {
		start := max(0, end-historyChunk)
		buf := make([]byte, end-start)
		if _, err := f.ReadAt(buf, start); err != nil {
			return nil, fmt.Errorf("codex: read %s: %w", fileHistory, err)
		}
		buf = append(buf, carry...)
		lines := bytes.Split(buf, []byte{newline})
		if start > 0 {
			carry = append([]byte{}, lines[0]...) // may be cut: finish it on the next step
			lines = lines[1:]
		} else {
			carry = nil
		}
		for i := len(lines) - 1; i >= 0; i-- {
			var w promptWire
			if json.Unmarshal(bytes.TrimSpace(lines[i]), &w) != nil {
				continue
			}
			e := PromptEntry{SessionID: w.SessionID, Time: unixTime(w.TS), Text: w.Text}
			if !q.Since.IsZero() && e.Time.Before(q.Since) {
				return out, nil
			}
			if q.ConversationID != "" && e.SessionID != q.ConversationID {
				continue
			}
			out = append(out, e)
			if q.Limit > 0 && len(out) >= q.Limit {
				return out, nil
			}
		}
		end = start
	}
	return out, nil
}

// Neutral maps onto agentio.PromptEntry. Pure.
func (e PromptEntry) Neutral() agentio.PromptEntry {
	return agentio.PromptEntry{Time: e.Time, Text: e.Text, ConversationID: e.SessionID}
}

// ReasoningLevel is one supported_reasoning_levels entry.
type ReasoningLevel struct {
	Effort      ReasoningEffort `json:"effort"`
	Description string          `json:"description,omitempty"`
}

// ModelUpgrade is a model's `upgrade` hint; shape kept raw until a non-null
// sample is recorded.
type ModelUpgrade struct {
	Raw agentio.Raw
}

// ModelInfo is one models_cache.json model.
type ModelInfo struct {
	Slug                    string
	DisplayName             string
	Description             string
	Visible                 bool // visibility == "list"
	Visibility              string
	SupportedInAPI          bool
	Priority                int
	DefaultReasoning        ReasoningEffort
	ReasoningLevels         []ReasoningLevel
	ContextWindow           int64
	MaxContextWindow        int64
	EffectiveContextPercent int
	InputModalities         []string
	DefaultVerbosity        Verbosity
	SupportsVerbosity       bool
	ServiceTiers            []string
	Upgrade                 *ModelUpgrade
	Unknown                 agentio.Unknown // ~25 more tool/behaviour flags kept raw
}

type modelWire struct {
	Slug                    string            `json:"slug"`
	DisplayName             string            `json:"display_name"`
	Description             string            `json:"description"`
	DefaultReasoningLevel   ReasoningEffort   `json:"default_reasoning_level"`
	SupportedReasoning      []ReasoningLevel  `json:"supported_reasoning_levels"`
	Visibility              string            `json:"visibility"`
	SupportedInAPI          bool              `json:"supported_in_api"`
	Priority                int               `json:"priority"`
	ContextWindow           int64             `json:"context_window"`
	MaxContextWindow        int64             `json:"max_context_window"`
	EffectiveContextPercent int               `json:"effective_context_window_percent"`
	InputModalities         []string          `json:"input_modalities"`
	DefaultVerbosity        Verbosity         `json:"default_verbosity"`
	SupportVerbosity        bool              `json:"support_verbosity"`
	ServiceTiers            []json.RawMessage `json:"service_tiers"`
	Upgrade                 json.RawMessage   `json:"upgrade"`
}

// visibilityList marks a model shown in pickers.
const visibilityList = "list"

// ModelCatalog is models_cache.json.
type ModelCatalog struct {
	FetchedAt     time.Time
	ClientVersion agentio.Version
	ETag          string
	Models        []ModelInfo
}

type catalogWire struct {
	FetchedAt     string            `json:"fetched_at"`
	ETag          string            `json:"etag"`
	ClientVersion string            `json:"client_version"`
	Models        []json.RawMessage `json:"models"`
}

// ParseModelCatalog decodes models_cache.json bytes. A model that fails to
// decode is skipped with a warning. Pure.
func ParseModelCatalog(b []byte) (ModelCatalog, agentio.Warnings, error) {
	var w catalogWire
	if err := json.Unmarshal(b, &w); err != nil {
		return ModelCatalog{}, nil, fmt.Errorf("codex: decode %s: %w", fileModelsCache, err)
	}
	cv, _ := agentio.ParseVersion(w.ClientVersion)
	cat := ModelCatalog{FetchedAt: parseRFC3339(w.FetchedAt), ClientVersion: cv, ETag: w.ETag}
	var warns agentio.Warnings
	known := modelKnownKeys()
	for _, raw := range w.Models {
		var m modelWire
		if err := json.Unmarshal(raw, &m); err != nil {
			warns.Add(agentio.WarnTypeChanged, fileModelsCache, "models", "a model entry did not decode")
			continue
		}
		info := ModelInfo{
			Slug: m.Slug, DisplayName: m.DisplayName, Description: m.Description,
			Visible: m.Visibility == visibilityList, Visibility: m.Visibility,
			SupportedInAPI: m.SupportedInAPI, Priority: m.Priority,
			DefaultReasoning: m.DefaultReasoningLevel, ReasoningLevels: m.SupportedReasoning,
			ContextWindow: m.ContextWindow, MaxContextWindow: m.MaxContextWindow,
			EffectiveContextPercent: m.EffectiveContextPercent, InputModalities: m.InputModalities,
			DefaultVerbosity: m.DefaultVerbosity, SupportsVerbosity: m.SupportVerbosity,
		}
		for _, t := range m.ServiceTiers {
			var s string
			if json.Unmarshal(t, &s) == nil {
				info.ServiceTiers = append(info.ServiceTiers, s)
			}
		}
		if len(bytes.TrimSpace(m.Upgrade)) > 0 && string(bytes.TrimSpace(m.Upgrade)) != jsonNull {
			info.Upgrade = &ModelUpgrade{Raw: agentio.Raw(m.Upgrade)}
		}
		info.Unknown, _ = agentio.ExtractUnknown(raw, known)
		cat.Models = append(cat.Models, info)
	}
	return cat, warns, nil
}

const jsonNull = "null"

// Models reads models_cache.json. Disk · Read.
func (a *Agent) Models(h Home) (ModelCatalog, error) {
	b, err := readFile(a.Paths(h).ModelsCache)
	if err != nil {
		return ModelCatalog{}, fmt.Errorf("codex: read %s: %w", fileModelsCache, err)
	}
	cat, _, err := ParseModelCatalog(b)
	return cat, err
}

// ErrUnknownModel means a model slug is not in the catalog.
var ErrUnknownModel = errors.New("codex: model not in the catalog")

// ErrUnsupportedEffort means a model does not advertise a reasoning effort.
var ErrUnsupportedEffort = errors.New("codex: reasoning effort not supported by the model")

// Validate checks model (and effort, when set) against the catalog. Pure.
func (c ModelCatalog) Validate(model string, effort ReasoningEffort) error {
	for _, m := range c.Models {
		if m.Slug != model {
			continue
		}
		if effort == "" || len(m.ReasoningLevels) == 0 {
			return nil
		}
		for _, l := range m.ReasoningLevels {
			if l.Effort == effort {
				return nil
			}
		}
		return fmt.Errorf("%w: %s on %s", ErrUnsupportedEffort, effort, model)
	}
	return fmt.Errorf("%w: %s", ErrUnknownModel, model)
}

// Neutral maps onto agentio.Model. Pure.
func (m ModelInfo) Neutral() agentio.Model {
	return agentio.Model{Slug: m.Slug, DisplayName: m.DisplayName, Visible: m.Visible, ContextWindow: m.ContextWindow}
}

// modelKnownKeys lists the models_cache.json model keys ModelInfo types.
func modelKnownKeys() []string {
	return []string{"slug", "display_name", "description", "default_reasoning_level", "supported_reasoning_levels",
		"visibility", "supported_in_api", "priority", "context_window", "max_context_window",
		"effective_context_window_percent", "input_modalities", "default_verbosity", "support_verbosity",
		"service_tiers", "upgrade"}
}
