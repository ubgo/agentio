package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/khanakia/agentcli"
)

// statsVersion is the only stats-cache.json format version with a sample.
const statsVersion = 5

// hoursPerDay sizes Stats.HourCounts.
const hoursPerDay = 24

// DailyActivity is one dailyActivity row.
type DailyActivity struct {
	Date          string `json:"date"`
	MessageCount  int64  `json:"messageCount"`
	SessionCount  int64  `json:"sessionCount"`
	ToolCallCount int64  `json:"toolCallCount"`
}

// DailyTokens is one dailyModelTokens row.
type DailyTokens struct {
	Date          string           `json:"date"`
	TokensByModel map[string]int64 `json:"tokensByModel"`
}

// LongestSession is longestSession.
type LongestSession struct {
	SessionID    string
	Duration     time.Duration
	MessageCount int64
	Timestamp    time.Time
}

// Stats is stats-cache.json (feeds /usage). Raw holds the file when the
// version is not 5.
type Stats struct {
	Version       int
	LastComputed  string // date "2026-11-11"
	FirstSession  time.Time
	TotalSessions int64
	TotalMessages int64
	Daily         []DailyActivity
	DailyTokens   []DailyTokens
	ModelUsage    map[string]ModelUsage
	Longest       *LongestSession
	HourCounts    [hoursPerDay]int64
	Raw           agentcli.Raw
	Unknown       agentcli.Unknown
}

type wireStats struct {
	Version                 int                       `json:"version"`
	LastComputedDate        string                    `json:"lastComputedDate"`
	FirstSessionDate        time.Time                 `json:"firstSessionDate"`
	TotalSessions           int64                     `json:"totalSessions"`
	TotalMessages           int64                     `json:"totalMessages"`
	DailyActivity           []DailyActivity           `json:"dailyActivity"`
	DailyModelTokens        []DailyTokens             `json:"dailyModelTokens"`
	DailyModelTokensVersion int                       `json:"dailyModelTokensVersion"`
	ModelUsage              map[string]wireModelUsage `json:"modelUsage"`
	LongestSession          *struct {
		SessionID    string    `json:"sessionId"`
		Duration     float64   `json:"duration"`
		MessageCount int64     `json:"messageCount"`
		Timestamp    time.Time `json:"timestamp"`
	} `json:"longestSession"`
	HourCounts map[string]int64 `json:"hourCounts"`
}

// ParseStats decodes a stats-cache.json body; a version other than 5 is a
// wrapped ErrUnknownVersion with Version and Raw set. cost: Pure.
func ParseStats(data []byte) (Stats, error) {
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return Stats{}, fmt.Errorf("claude: decode %s: %w", statsCacheFile, err)
	}
	if head.Version != statsVersion {
		return Stats{Version: head.Version, Raw: agentcli.Raw(data)}, fmt.Errorf("%w: %s version %d", agentcli.ErrUnknownVersion, statsCacheFile, head.Version)
	}
	var w wireStats
	unknown, _, err := decodeLenient(statsCacheFile, data, reflect.TypeFor[wireStats](), func(b []byte) error { w = wireStats{}; return json.Unmarshal(b, &w) })
	if err != nil {
		return Stats{}, err
	}
	s := Stats{Version: w.Version, LastComputed: w.LastComputedDate, FirstSession: w.FirstSessionDate,
		TotalSessions: w.TotalSessions, TotalMessages: w.TotalMessages, Daily: w.DailyActivity, DailyTokens: w.DailyModelTokens,
		ModelUsage: modelUsageMap(w.ModelUsage), Unknown: unknown}
	if l := w.LongestSession; l != nil {
		s.Longest = &LongestSession{SessionID: l.SessionID, Duration: msDuration(l.Duration), MessageCount: l.MessageCount, Timestamp: l.Timestamp}
	}
	for k, v := range w.HourCounts {
		if h, err := strconv.Atoi(k); err == nil && h >= 0 && h < hoursPerDay {
			s.HourCounts[h] = v
		}
	}
	return s, nil
}

// Stats reads <home>/stats-cache.json; the zero Stats with a nil error
// when absent. cost: Disk · Read.
func (a *Agent) Stats(h Home) (Stats, error) {
	p := filepath.Join(h.Dir(), statsCacheFile)
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil || !found {
		return Stats{}, err
	}
	return ParseStats(data)
}
