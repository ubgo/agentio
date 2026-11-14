package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khanakia/agentcli"
)

// Wire kinds of /api/oauth/usage limits[].kind.
const (
	kindSession      = "session"
	kindWeeklyAll    = "weekly_all"
	kindWeeklyScoped = "weekly_scoped"
	groupSession     = "session"
	groupWeekly      = "weekly"
)

// Display labels for kinds whose label the payload does not carry.
const (
	labelSession      = "Current session"
	labelWeeklyAll    = "All models"
	labelScopedFallbk = "Scoped"
	labelOpus         = "Opus"
	labelSonnet       = "Sonnet"
	labelExtraUsage   = "Extra usage"
	vendorKindExtra   = "extra_usage"
)

// Window lengths of the Claude kinds: the session window is five hours,
// the weekly ones seven days (claude.ai /usage wording).
const (
	sessionLength = 5 * time.Hour
	weeklyLength  = 7 * 24 * time.Hour
)

// ErrNoLimits means a usage body parsed but carried no limit data in
// either shape; never mistaken for "0% used".
var ErrNoLimits = errors.New("claude: usage response has no limits")

// ErrNoOrganization means a profile body carried no organization.
var ErrNoOrganization = errors.New("claude: profile response has no organization")

// ErrProbeHeaders means a probe answered without usable rate-limit headers.
var ErrProbeHeaders = errors.New("claude: probe response has no usable rate-limit headers")

// Usage is the rich usage result: the neutral usage plus the decoded
// extra_usage object and the wire rows.
type Usage struct {
	agentcli.Usage
	Extra  ExtraUsage
	Limits []Limit
}

// Neutral returns the embedded neutral usage. cost: Pure.
func (u Usage) Neutral() agentcli.Usage { return u.Usage }

// ExtraUsage is pay-as-you-go overage beyond the plan.
type ExtraUsage struct {
	IsEnabled   bool             `json:"is_enabled"`
	Utilization *float64         `json:"utilization"`
	Unknown     agentcli.Unknown `json:"-"`
}

// Limit is one row of limits[] in /api/oauth/usage, in the wire shape.
type Limit struct {
	Kind     string     `json:"kind"`
	Group    string     `json:"group"`
	Percent  float64    `json:"percent"`
	Severity string     `json:"severity"`
	ResetsAt *time.Time `json:"resets_at"`
	Scope    *Scope     `json:"scope"`
	IsActive bool       `json:"is_active"`
}

// Scope narrows a weekly_scoped limit to one model or surface.
type Scope struct {
	Model   *ScopeRef       `json:"model"`
	Surface json.RawMessage `json:"surface"`
}

// ScopeRef names the model a scoped limit applies to.
type ScopeRef struct {
	ID          *string `json:"id"`
	DisplayName *string `json:"display_name"`
}

// Label is the claude.ai wording of a limit row; unknown kinds fall back to
// their raw wire value. cost: Pure.
func (l Limit) Label() string {
	switch l.Kind {
	case kindSession:
		return labelSession
	case kindWeeklyAll:
		return labelWeeklyAll
	case kindWeeklyScoped:
		if n := l.scopeName(); n != "" {
			return n
		}
		return labelScopedFallbk
	}
	return l.Kind
}

func (l Limit) scopeName() string {
	if l.Scope != nil && l.Scope.Model != nil && l.Scope.Model.DisplayName != nil {
		return *l.Scope.Model.DisplayName
	}
	return ""
}

// length maps a wire kind to its window length; 0 when unknown.
func (l Limit) length() time.Duration {
	switch l.Kind {
	case kindSession:
		return sessionLength
	case kindWeeklyAll, kindWeeklyScoped:
		return weeklyLength
	}
	switch l.Group {
	case groupSession:
		return sessionLength
	case groupWeekly:
		return weeklyLength
	}
	return 0
}

func (l Limit) window() agentcli.Window {
	return agentcli.Window{Length: l.length(), Scope: l.scopeName(), Label: l.Label(), Percent: l.Percent, ResetsAt: l.ResetsAt, VendorKind: l.Kind}
}

// wireUsage is the subset of /api/oauth/usage read: the current limits
// list, or the legacy per-window objects when limits is absent.
type wireUsage struct {
	Limits         []Limit         `json:"limits"`
	ExtraUsage     json.RawMessage `json:"extra_usage"`
	FiveHour       *wireWindow     `json:"five_hour"`
	SevenDay       *wireWindow     `json:"seven_day"`
	SevenDayOpus   *wireWindow     `json:"seven_day_opus"`
	SevenDaySonnet *wireWindow     `json:"seven_day_sonnet"`
}

type wireWindow struct {
	Utilization float64    `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
}

// ParseUsage decodes a usage body (network or cached). A nil error always
// comes with at least one window. Source and ObservedAt are left for the
// caller. cost: Pure.
func ParseUsage(body []byte) (Usage, error) {
	var w wireUsage
	if err := json.Unmarshal(body, &w); err != nil {
		return Usage{}, fmt.Errorf("claude: decode usage: %w", err)
	}
	limits := w.Limits
	if len(limits) == 0 {
		limits = legacyLimits(w)
	}
	if len(limits) == 0 {
		return Usage{}, ErrNoLimits
	}
	u := Usage{Usage: agentcli.Usage{Agent: ID, Billing: agentcli.BillingSubscription, Raw: agentcli.Raw(bytes.Clone(body))}, Limits: limits}
	for _, l := range limits {
		u.Windows = append(u.Windows, l.window())
	}
	if len(w.ExtraUsage) > 0 && string(w.ExtraUsage) != jsonNull {
		extra, err := parseExtra(w.ExtraUsage)
		if err != nil {
			return Usage{}, err
		}
		u.Extra = extra
		if extra.IsEnabled && extra.Utilization != nil {
			u.Windows = append(u.Windows, agentcli.Window{Label: labelExtraUsage, Percent: *extra.Utilization, VendorKind: vendorKindExtra})
		}
	}
	return u, nil
}

func parseExtra(b []byte) (ExtraUsage, error) {
	type plain ExtraUsage
	var v plain
	unknown, _, err := decodeLenient(vendorKindExtra, b, reflect.TypeFor[plain](), func(d []byte) error { return json.Unmarshal(d, &v) })
	if err != nil {
		return ExtraUsage{}, err
	}
	e := ExtraUsage(v)
	e.Unknown = unknown
	return e, nil
}

// ExtraUsageOf decodes the extra_usage object from a neutral usage's Raw
// body. cost: Pure.
func ExtraUsageOf(u agentcli.Usage) (ExtraUsage, error) {
	var w wireUsage
	if err := json.Unmarshal(u.Raw, &w); err != nil {
		return ExtraUsage{}, fmt.Errorf("claude: decode usage: %w", err)
	}
	if len(w.ExtraUsage) == 0 || string(w.ExtraUsage) == jsonNull {
		return ExtraUsage{}, nil
	}
	return parseExtra(w.ExtraUsage)
}

func legacyLimits(w wireUsage) []Limit {
	var out []Limit
	add := func(win *wireWindow, kind, group, model string) {
		if win == nil {
			return
		}
		l := Limit{Kind: kind, Group: group, Percent: win.Utilization, ResetsAt: win.ResetsAt}
		if model != "" {
			name := model
			l.Scope = &Scope{Model: &ScopeRef{DisplayName: &name}}
		}
		out = append(out, l)
	}
	add(w.FiveHour, kindSession, groupSession, "")
	add(w.SevenDay, kindWeeklyAll, groupWeekly, "")
	add(w.SevenDayOpus, kindWeeklyScoped, groupWeekly, labelOpus)
	add(w.SevenDaySonnet, kindWeeklyScoped, groupWeekly, labelSonnet)
	return out
}

// tierMultiplier pulls "20x" out of "default_claude_max_20x".
func tierMultiplier() *regexp.Regexp { return regexp.MustCompile(`_(\d+x)$`) }

// orgTypePrefix is stripped from organization_type ("claude_max" → "max").
const orgTypePrefix = "claude_"

// PlanLabel renders the plan the way claude.ai does, e.g. "Max (20x)"; an
// empty subscription yields "". cost: Pure.
func PlanLabel(subscriptionType, rateLimitTier string) string {
	subscriptionType = strings.TrimPrefix(subscriptionType, orgTypePrefix)
	if subscriptionType == "" {
		return ""
	}
	name := strings.ToUpper(subscriptionType[:1]) + subscriptionType[1:]
	if m := tierMultiplier().FindStringSubmatch(rateLimitTier); m != nil {
		return name + " (" + m[1] + ")"
	}
	return name
}

type wireProfile struct {
	Organization *struct {
		OrganizationType      string     `json:"organization_type"`
		RateLimitTier         string     `json:"rate_limit_tier"`
		BillingType           string     `json:"billing_type"`
		SubscriptionStatus    string     `json:"subscription_status"`
		SubscriptionCreatedAt *time.Time `json:"subscription_created_at"`
	} `json:"organization"`
}

// ParseProfile decodes /api/oauth/profile into a Subscription; personal
// fields are not decoded. cost: Pure.
func ParseProfile(body []byte) (agentcli.Subscription, error) {
	var w wireProfile
	if err := json.Unmarshal(body, &w); err != nil {
		return agentcli.Subscription{}, fmt.Errorf("claude: decode profile: %w", err)
	}
	if w.Organization == nil {
		return agentcli.Subscription{}, ErrNoOrganization
	}
	o := w.Organization
	return agentcli.Subscription{
		Plan: PlanLabel(o.OrganizationType, o.RateLimitTier), Status: o.SubscriptionStatus,
		BillingType: o.BillingType, StartedAt: o.SubscriptionCreatedAt,
	}, nil
}

// Probe header names (verified on the real API 2026-10-16, agx probe.go).
// Utilization is a fraction 0..1; reset is unix seconds.
const (
	hdrPrefix          = "anthropic-ratelimit-unified-"
	hdrUtilization     = "-utilization"
	hdrReset           = "-reset"
	hdrStatus          = "-status"
	hdrRepresentative  = hdrPrefix + "representative-claim"
	probeInfix5h       = "5h"
	probeInfix7d       = "7d"
	claim5h            = "five_hour"
	claim7d            = "seven_day"
	percentPerFraction = 100
)

// ParseProbeHeaders maps anthropic-ratelimit-unified-* headers onto the
// usage windows. A window whose utilization and reset are both absent is
// one the account does not have and is omitted; half a pair or a malformed
// value is an error naming the header. cost: Pure.
func ParseProbeHeaders(h http.Header) (agentcli.Usage, error) {
	claim := h.Get(hdrRepresentative)
	u := agentcli.Usage{Agent: ID, Billing: agentcli.BillingSubscription, Source: agentcli.UsageProbe}
	var absent []string
	for _, pw := range []struct{ infix, kind, group, claim string }{
		{probeInfix5h, kindSession, groupSession, claim5h},
		{probeInfix7d, kindWeeklyAll, groupWeekly, claim7d},
	} {
		base := hdrPrefix + pw.infix
		utilRaw, resetRaw := h.Get(base+hdrUtilization), h.Get(base+hdrReset)
		if utilRaw == "" && resetRaw == "" {
			absent = append(absent, base+hdrUtilization)
			continue
		}
		if utilRaw == "" || resetRaw == "" {
			return agentcli.Usage{}, fmt.Errorf("%w: %s pair incomplete", ErrProbeHeaders, base)
		}
		util, err := strconv.ParseFloat(utilRaw, 64)
		if err != nil || util < 0 || math.IsNaN(util) || math.IsInf(util, 0) {
			return agentcli.Usage{}, fmt.Errorf("%w: %s is not a fraction", ErrProbeHeaders, base+hdrUtilization)
		}
		secs, err := strconv.ParseInt(resetRaw, 10, 64)
		if err != nil || secs <= 0 {
			return agentcli.Usage{}, fmt.Errorf("%w: %s is not a unix time", ErrProbeHeaders, base+hdrReset)
		}
		reset := time.Unix(secs, 0).UTC()
		l := Limit{Kind: pw.kind, Group: pw.group, Percent: util * percentPerFraction, Severity: h.Get(base + hdrStatus), ResetsAt: &reset, IsActive: claim == pw.claim}
		u.Windows = append(u.Windows, l.window())
	}
	if len(u.Windows) == 0 {
		return agentcli.Usage{}, fmt.Errorf("%w: %s missing", ErrProbeHeaders, strings.Join(absent, ", "))
	}
	return u, nil
}

// scopeErrorCode is error.details.error_code on a 403 whose token is valid
// but lacks the profile scope (agx real sample 2026-11-01).
const scopeErrorCode = "oauth_scope_insufficient"

func scopeInsufficient(body []byte) bool {
	var w struct {
		Error struct {
			Details struct {
				ErrorCode string `json:"error_code"`
			} `json:"details"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &w) == nil && w.Error.Details.ErrorCode == scopeErrorCode
}

// snippet trims an error body to one line at a rune boundary.
func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > snippetMax {
		cut := snippetMax
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		return s[:cut] + "…"
	}
	return s
}

// redactedMark replaces a token wherever an error body repeats it.
const redactedMark = "[redacted]"

func redactIn(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, redactedMark)
}

// get performs one authenticated GET and maps failures onto the root
// sentinels: 401/403 → ErrUnauthorized, scope 403 → ErrScopeLimited,
// 429 → ErrRateLimited. No retry (invariant 7). The token never appears in
// an error.
func (a *Agent) get(ctx context.Context, path string, tok agentcli.Token) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("claude: build request: %w", err)
	}
	req.Header.Set(headerAuthorization, bearerPrefix+tok.Reveal())
	req.Header.Set(oauthBetaHeader, oauthBetaValue)
	req.Header.Set(headerAccept, mimeJSON)
	req.Header.Set(headerUserAgent, userAgent)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude: request %s: %w", path, err)
	}
	// The body is fully read (bounded) below; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("claude: read response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusForbidden && scopeInsufficient(body):
		return nil, fmt.Errorf("%w: use a home login, not a `claude setup-token` token, for account reads", agentcli.ErrScopeLimited)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w (HTTP %d%s)", agentcli.ErrUnauthorized, resp.StatusCode, apiMessage(body, tok.Reveal()))
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, agentcli.RateLimitError(resp.Header, a.now())
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("claude: unexpected HTTP %d: %s", resp.StatusCode, snippet([]byte(redactIn(string(body), tok.Reveal()))))
	}
	return body, nil
}

func apiMessage(body []byte, token string) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error.Message == "" {
		return ""
	}
	return ": " + snippet([]byte(redactIn(e.Error.Message, token)))
}

// httpClient is Host.HTTP, or a client with defaultTimeout.
func (a *Agent) httpClient() *http.Client {
	if a.host.HTTP != nil {
		return a.host.HTTP
	}
	return &http.Client{Timeout: defaultTimeout}
}

// now reads Host.Now; a zero time when the host has no clock.
func (a *Agent) now() time.Time {
	if a.host.Now == nil {
		return time.Time{}
	}
	return a.host.Now()
}

// defaultTimeout bounds one usage request end to end.
const defaultTimeout = 15 * time.Second

// usableCredential resolves the home's login and refuses expired ones
// before any request (never refreshed, invariant 1).
func (a *Agent) usableCredential(ctx context.Context, h Home) (Credential, error) {
	c, err := a.CredentialContext(ctx, h)
	if err != nil {
		return Credential{}, err
	}
	if c.Expired(a.now()) {
		return Credential{}, fmt.Errorf("%w at %s: run claude once in this home to refresh it", agentcli.ErrExpired, c.ExpiresAt.Format(time.RFC3339))
	}
	return c, nil
}

// Usage fetches /api/oauth/usage for h's login once. cost: Network · Read
// (may 429; cache, never poll).
func (a *Agent) Usage(ctx context.Context, h Home) (Usage, error) {
	c, err := a.usableCredential(ctx, h)
	if err != nil {
		return Usage{}, err
	}
	body, err := a.get(ctx, usagePath, c.AccessToken)
	if err != nil {
		return Usage{}, err
	}
	u, err := ParseUsage(body)
	if err != nil {
		return Usage{}, err
	}
	u.Source, u.ObservedAt = agentcli.UsageServer, a.now()
	return u, nil
}

// Subscription fetches /api/oauth/profile for h's login once.
// cost: Network · Read.
func (a *Agent) Subscription(ctx context.Context, h Home) (agentcli.Subscription, error) {
	c, err := a.usableCredential(ctx, h)
	if err != nil {
		return agentcli.Subscription{}, err
	}
	body, err := a.get(ctx, profilePath, c.AccessToken)
	if err != nil {
		return agentcli.Subscription{}, err
	}
	s, err := ParseProfile(body)
	if err == nil {
		s.Raw = agentcli.Raw(body)
	}
	return s, err
}

// CachedUsage reads cachedUsageUtilization from .claude.json: usage with
// no network call, Source Cache, ObservedAt = fetchedAtMs. found is false
// when the home has no cached body. cost: Disk · Read.
func (a *Agent) CachedUsage(h Home) (Usage, bool, error) {
	c, err := a.CachedUsageBody(h)
	if err != nil || c == nil || len(c.Body) == 0 {
		return Usage{}, false, err
	}
	u, err := ParseUsage(c.Body)
	if err != nil {
		return Usage{}, true, err
	}
	u.Source, u.ObservedAt = agentcli.UsageCache, c.FetchedAt
	return u, true, nil
}
