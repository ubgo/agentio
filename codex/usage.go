package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ubgo/agentio"
)

// Usage endpoint and request details (agx provider/codex/codex.go, the
// endpoint the Codex CLI's /status reads; undocumented, parsed defensively).
const (
	UsageEndpoint    = "https://chatgpt.com/backend-api/wham/usage"
	headerAccountID  = "ChatGPT-Account-Id"
	headerAuth       = "Authorization"
	headerAccept     = "Accept"
	headerUserAgent  = "User-Agent"
	bearerPrefix     = "Bearer "
	mimeJSON         = "application/json"
	userAgent        = "agentio"
	maxResponseBytes = 1 << 20
	snippetMax       = 200
)

// Vendor window kinds.
const (
	vendorPrimary   = "primary_window"
	vendorSecondary = "secondary_window"
	vendorPrimaryL  = "primary"
	vendorSecondL   = "secondary"
)

// UsageWindow is one Codex rate-limit window.
type UsageWindow struct {
	Length   time.Duration // limit_window_seconds or window_minutes
	Percent  float64
	ResetsAt *time.Time
	LimitID  string // "codex" | "premium" | … (local snapshot only)
	Vendor   string // primary_window | secondary_window (server) or primary | secondary (snapshot)
}

// Usage is a Codex usage snapshot from the server or from a rollout.
type Usage struct {
	Email               agentio.Personal
	Plan                Plan
	LimitReached        bool
	Windows             []UsageWindow
	Credits             agentio.Raw // shape unseen (null on the sample home)
	IndividualLimit     agentio.Raw // shape unseen (null on the sample home)
	SpendControlReached *bool
	ObservedAt          time.Time // server fetch time for Usage, record time for CachedUsage
	Source              agentio.UsageSource
	Raw                 agentio.Raw // vendor body (server) or the token_count payload (cached)
}

type wireUsage struct {
	Email     string `json:"email"`
	PlanType  string `json:"plan_type"`
	RateLimit *struct {
		LimitReached    bool        `json:"limit_reached"`
		PrimaryWindow   *wireWindow `json:"primary_window"`
		SecondaryWindow *wireWindow `json:"secondary_window"`
	} `json:"rate_limit"`
}

type wireWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAt            *int64  `json:"reset_at"`
}

// ErrNoWindows means a usage response carried no rate-limit windows.
var ErrNoWindows = errors.New("codex: usage response has no rate-limit windows")

// ParseUsage decodes a wham/usage response body. A nil error always comes
// with at least one window. Pure.
func ParseUsage(body []byte) (Usage, error) {
	var w wireUsage
	if err := json.Unmarshal(body, &w); err != nil {
		return Usage{}, fmt.Errorf("codex: decode usage: %w", err)
	}
	u := Usage{Email: agentio.NewPersonal(w.Email), Source: agentio.UsageServer, Raw: agentio.Raw(body)}
	if w.PlanType != "" {
		u.Plan = ParsePlan(w.PlanType)
	}
	if w.RateLimit != nil {
		u.LimitReached = w.RateLimit.LimitReached
		for _, pair := range []struct {
			win    *wireWindow
			vendor string
		}{{w.RateLimit.PrimaryWindow, vendorPrimary}, {w.RateLimit.SecondaryWindow, vendorSecondary}} {
			if pair.win == nil {
				continue
			}
			uw := UsageWindow{
				Length:  time.Duration(pair.win.LimitWindowSeconds) * time.Second,
				Percent: pair.win.UsedPercent,
				Vendor:  pair.vendor,
			}
			if pair.win.ResetAt != nil {
				t := time.Unix(*pair.win.ResetAt, 0).UTC()
				uw.ResetsAt = &t
			}
			u.Windows = append(u.Windows, uw)
		}
	}
	if len(u.Windows) == 0 {
		return Usage{}, ErrNoWindows
	}
	return u, nil
}

// Usage fetches GET wham/usage once with the home's access token (from
// auth.json or the OS keyring, as cli_auth_credentials_store says). It never
// refreshes: an expired token returns ErrExpired, an API-key login returns
// ErrBilledPerToken, 429 maps to ErrRateLimited (no retry). Network · Read
// (plus Process · Read via Host.Secrets for keyring homes): call once,
// never poll.
func (a *Agent) Usage(ctx context.Context, h Home) (Usage, error) {
	return a.usageAt(ctx, h, UsageEndpoint)
}

func (a *Agent) usageAt(ctx context.Context, h Home, endpoint string) (Usage, error) {
	tok, account, mode, apiKey, err := a.readAccess(ctx, h)
	if err != nil {
		return Usage{}, err
	}
	if tok.IsZero() {
		if apiKey || mode == AuthAPIKey {
			return Usage{}, fmt.Errorf("codex: API-key login: %w", agentio.ErrBilledPerToken)
		}
		return Usage{}, agentio.ErrNotLoggedIn
	}
	now := a.now()
	if c, ok := decodeJWT(tok.Reveal()); ok && c.Exp > 0 && !now.Before(unixTime(c.Exp)) {
		return Usage{}, fmt.Errorf("codex: access token expired at %s (run codex to refresh it): %w", unixTime(c.Exp).Format(time.RFC3339), agentio.ErrExpired)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Usage{}, fmt.Errorf("codex: build usage request: %w", err)
	}
	req.Header.Set(headerAuth, bearerPrefix+tok.Reveal())
	if account != "" {
		req.Header.Set(headerAccountID, account)
	}
	req.Header.Set(headerAccept, mimeJSON)
	req.Header.Set(headerUserAgent, userAgent)
	client := a.host.HTTP
	if client == nil {
		return Usage{}, agentio.UnsupportedError(ID, "usage", "Host.HTTP is nil")
	}
	resp, err := client.Do(req)
	if err != nil {
		return Usage{}, fmt.Errorf("codex: usage request: %w", err)
	}
	// Body is read fully (bounded) below; Close only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Usage{}, fmt.Errorf("codex: read usage response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Usage{}, fmt.Errorf("codex: HTTP %d: %w", resp.StatusCode, agentio.ErrUnauthorized)
	case resp.StatusCode == http.StatusTooManyRequests:
		return Usage{}, agentio.RateLimitError(resp.Header, now)
	case resp.StatusCode != http.StatusOK:
		return Usage{}, fmt.Errorf("codex: unexpected HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	u, err := ParseUsage(body)
	if err != nil {
		return Usage{}, err
	}
	u.ObservedAt = now
	return u, nil
}

// snippet trims an error body to one line of at most snippetMax bytes at a
// rune boundary.
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

// Window labels by length (Codex does not name its windows).
const (
	minutesPerHour = 60
	hoursPerDay    = 24
)

// windowLabel names a window by its length: "5h", "7d", "30d".
func windowLabel(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d < hoursPerDay*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dd", int(d/(hoursPerDay*time.Hour)))
}

// Neutral projects onto agentio.Usage (Length, Percent, ResetsAt,
// VendorKind; Raw = vendor body). Pure.
func (u Usage) Neutral() agentio.Usage {
	out := agentio.Usage{
		Agent:      ID,
		Billing:    agentio.BillingSubscription,
		ObservedAt: u.ObservedAt,
		Source:     u.Source,
		Raw:        u.Raw,
	}
	for _, w := range u.Windows {
		scope := ""
		if w.LimitID != "" && w.LimitID != limitIDCodex {
			scope = w.LimitID
		}
		out.Windows = append(out.Windows, agentio.Window{
			Length: w.Length, Scope: scope, Label: windowLabel(w.Length),
			Percent: w.Percent, ResetsAt: w.ResetsAt, VendorKind: w.Vendor,
		})
	}
	return out
}

// limitIDCodex is the default limit bucket in rollout snapshots.
const limitIDCodex = "codex"

// Subscription is the plan and subscription window of a ChatGPT login.
type Subscription struct {
	Plan        Plan
	ActiveStart *time.Time
	ActiveUntil *time.Time
	LastChecked *time.Time
	Source      agentio.UsageSource // Cache (id_token claims) or Server
}

// Subscription answers from the id_token claims on disk first; only when
// the plan claim is absent does it fall back to one usage fetch. Disk ·
// Read (keyring homes: Process · Read via Host.Secrets), then Network · Read.
func (a *Agent) Subscription(ctx context.Context, h Home) (Subscription, error) {
	id, err := a.IdentityContext(ctx, h)
	if err != nil {
		return Subscription{}, err
	}
	if id.Plan != "" {
		return Subscription{Plan: id.Plan, ActiveStart: id.SubscriptionActiveStart, ActiveUntil: id.SubscriptionActiveUntil,
			LastChecked: id.SubscriptionLastChecked, Source: agentio.UsageCache}, nil
	}
	u, err := a.Usage(ctx, h)
	if err != nil {
		return Subscription{}, err
	}
	return Subscription{Plan: u.Plan, Source: agentio.UsageServer}, nil
}

// Neutral projects onto agentio.Subscription. Pure.
func (s Subscription) Neutral() agentio.Subscription {
	return agentio.Subscription{Plan: s.Plan.DisplayName(), StartedAt: s.ActiveStart}
}
