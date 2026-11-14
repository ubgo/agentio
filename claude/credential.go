package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/khanakia/agentcli"
)

// CredentialSource is where a login was read from.
type CredentialSource string

// IsKnown reports a recorded source. cost: Pure.
func (s CredentialSource) IsKnown() bool { return s == SourceKeychain || s == SourceFile }

// Credential is the OAuth login Claude Code stored for one home. It never
// holds the refresh token; the access token is an agentcli.Token revealed
// only to this module's usage client.
type Credential struct {
	Source           CredentialSource
	Location         string // keychain service name or file path
	AccessToken      agentcli.Token
	ExpiresAt        *time.Time
	Scopes           []string
	SubscriptionType string
	RateLimitTier    string
}

// Expired reports a known expiry at or before now; unknown expiry is never
// expired (the server's 401 decides). cost: Pure.
func (c Credential) Expired(now time.Time) bool {
	return c.ExpiresAt != nil && !now.Before(*c.ExpiresAt)
}

// KeychainServices returns the keychain service names that may hold dir's
// login, most specific first: the hashed name, then for the default home
// the bare legacy name. dir must be the exact CLAUDE_CONFIG_DIR string
// Claude Code saw (the hash is over it). cost: Pure.
func KeychainServices(dir string, isDefault bool) []string {
	sum := sha256.Sum256([]byte(filepath.Clean(dir)))
	names := []string{keychainService + "-" + hex.EncodeToString(sum[:])[:keychainHashLen]}
	if isDefault {
		names = append(names, keychainService)
	}
	return names
}

// wireCredentials is the JSON Claude Code stores in .credentials.json and
// the keychain. The refresh token and mcpOAuth are deliberately not decoded.
type wireCredentials struct {
	ClaudeAIOAuth *struct {
		AccessToken      string   `json:"accessToken"`
		ExpiresAt        millis   `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		SubscriptionType string   `json:"subscriptionType"`
		RateLimitTier    string   `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
}

// parseCredential decodes one stored blob; errors never carry the value.
func parseCredential(raw []byte, src CredentialSource, loc string) (Credential, error) {
	var w wireCredentials
	if err := json.Unmarshal(raw, &w); err != nil {
		return Credential{}, fmt.Errorf("claude: %s %s: not a credential document", src, loc)
	}
	if w.ClaudeAIOAuth == nil || w.ClaudeAIOAuth.AccessToken == "" {
		return Credential{}, fmt.Errorf("claude: %s %s: no claudeAiOauth.accessToken", src, loc)
	}
	o := w.ClaudeAIOAuth
	return Credential{
		Source: src, Location: loc, AccessToken: agentcli.NewToken(o.AccessToken), ExpiresAt: o.ExpiresAt.timePtr(),
		Scopes: o.Scopes, SubscriptionType: o.SubscriptionType, RateLimitTier: o.RateLimitTier,
	}, nil
}

// keychainAccount is the account Claude Code saves its login under: the OS
// login, from Host.User (never os/user).
func (a *Agent) keychainAccount() string { return a.host.User }

// keychainGet looks one service up naming the account first and only then
// any entry with the service: stale same-service entries with another
// account ("unknown", empty token) exist (invariant 3).
func (a *Agent) keychainGet(ctx context.Context, service string) (agentcli.Token, bool, error) {
	if a.host.Secrets == nil {
		return agentcli.Token{}, false, nil
	}
	if acct := a.keychainAccount(); acct != "" {
		t, ok, err := a.host.Secrets.Get(ctx, service, acct)
		if err != nil || ok {
			return t, ok, err
		}
	}
	return a.host.Secrets.Get(ctx, service, "")
}

// Credential finds the freshest login for h across .credentials.json and
// the keychain (the latest ExpiresAt wins; a refreshed token may sit in one
// store while the other keeps an old copy). cost: Process · Read via
// Host.Secrets (MayTouchLogin false) or Disk; never refreshes.
func (a *Agent) Credential(h Home) (Credential, error) {
	return a.CredentialContext(context.Background(), h)
}

// CredentialContext is Credential with a context for the keychain lookup.
// cost: Process · Read (keychain) / Disk · Read.
func (a *Agent) CredentialContext(ctx context.Context, h Home) (Credential, error) {
	var best Credential
	found := false
	var reasons []error
	consider := func(c Credential) {
		if !found || expiresAfter(c, best) {
			best, found = c, true
		}
	}
	path := filepath.Join(h.Dir(), credentialsFile)
	if raw, ok, err := readFileBounded(path, maxResponseBytes); err != nil {
		reasons = append(reasons, fmt.Errorf("claude: read %s: %w", path, err))
	} else if ok {
		if c, perr := parseCredential(raw, SourceFile, path); perr == nil {
			consider(c)
		} else {
			reasons = append(reasons, perr)
		}
	}
	for _, svc := range KeychainServices(h.Dir(), h.IsDefault()) {
		tok, ok, err := a.keychainGet(ctx, svc)
		if err != nil {
			reasons = append(reasons, fmt.Errorf("claude: keychain %q: %w", svc, err))
			continue
		}
		if !ok {
			continue
		}
		if c, perr := parseCredential([]byte(tok.Reveal()), SourceKeychain, svc); perr == nil {
			consider(c)
		} else {
			reasons = append(reasons, perr)
		}
	}
	if !found {
		return Credential{}, errors.Join(append([]error{agentcli.ErrNotLoggedIn}, reasons...)...)
	}
	return best, nil
}

// expiresAfter ranks a known expiry over an unknown one, then later first.
func expiresAfter(c, best Credential) bool {
	switch {
	case c.ExpiresAt == nil:
		return false
	case best.ExpiresAt == nil:
		return true
	}
	return c.ExpiresAt.After(*best.ExpiresAt)
}

// Identity is who is logged into h, from .claude.json oauthAccount plus the
// credential's expiry and plan. Never a network call, never runs claude.
type Identity struct {
	agentcli.Identity
	Account    *OAuthAccount
	Credential *CredentialSummary
}

// CredentialSummary describes the credential without its token.
type CredentialSummary struct {
	Source           CredentialSource
	Location         string
	ExpiresAt        *time.Time
	Scopes           []string
	SubscriptionType string
	RateLimitTier    string
}

// Neutral returns the shared identity. cost: Pure.
func (i Identity) Neutral() agentcli.Identity { return i.Identity }

// Identity reads h's account and login. LoggedIn needs a credential; a home
// with only an account record is reported logged out. cost: Disk · Read +
// keychain (Process · Read via Host.Secrets, MayTouchLogin false).
func (a *Agent) Identity(h Home) (Identity, error) {
	g, err := a.GlobalConfig(h)
	if err != nil {
		return Identity{}, err
	}
	id := Identity{Account: g.OAuthAccount, Identity: agentcli.Identity{Billing: agentcli.BillingUnknown}}
	if acct := g.OAuthAccount; acct != nil {
		id.Email, id.Org = acct.EmailAddress, acct.OrganizationName
		tier := ""
		if acct.OrganizationRateLimitTier != nil {
			tier = *acct.OrganizationRateLimitTier
		}
		id.PlanLabel = PlanLabel(string(acct.OrganizationType), tier)
		id.Billing = agentcli.BillingSubscription
	}
	c, cerr := a.Credential(h)
	if cerr != nil {
		if errors.Is(cerr, agentcli.ErrNotLoggedIn) {
			return id, nil
		}
		return id, cerr
	}
	id.LoggedIn, id.ExpiresAt = true, c.ExpiresAt
	id.Identity.Credential = agentcli.CredentialFile
	if c.Source == SourceKeychain {
		id.Identity.Credential = agentcli.CredentialKeychain
	}
	if id.PlanLabel == "" {
		id.PlanLabel = PlanLabel(c.SubscriptionType, c.RateLimitTier)
	}
	if c.SubscriptionType != "" {
		id.Billing = agentcli.BillingSubscription
	}
	id.Credential = &CredentialSummary{Source: c.Source, Location: c.Location, ExpiresAt: c.ExpiresAt, Scopes: c.Scopes, SubscriptionType: c.SubscriptionType, RateLimitTier: c.RateLimitTier}
	return id, nil
}

// Token prefixes Anthropic puts on its credentials (agx login.go).
const (
	oauthAccessPrefix  = "sk-ant-oat"
	oauthRefreshPrefix = "sk-ant-ort"
	apiKeyPrefix       = "sk-ant-api"
	adminKeyPrefix     = "sk-ant-admin"
)

// ErrUnusableToken means a value is an Anthropic credential no login can
// run Claude with (a refresh token, an Admin key).
var ErrUnusableToken = errors.New("claude: this token cannot run Claude")

// SetupTokenLifetime is how long a setup-token is believed valid
// (unverified, agx logins spec V3).
const SetupTokenLifetime = 365 * 24 * time.Hour

// TokenKinds lists the token logins Claude supports, OAuth token first so a
// plain login spends plan quota rather than an API key. cost: Pure.
func (a *Agent) TokenKinds() []agentcli.TokenKind {
	return []agentcli.TokenKind{agentcli.TokenOAuthToken, agentcli.TokenAPIKey}
}

// DetectTokenKind classifies a token by prefix; false for no known prefix.
// Refresh tokens and Admin keys are refused with ErrUnusableToken. cost: Pure.
func (a *Agent) DetectTokenKind(t agentcli.Token) (agentcli.TokenKind, bool, error) {
	v := strings.TrimSpace(t.Reveal())
	switch {
	case strings.HasPrefix(v, oauthAccessPrefix):
		return agentcli.TokenOAuthToken, true, nil
	case strings.HasPrefix(v, apiKeyPrefix):
		return agentcli.TokenAPIKey, true, nil
	case strings.HasPrefix(v, oauthRefreshPrefix):
		return agentcli.TokenKindUnknown, true, fmt.Errorf("%w: it is an OAuth refresh token; use the token `claude setup-token` prints", ErrUnusableToken)
	case strings.HasPrefix(v, adminKeyPrefix):
		return agentcli.TokenKindUnknown, true, fmt.Errorf("%w: it is an Admin API key", ErrUnusableToken)
	}
	return agentcli.TokenKindUnknown, false, nil
}

// TokenEnv maps a token login onto Claude's variables: the token's own
// variable set as a secret; the other credential variables are scrubbed by
// the core (invariant 9). cost: Pure.
func (a *Agent) TokenEnv(c agentcli.TokenCredential) ([]agentcli.EnvChange, error) {
	switch c.Kind {
	case agentcli.TokenOAuthToken:
		return []agentcli.EnvChange{agentcli.EnvSetSecret(OAuthTokenEnv, c.Token)}, nil
	case agentcli.TokenAPIKey:
		return []agentcli.EnvChange{agentcli.EnvSetSecret(APIKeyEnv, c.Token)}, nil
	}
	return nil, agentcli.UnsupportedError(ID, "token kind "+c.Kind.String(), "Claude takes an OAuth token or an API key")
}

// TokenLifetime is how long a token kind stays valid; 0 = until revoked.
// cost: Pure.
func (a *Agent) TokenLifetime(k agentcli.TokenKind) time.Duration {
	if k == agentcli.TokenOAuthToken {
		return SetupTokenLifetime
	}
	return 0
}

// Probe request constants (agx probe.go): the smallest valid conversation
// on the cheapest model, identifying as Claude Code (the API accepts a
// setup-token only for such requests).
const (
	probeModel             = "claude-haiku-5-5"
	probeMaxTokens         = 1
	probeSystem            = "You are Claude Code, Anthropic's official CLI for Claude."
	probeRole              = "user"
	probeUserText          = "hi"
	anthropicVersionHeader = "anthropic-version"
	anthropicVersion       = "2023-06-01"
	probeTimeout           = 30 * time.Second
)

type probeBody struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system"`
	Messages  []probeMessage `json:"messages"`
}

type probeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Prober is the Quota door: calls that spend model tokens of an account.
type Prober struct{ a *Agent }

// Prober returns the Quota door. cost: Pure.
func (a *Agent) Prober() *Prober { return &Prober{a: a} }

// TokenUsage reads a token login's usage from one minimal /v1/messages call
// and its rate-limit headers. An API key has no plan windows and is never
// probed (ErrBilledPerToken). A 429 carrying the unified headers is parsed
// as usage at its limit (invariant 7). cost: Quota · Read: spends a few
// tokens; never loop.
func (p *Prober) TokenUsage(ctx context.Context, c agentcli.TokenCredential) (Usage, error) {
	if c.Kind != agentcli.TokenOAuthToken {
		return Usage{Usage: agentcli.Usage{Agent: ID, Billing: agentcli.BillingPerToken}}, agentcli.ErrBilledPerToken
	}
	body, err := json.Marshal(probeBody{Model: probeModel, MaxTokens: probeMaxTokens, System: probeSystem, Messages: []probeMessage{{Role: probeRole, Content: probeUserText}}})
	if err != nil {
		return Usage{}, fmt.Errorf("claude: encode probe: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.a.baseURL+messagesPath, bytes.NewReader(body))
	if err != nil {
		return Usage{}, fmt.Errorf("claude: build probe: %w", err)
	}
	tok := c.Token.Reveal()
	req.Header.Set(headerAuthorization, bearerPrefix+tok)
	req.Header.Set(oauthBetaHeader, oauthBetaValue)
	req.Header.Set(anthropicVersionHeader, anthropicVersion)
	req.Header.Set(headerContentType, mimeJSON)
	req.Header.Set(headerUserAgent, userAgent)
	client := p.a.host.HTTP
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Usage{}, fmt.Errorf("claude: probe: %w", err)
	}
	// The body is read (bounded) below; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	rb, err := readAllBounded(resp.Body)
	if err != nil {
		return Usage{}, fmt.Errorf("claude: read probe response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Usage{}, fmt.Errorf("%w (HTTP %d%s)", agentcli.ErrUnauthorized, resp.StatusCode, apiMessage(rb, tok))
	case resp.StatusCode == http.StatusTooManyRequests:
		if u, perr := ParseProbeHeaders(resp.Header); perr == nil {
			return p.a.probed(u), nil
		}
		return Usage{}, agentcli.RateLimitError(resp.Header, p.a.now())
	case resp.StatusCode != http.StatusOK:
		return Usage{}, fmt.Errorf("claude: probe: unexpected HTTP %d: %s", resp.StatusCode, snippet([]byte(redactIn(string(rb), tok))))
	}
	u, err := ParseProbeHeaders(resp.Header)
	if err != nil {
		return Usage{}, err
	}
	return p.a.probed(u), nil
}

func (a *Agent) probed(u agentcli.Usage) Usage {
	u.ObservedAt = a.now()
	return Usage{Usage: u}
}
