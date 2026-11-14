package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/khanakia/agentcli"
)

// historyChunk is how much of history.jsonl one reverse step reads; the
// file is ~11 MB on a busy machine and a typical query wants the newest few.
const historyChunk = 256 << 10

// pasteExt is the paste-cache file extension.
const pasteExt = ".txt"

// HistoryEntry is one prompt in history.jsonl.
type HistoryEntry struct {
	Display   string
	Pasted    []Paste
	Time      time.Time
	Project   string
	SessionID string
}

// Paste is one pasted block; ContentHash points into paste-cache when the
// content was too large to inline.
type Paste struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`
}

// HistoryQuery narrows PromptHistory; zero fields do not filter.
type HistoryQuery struct {
	Project   string
	SessionID string
	Since     time.Time
	// Limit caps results, newest first; <= 0 = no cap.
	Limit int
}

type wireHistory struct {
	Display        string          `json:"display"`
	PastedContents json.RawMessage `json:"pastedContents"`
	Timestamp      millis          `json:"timestamp"`
	Project        string          `json:"project"`
	SessionID      string          `json:"sessionId"`
}

// parsePasted accepts both seen forms: [] (or {}) when empty, and an
// object keyed by paste id when filled.
func parsePasted(raw json.RawMessage) []Paste {
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var m map[string]Paste
	if json.Unmarshal(raw, &m) != nil || len(m) == 0 {
		return nil
	}
	out := make([]Paste, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ParseHistoryLine decodes one history.jsonl line. cost: Pure.
func ParseHistoryLine(line []byte) (HistoryEntry, error) {
	var w wireHistory
	if err := json.Unmarshal(line, &w); err != nil {
		return HistoryEntry{}, fmt.Errorf("claude: history line: %w", err)
	}
	return HistoryEntry{Display: w.Display, Pasted: parsePasted(w.PastedContents), Time: w.Timestamp.Time(), Project: w.Project, SessionID: w.SessionID}, nil
}

func (q HistoryQuery) match(e HistoryEntry) bool {
	return (q.Project == "" || filepath.Clean(q.Project) == filepath.Clean(e.Project)) &&
		(q.SessionID == "" || q.SessionID == e.SessionID) &&
		(q.Since.IsZero() || !e.Time.Before(q.Since))
}

// PromptHistory reads history.jsonl backwards from the tail in chunks and
// returns matching entries newest first; it stops at Limit, or at the
// first entry older than Since. cost: Disk · Read (reverse scan).
func (a *Agent) PromptHistory(h Home, q HistoryQuery) ([]HistoryEntry, error) {
	p := filepath.Join(h.Dir(), historyFile)
	f, err := os.Open(p)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: open history: %w", err)
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var out []HistoryEntry
	end := info.Size()
	var carry []byte // a partial first line of the previous chunk
	for end > 0 {
		start := max(end-historyChunk, 0)
		buf := make([]byte, end-start, end-start+int64(len(carry)))
		if _, err := f.ReadAt(buf, start); err != nil {
			return out, fmt.Errorf("claude: read history: %w", err)
		}
		buf = append(buf, carry...)
		lines := splitLines(buf)
		if start > 0 && len(lines) > 0 {
			carry, lines = lines[0], lines[1:]
		} else {
			carry = nil
		}
		for i := len(lines) - 1; i >= 0; i-- {
			e, err := ParseHistoryLine(lines[i])
			if err != nil {
				continue
			}
			if !q.Since.IsZero() && e.Time.Before(q.Since) {
				return out, nil
			}
			if q.match(e) {
				out = append(out, e)
				if q.Limit > 0 && len(out) >= q.Limit {
					return out, nil
				}
			}
		}
		end = start
	}
	return out, nil
}

// PastedContent reads paste-cache/<hash>.txt (bounded). cost: Disk · Read.
func (a *Agent) PastedContent(h Home, hash string) (string, error) {
	if hash == "" || strings.ContainsAny(hash, `/\.`) {
		return "", fmt.Errorf("%w: paste %q", agentcli.ErrNotFound, hash)
	}
	data, found, err := readFileBounded(filepath.Join(h.Dir(), pasteCacheDir, hash+pasteExt), maxJSONFileBytes)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%w: paste %s", agentcli.ErrNotFound, hash)
	}
	return string(data), nil
}

// neutral converts a history entry.
func (e HistoryEntry) neutral() agentcli.PromptEntry {
	return agentcli.PromptEntry{Time: e.Time, Text: e.Display, Dir: e.Project, ConversationID: e.SessionID}
}
