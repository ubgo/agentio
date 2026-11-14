package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// GlobalConfig is the typed part of ~/.claude.json (default home) or
// <home>/.claude.json (other homes): install facts, the logged-in account,
// user-scope MCP servers, the per-project state map and the cached usage
// body. The ~100 UI flags and caches Claude Code keeps there stay in Unknown.
type GlobalConfig struct {
	Path                           string
	InstallMethod                  string
	AutoUpdates                    *bool
	AutoUpdatesProtectedForNative  *bool
	NumStartups                    int
	FirstStartTime                 *time.Time
	FirstStartVersion              string
	LastReleaseNotesSeen           string
	MigrationVersion               int
	Theme, EditorMode              string
	Verbose, AutoConnectIDE        *bool
	UserID, AnonymousID, MachineID agentio.Personal
	OAuthAccount                   *OAuthAccount
	MCPServers                     map[string]MCPServer
	Projects                       map[string]ProjectState
	CachedUsage                    *CachedUsage
	Unknown                        agentio.Unknown
	Warnings                       agentio.Warnings
}

// OAuthAccount is `oauthAccount`: who is logged in. Every id, email and name
// is agentio.Personal (OD-6) and prints redacted.
type OAuthAccount struct {
	AccountUUID, EmailAddress, OrganizationUUID, OrganizationName agentio.Personal
	OrganizationType                                              OrgType
	OrganizationRole                                              *string
	WorkspaceRole                                                 *string
	OrganizationRateLimitTier                                     *string
	UserRateLimitTier                                             *string
	BillingType                                                   BillingType
	PlanDisplayName, SeatTier                                     *string
	HasExtraUsageEnabled                                          bool
	DisplayName, FullName                                         agentio.Personal
	AccountCreatedAt                                              *time.Time
	SubscriptionCreatedAt                                         *time.Time
	ProfileFetchedAt                                              time.Time
	TrialEndsAt                                                   *time.Time
	TrialDurationDays                                             *int
	Unknown                                                       agentio.Unknown
}

// OrgType is `organizationType`: "claude_max" seen; others from agx label.go.
type OrgType string

// Known organization types.
const (
	OrgMax        OrgType = "claude_max"
	OrgPro        OrgType = "claude_pro"
	OrgTeam       OrgType = "claude_team"
	OrgEnterprise OrgType = "claude_enterprise"
)

// IsKnown reports whether t is a recorded organization type.
func (t OrgType) IsKnown() bool {
	switch t {
	case OrgMax, OrgPro, OrgTeam, OrgEnterprise:
		return true
	}
	return false
}

// BillingType is `billingType`: "stripe_subscription" seen.
type BillingType string

// BillingStripeSubscription is the only billing type recorded on disk.
const BillingStripeSubscription BillingType = "stripe_subscription"

// IsKnown reports whether b is a recorded billing type.
func (b BillingType) IsKnown() bool { return b == BillingStripeSubscription }

// ProjectState is one entry of `projects` (key = absolute folder).
type ProjectState struct {
	Dir                      string
	AllowedTools             []string
	TrustAccepted            bool
	MCPServers               map[string]MCPServer
	EnabledMcpjsonServers    []string
	DisabledMcpjsonServers   []string
	ExternalIncludesApproved bool
	Last                     LastSession
	Unknown                  agentio.Unknown
}

// LastSession is the `last*` group of a project entry: totals of the last
// session that ran in the folder.
type LastSession struct {
	SessionID        string
	Cost             float64
	Duration         time.Duration
	APIDuration      time.Duration
	ToolDuration     time.Duration
	LinesAdded       int
	LinesRemoved     int
	Tokens           TokenCounts
	ModelUsage       map[string]ModelUsage
	Metrics          map[string]float64
	GracefulShutdown *bool
	VersionBase      string
	StartTime        time.Time
}

// TokenCounts are Claude's token columns (camelCase files).
type TokenCounts struct{ Input, Output, CacheCreation, CacheRead, WebSearch int64 }

// Neutral maps onto the shared counts: cache reads are cached input, cache
// creation is a cache write; Claude reports no separate reasoning tokens.
func (t TokenCounts) Neutral() agentio.TokenCounts {
	return agentio.TokenCounts{Input: t.Input, CachedInput: t.CacheRead, CacheWrite: t.CacheCreation, Output: t.Output}
}

// ModelUsage is one model's totals (~/.claude.json lastModelUsage, stats-cache modelUsage).
type ModelUsage struct {
	TokenCounts
	CostUSD         float64
	ContextWindow   int
	MaxOutputTokens int
}

// CachedUsage is `cachedUsageUtilization`: a cached copy of the
// /api/oauth/usage body with its fetch time. Parse Body with ParseUsage.
type CachedUsage struct {
	FetchedAt   time.Time
	AccountUUID agentio.Personal
	Body        agentio.Raw
}

// wireGlobalConfig is the subset of ~/.claude.json the library types.
type wireGlobalConfig struct {
	InstallMethod                 string                     `json:"installMethod"`
	AutoUpdates                   *bool                      `json:"autoUpdates"`
	AutoUpdatesProtectedForNative *bool                      `json:"autoUpdatesProtectedForNative"`
	NumStartups                   int                        `json:"numStartups"`
	FirstStartTime                *time.Time                 `json:"firstStartTime"`
	FirstStartVersion             string                     `json:"firstStartVersion"`
	LastReleaseNotesSeen          string                     `json:"lastReleaseNotesSeen"`
	MigrationVersion              int                        `json:"migrationVersion"`
	Theme                         string                     `json:"theme"`
	EditorMode                    string                     `json:"editorMode"`
	Verbose                       *bool                      `json:"verbose"`
	AutoConnectIDE                *bool                      `json:"autoConnectIde"`
	UserID                        agentio.Personal           `json:"userID"`
	AnonymousID                   agentio.Personal           `json:"anonymousId"`
	MachineID                     agentio.Personal           `json:"machineID"`
	OAuthAccount                  json.RawMessage            `json:"oauthAccount"`
	MCPServers                    map[string]json.RawMessage `json:"mcpServers"`
	Projects                      map[string]json.RawMessage `json:"projects"`
	CachedUsage                   *wireCachedUsage           `json:"cachedUsageUtilization"`
}

type wireCachedUsage struct {
	FetchedAtMs millis           `json:"fetchedAtMs"`
	AccountUUID agentio.Personal `json:"accountUuid"`
	Utilization json.RawMessage  `json:"utilization"`
}

type wireOAuthAccount struct {
	AccountUUID               agentio.Personal `json:"accountUuid"`
	EmailAddress              agentio.Personal `json:"emailAddress"`
	OrganizationUUID          agentio.Personal `json:"organizationUuid"`
	OrganizationName          agentio.Personal `json:"organizationName"`
	OrganizationType          OrgType          `json:"organizationType"`
	OrganizationRole          *string          `json:"organizationRole"`
	WorkspaceRole             *string          `json:"workspaceRole"`
	OrganizationRateLimitTier *string          `json:"organizationRateLimitTier"`
	UserRateLimitTier         *string          `json:"userRateLimitTier"`
	BillingType               BillingType      `json:"billingType"`
	PlanDisplayName           *string          `json:"planDisplayName"`
	SeatTier                  *string          `json:"seatTier"`
	HasExtraUsageEnabled      bool             `json:"hasExtraUsageEnabled"`
	DisplayName               agentio.Personal `json:"displayName"`
	FullName                  agentio.Personal `json:"fullName"`
	AccountCreatedAt          *time.Time       `json:"accountCreatedAt"`
	SubscriptionCreatedAt     *time.Time       `json:"subscriptionCreatedAt"`
	ProfileFetchedAt          millis           `json:"profileFetchedAt"`
	TrialEndsAt               *time.Time       `json:"claudeCodeTrialEndsAt"`
	TrialDurationDays         *int             `json:"claudeCodeTrialDurationDays"`
}

type wireProjectState struct {
	AllowedTools                        []string                   `json:"allowedTools"`
	HasTrustDialogAccepted              bool                       `json:"hasTrustDialogAccepted"`
	MCPServers                          map[string]json.RawMessage `json:"mcpServers"`
	EnabledMcpjsonServers               []string                   `json:"enabledMcpjsonServers"`
	DisabledMcpjsonServers              []string                   `json:"disabledMcpjsonServers"`
	HasClaudeMdExternalIncludesApproved bool                       `json:"hasClaudeMdExternalIncludesApproved"`
	LastSessionID                       string                     `json:"lastSessionId"`
	LastCost                            float64                    `json:"lastCost"`
	LastDuration                        float64                    `json:"lastDuration"`
	LastAPIDuration                     float64                    `json:"lastAPIDuration"`
	LastToolDuration                    float64                    `json:"lastToolDuration"`
	LastLinesAdded                      int                        `json:"lastLinesAdded"`
	LastLinesRemoved                    int                        `json:"lastLinesRemoved"`
	LastTotalInputTokens                int64                      `json:"lastTotalInputTokens"`
	LastTotalOutputTokens               int64                      `json:"lastTotalOutputTokens"`
	LastTotalCacheCreationInputTokens   int64                      `json:"lastTotalCacheCreationInputTokens"`
	LastTotalCacheReadInputTokens       int64                      `json:"lastTotalCacheReadInputTokens"`
	LastTotalWebSearchRequests          int64                      `json:"lastTotalWebSearchRequests"`
	LastModelUsage                      map[string]wireModelUsage  `json:"lastModelUsage"`
	LastSessionMetrics                  map[string]float64         `json:"lastSessionMetrics"`
	LastGracefulShutdown                *bool                      `json:"lastGracefulShutdown"`
	LastVersionBase                     string                     `json:"lastVersionBase"`
	LastStartTime                       millis                     `json:"lastStartTime"`
}

type wireModelUsage struct {
	InputTokens              int64   `json:"inputTokens"`
	OutputTokens             int64   `json:"outputTokens"`
	CacheReadInputTokens     int64   `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int64   `json:"cacheCreationInputTokens"`
	WebSearchRequests        int64   `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
	ContextWindow            int     `json:"contextWindow"`
	MaxOutputTokens          int     `json:"maxOutputTokens"`
}

func (w wireModelUsage) typed() ModelUsage {
	return ModelUsage{
		TokenCounts: TokenCounts{Input: w.InputTokens, Output: w.OutputTokens, CacheCreation: w.CacheCreationInputTokens, CacheRead: w.CacheReadInputTokens, WebSearch: w.WebSearchRequests},
		CostUSD:     w.CostUSD, ContextWindow: w.ContextWindow, MaxOutputTokens: w.MaxOutputTokens,
	}
}

func modelUsageMap(in map[string]wireModelUsage) map[string]ModelUsage {
	if in == nil {
		return nil
	}
	out := make(map[string]ModelUsage, len(in))
	for k, v := range in {
		out[k] = v.typed()
	}
	return out
}

// ParseGlobalConfig decodes a .claude.json body. cost: Pure.
func ParseGlobalConfig(data []byte) (GlobalConfig, error) {
	var w wireGlobalConfig
	unknown, warns, err := decodeLenient(globalConfigFile, data, reflect.TypeFor[wireGlobalConfig](), func(b []byte) error { return json.Unmarshal(b, &w) })
	if err != nil {
		return GlobalConfig{}, err
	}
	g := GlobalConfig{
		InstallMethod: w.InstallMethod, AutoUpdates: w.AutoUpdates, AutoUpdatesProtectedForNative: w.AutoUpdatesProtectedForNative,
		NumStartups: w.NumStartups, FirstStartTime: w.FirstStartTime, FirstStartVersion: w.FirstStartVersion,
		LastReleaseNotesSeen: w.LastReleaseNotesSeen, MigrationVersion: w.MigrationVersion,
		Theme: w.Theme, EditorMode: w.EditorMode, Verbose: w.Verbose, AutoConnectIDE: w.AutoConnectIDE,
		UserID: w.UserID, AnonymousID: w.AnonymousID, MachineID: w.MachineID,
		Unknown: unknown, Warnings: warns,
	}
	if acct, err := parseOAuthAccount(w.OAuthAccount); err != nil {
		g.Warnings = append(g.Warnings, agentio.Warning{Kind: agentio.WarnTypeChanged, File: globalConfigFile, Key: "oauthAccount", Detail: err.Error()})
	} else {
		g.OAuthAccount = acct
	}
	g.MCPServers, g.Warnings = parseServerMap(w.MCPServers, MCPScopeUser, globalConfigFile, g.Warnings)
	if w.Projects != nil {
		g.Projects = make(map[string]ProjectState, len(w.Projects))
		for dir, raw := range w.Projects {
			ps, pw, err := parseProjectState(dir, raw)
			g.Warnings = append(g.Warnings, pw...)
			if err != nil {
				g.Warnings = append(g.Warnings, agentio.Warning{Kind: agentio.WarnTypeChanged, File: globalConfigFile, Key: "projects", Detail: "one project entry did not decode"})
				continue
			}
			g.Projects[dir] = ps
		}
	}
	if w.CachedUsage != nil {
		g.CachedUsage = &CachedUsage{FetchedAt: w.CachedUsage.FetchedAtMs.Time(), AccountUUID: w.CachedUsage.AccountUUID, Body: rawOrNil(w.CachedUsage.Utilization)}
	}
	return g, nil
}

func parseOAuthAccount(raw json.RawMessage) (*OAuthAccount, error) {
	if len(raw) == 0 || string(raw) == jsonNull {
		return nil, nil
	}
	var w wireOAuthAccount
	unknown, _, err := decodeLenient("oauthAccount", raw, reflect.TypeFor[wireOAuthAccount](), func(b []byte) error { return json.Unmarshal(b, &w) })
	if err != nil {
		return nil, err
	}
	if w.AccountUUID.IsZero() && w.EmailAddress.IsZero() {
		return nil, nil // an empty {} after logout
	}
	return &OAuthAccount{
		AccountUUID: w.AccountUUID, EmailAddress: w.EmailAddress, OrganizationUUID: w.OrganizationUUID, OrganizationName: w.OrganizationName,
		OrganizationType: w.OrganizationType, OrganizationRole: w.OrganizationRole, WorkspaceRole: w.WorkspaceRole,
		OrganizationRateLimitTier: w.OrganizationRateLimitTier, UserRateLimitTier: w.UserRateLimitTier,
		BillingType: w.BillingType, PlanDisplayName: w.PlanDisplayName, SeatTier: w.SeatTier,
		HasExtraUsageEnabled: w.HasExtraUsageEnabled, DisplayName: w.DisplayName, FullName: w.FullName,
		AccountCreatedAt: w.AccountCreatedAt, SubscriptionCreatedAt: w.SubscriptionCreatedAt,
		ProfileFetchedAt: w.ProfileFetchedAt.Time(), TrialEndsAt: w.TrialEndsAt, TrialDurationDays: w.TrialDurationDays,
		Unknown: unknown,
	}, nil
}

// msDuration converts the float millisecond durations of project state.
func msDuration(ms float64) time.Duration { return time.Duration(ms * float64(time.Millisecond)) }

func parseProjectState(dir string, raw json.RawMessage) (ProjectState, agentio.Warnings, error) {
	var w wireProjectState
	unknown, warns, err := decodeLenient(globalConfigFile+" projects", raw, reflect.TypeFor[wireProjectState](), func(b []byte) error { return json.Unmarshal(b, &w) })
	if err != nil {
		return ProjectState{}, warns, err
	}
	ps := ProjectState{
		Dir: dir, AllowedTools: w.AllowedTools, TrustAccepted: w.HasTrustDialogAccepted,
		EnabledMcpjsonServers: w.EnabledMcpjsonServers, DisabledMcpjsonServers: w.DisabledMcpjsonServers,
		ExternalIncludesApproved: w.HasClaudeMdExternalIncludesApproved,
		Last: LastSession{
			SessionID: w.LastSessionID, Cost: w.LastCost,
			Duration: msDuration(w.LastDuration), APIDuration: msDuration(w.LastAPIDuration), ToolDuration: msDuration(w.LastToolDuration),
			LinesAdded: w.LastLinesAdded, LinesRemoved: w.LastLinesRemoved,
			Tokens:     TokenCounts{Input: w.LastTotalInputTokens, Output: w.LastTotalOutputTokens, CacheCreation: w.LastTotalCacheCreationInputTokens, CacheRead: w.LastTotalCacheReadInputTokens, WebSearch: w.LastTotalWebSearchRequests},
			ModelUsage: modelUsageMap(w.LastModelUsage), Metrics: w.LastSessionMetrics,
			GracefulShutdown: w.LastGracefulShutdown, VersionBase: w.LastVersionBase, StartTime: w.LastStartTime.Time(),
		},
		Unknown: unknown,
	}
	ps.MCPServers, warns = parseServerMap(w.MCPServers, MCPScopeLocal, globalConfigFile, warns)
	return ps, warns, nil
}

// globalConfigPath is where a home's .claude.json lives: one level up for
// the default home, inside the dir for every other home (agx identity.go).
func (a *Agent) globalConfigPath(h Home) string {
	if h.IsDefault() {
		return filepath.Join(a.host.UserHome, globalConfigFile)
	}
	return filepath.Join(h.Dir(), globalConfigFile)
}

// GlobalConfig reads the home's .claude.json. A missing file is the zero
// value with Path set (a fresh home). cost: Disk · Read (file is ~MBs; one
// JSON decode).
func (a *Agent) GlobalConfig(h Home) (GlobalConfig, error) {
	p := a.globalConfigPath(h)
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil {
		return GlobalConfig{Path: p}, fmt.Errorf("claude: read %s: %w", p, err)
	}
	if !found {
		return GlobalConfig{Path: p}, nil
	}
	g, err := ParseGlobalConfig(data)
	g.Path = p
	return g, err
}

// Account returns the logged-in account from .claude.json, nil when none.
// cost: Disk · Read.
func (a *Agent) Account(h Home) (*OAuthAccount, error) {
	g, err := a.GlobalConfig(h)
	if err != nil {
		return nil, err
	}
	return g.OAuthAccount, nil
}

// ProjectState returns the per-project entry for dir, nil when Claude Code
// never ran there. cost: Disk · Read.
func (a *Agent) ProjectState(h Home, dir string) (*ProjectState, error) {
	g, err := a.GlobalConfig(h)
	if err != nil {
		return nil, err
	}
	ps, ok := g.Projects[filepath.Clean(dir)]
	if !ok {
		return nil, nil
	}
	return &ps, nil
}

// CachedUsageBody returns the raw cachedUsageUtilization, nil when absent.
// cost: Disk · Read.
func (a *Agent) CachedUsageBody(h Home) (*CachedUsage, error) {
	g, err := a.GlobalConfig(h)
	if err != nil {
		return nil, err
	}
	return g.CachedUsage, nil
}

// ConfigBackups lists <home>/backups/.claude.json.* newest name last.
// cost: Disk · Read.
func (a *Agent) ConfigBackups(h Home) ([]string, error) {
	dir := filepath.Join(h.Dir(), backupsDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), configBackupPrefix) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}
