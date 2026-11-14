package codex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Plan is chatgpt_plan_type (protocol/src/auth.rs KnownPlan). Unknown plans
// are kept verbatim.
type Plan string

// Plan values (raw_value spelling upstream).
const (
	PlanFree                        Plan = "free"
	PlanGo                          Plan = "go"
	PlanPlus                        Plan = "plus"
	PlanPro                         Plan = "pro"
	PlanProLite                     Plan = "prolite"
	PlanProMax                      Plan = "promax"
	PlanTeam                        Plan = "team"
	PlanSelfServeBusinessProLite    Plan = "self_serve_business_prolite"
	PlanSelfServeBusinessUsageBased Plan = "self_serve_business_usage_based"
	PlanBusiness                    Plan = "business"
	PlanEnt26                       Plan = "ent26"
	PlanEnterpriseCBPAutomation     Plan = "enterprise_cbp_automation"
	PlanEnterpriseCBPUsageBased     Plan = "enterprise_cbp_usage_based"
	PlanEnterprise                  Plan = "enterprise"
	PlanEdu                         Plan = "edu"
	PlanEduPlus                     Plan = "edu_plus"
	PlanEduPro                      Plan = "edu_pro"
)

// Plan aliases upstream normalises (from_raw_value).
const (
	planAliasHC        = "hc"
	planAliasEducation = "education"
)

// ParsePlan normalises a raw plan_type the way upstream from_raw_value
// does (lowercase, aliases hc → enterprise, education → edu). Pure.
func ParsePlan(raw string) Plan {
	p := Plan(strings.ToLower(raw))
	switch string(p) {
	case planAliasHC:
		return PlanEnterprise
	case planAliasEducation:
		return PlanEdu
	}
	if p.IsKnown() {
		return p
	}
	return Plan(raw)
}

// planDisplay is upstream KnownPlan::display_name.
func planDisplay(p Plan) (string, bool) {
	switch p {
	case PlanFree:
		return "Free", true
	case PlanGo:
		return "Go", true
	case PlanPlus:
		return "Plus", true
	case PlanPro:
		return "Pro (More)", true
	case PlanProLite:
		return "Pro", true
	case PlanProMax:
		return "Pro (Max)", true
	case PlanTeam:
		return "Team", true
	case PlanSelfServeBusinessProLite:
		return "Self Serve Business ProLite", true
	case PlanSelfServeBusinessUsageBased:
		return "Self Serve Business Usage Based", true
	case PlanBusiness:
		return "Business", true
	case PlanEnt26, PlanEnterprise:
		return "Enterprise", true
	case PlanEnterpriseCBPAutomation:
		return "Enterprise (Automation)", true
	case PlanEnterpriseCBPUsageBased:
		return "Enterprise CBP Usage Based", true
	case PlanEdu:
		return "Edu", true
	case PlanEduPlus:
		return "Edu Plus", true
	case PlanEduPro:
		return "Edu Pro", true
	}
	return "", false
}

// IsKnown reports a plan upstream names. Pure.
func (p Plan) IsKnown() bool {
	_, ok := planDisplay(p)
	return ok
}

// DisplayName is upstream's display name; an unknown plan shows verbatim. Pure.
func (p Plan) DisplayName() string {
	if d, ok := planDisplay(p); ok {
		return d
	}
	return string(p)
}

// IsWorkspace reports the team/business/enterprise/edu family
// (auth.rs is_workspace_account). Pure.
func (p Plan) IsWorkspace() bool {
	switch p {
	case PlanTeam, PlanSelfServeBusinessProLite, PlanSelfServeBusinessUsageBased, PlanBusiness,
		PlanEnt26, PlanEnterpriseCBPAutomation, PlanEnterpriseCBPUsageBased, PlanEnterprise,
		PlanEdu, PlanEduPlus, PlanEduPro:
		return true
	}
	return false
}

// Identity is what auth.json says about a home's login, decoded from the
// JWT claims without verifying or refreshing anything. Personal values
// redact in every output (OD-6).
type Identity struct {
	Mode      AuthMode
	LoggedIn  bool
	Email     agentio.Personal
	Name      agentio.Personal
	AccountID agentio.Personal // tokens.account_id / chatgpt_account_id
	UserID    agentio.Personal
	Plan      Plan

	SubscriptionActiveStart *time.Time
	SubscriptionActiveUntil *time.Time
	SubscriptionLastChecked *time.Time

	// Organizations: element shape unseen (empty list on the sample home).
	Organizations []agentio.Raw

	AccessExpiresAt  time.Time // access_token exp; zero when not a JWT
	IDTokenExpiresAt time.Time
	LastRefresh      time.Time
	// CredentialStore is where the login was found (file or keyring), or
	// the configured store (auto, keyring, ephemeral) when none was found.
	CredentialStore CredentialStore
	Source          string       // auth.json path when read from the file; "" for a keyring login
	Keyring         KeyringEntry // the keyring entry consulted (keyring and auto stores); zero for file
	HasAPIKey       bool         // OPENAI_API_KEY present (value never loaded)

	Unknown agentio.Unknown
}

// authFileWire is auth.json. The refresh token is never decoded; token
// strings live only for the duration of a claims decode or a usage request.
type authFileWire struct {
	AuthMode     AuthMode        `json:"auth_mode"`
	OpenAIAPIKey *string         `json:"OPENAI_API_KEY"`
	Tokens       *authTokensWire `json:"tokens"`
	LastRefresh  string          `json:"last_refresh"`
}

type authTokensWire struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id"`
}

// authKnownKeys are the auth.json keys Identity models; others → Unknown.
func authKnownKeys() []string {
	return []string{"auth_mode", "OPENAI_API_KEY", "tokens", "last_refresh"}
}

// JWT claim namespaces (login/src/token_data.rs).
const (
	claimAuthNS    = "https://api.openai.com/auth"
	claimProfileNS = "https://api.openai.com/profile"
	jwtSep         = "."
	jwtParts       = 3
)

type jwtClaims struct {
	Email   string            `json:"email"`
	Name    string            `json:"name"`
	Exp     int64             `json:"exp"`
	Auth    *jwtAuthClaims    `json:"https://api.openai.com/auth"`
	Profile *jwtProfileClaims `json:"https://api.openai.com/profile"`
}

type jwtProfileClaims struct {
	Email string `json:"email"`
}

type jwtAuthClaims struct {
	ChatGPTAccountID     string            `json:"chatgpt_account_id"`
	ChatGPTPlanType      string            `json:"chatgpt_plan_type"`
	ChatGPTUserID        string            `json:"chatgpt_user_id"`
	UserID               string            `json:"user_id"`
	SubscriptionStart    json.RawMessage   `json:"chatgpt_subscription_active_start"`
	SubscriptionUntil    json.RawMessage   `json:"chatgpt_subscription_active_until"`
	SubscriptionChecked  json.RawMessage   `json:"chatgpt_subscription_last_checked"`
	Organizations        []json.RawMessage `json:"organizations"`
	ChatGPTAccountUserID string            `json:"chatgpt_account_user_id"`
}

// decodeJWT reads a JWT payload WITHOUT verifying it: only to show the
// login's own claims and expiry. Never printed or stored.
func decodeJWT(token string) (jwtClaims, bool) {
	parts := strings.Split(token, jwtSep)
	if len(parts) != jwtParts || parts[1] == "" {
		return jwtClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return jwtClaims{}, false
	}
	var c jwtClaims
	if json.Unmarshal(payload, &c) != nil {
		return jwtClaims{}, false
	}
	return c, true
}

// claimTime accepts an RFC 3339 string or Unix seconds (the claim's type
// was not recorded in the sample); nil when absent or unparsable.
func claimTime(raw json.RawMessage) *time.Time {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			t = t.UTC()
			return &t
		}
		return nil
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		t := time.Unix(int64(n), 0).UTC()
		return &t
	}
	return nil
}

func unixTime(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

// ParseAuth decodes auth.json bytes into an Identity. Pure.
func ParseAuth(b []byte, source string) (Identity, error) {
	var w authFileWire
	if err := json.Unmarshal(b, &w); err != nil {
		return Identity{}, fmt.Errorf("codex: decode %s: %w", fileAuth, err)
	}
	id := Identity{Mode: w.AuthMode, CredentialStore: StoreFile, Source: source}
	id.Unknown, _ = agentio.ExtractUnknown(b, authKnownKeys())
	id.HasAPIKey = w.OpenAIAPIKey != nil && *w.OpenAIAPIKey != ""
	if t, err := time.Parse(time.RFC3339Nano, w.LastRefresh); err == nil {
		id.LastRefresh = t.UTC()
	}
	if w.Tokens != nil {
		id.AccountID = agentio.NewPersonal(w.Tokens.AccountID)
		if c, ok := decodeJWT(w.Tokens.AccessToken); ok {
			id.AccessExpiresAt = unixTime(c.Exp)
		}
		if c, ok := decodeJWT(w.Tokens.IDToken); ok {
			id.IDTokenExpiresAt = unixTime(c.Exp)
			applyIDClaims(&id, c)
		}
	}
	id.LoggedIn = (w.Tokens != nil && w.Tokens.AccessToken != "") || id.HasAPIKey
	if id.Mode == AuthNone && id.HasAPIKey && (w.Tokens == nil || w.Tokens.AccessToken == "") {
		id.Mode = AuthAPIKey
	}
	return id, nil
}

func applyIDClaims(id *Identity, c jwtClaims) {
	email := c.Email
	if email == "" && c.Profile != nil {
		email = c.Profile.Email
	}
	id.Email = agentio.NewPersonal(email)
	id.Name = agentio.NewPersonal(c.Name)
	if c.Auth == nil {
		return
	}
	a := c.Auth
	if id.AccountID.IsZero() {
		id.AccountID = agentio.NewPersonal(a.ChatGPTAccountID)
	}
	uid := a.ChatGPTUserID
	if uid == "" {
		uid = a.UserID
	}
	id.UserID = agentio.NewPersonal(uid)
	if a.ChatGPTPlanType != "" {
		id.Plan = ParsePlan(a.ChatGPTPlanType)
	}
	id.SubscriptionActiveStart = claimTime(a.SubscriptionStart)
	id.SubscriptionActiveUntil = claimTime(a.SubscriptionUntil)
	id.SubscriptionLastChecked = claimTime(a.SubscriptionChecked)
	for _, o := range a.Organizations {
		id.Organizations = append(id.Organizations, agentio.Raw(o))
	}
}

// KeyringService is the OS keyring service Codex saves CLI logins under
// when cli_auth_credentials_store is keyring or auto (upstream
// login/src/auth/storage.rs:243, KEYRING_SERVICE).
const KeyringService = "Codex Auth"

// Keyring account derivation (storage.rs:246-257 compute_store_key): the
// first 16 hex digits of sha256(canonical CODEX_HOME path), prefixed.
const (
	keyringAccountPrefix = "cli|"
	keyringAccountHexLen = 16
)

// KeyringEntry names one keyring item (service and account), never its
// value. The account is a truncated hash of the home path, not personal.
type KeyringEntry struct {
	Service string
	Account string
}

// String is "<service>/<account>" for messages. Pure.
func (e KeyringEntry) String() string { return e.Service + "/" + e.Account }

// KeyringAccount is the keyring account Codex derives for a home dir:
// "cli|" + the first 16 hex digits of sha256 of the canonical path. Like
// upstream (Path::canonicalize, falling back to the path as given when it
// cannot be resolved) symlinks are resolved first, so /var and
// /private/var name the same account. Disk · Read (symlink resolution).
func KeyringAccount(dir string) string {
	canonical := dir
	if real, err := filepath.EvalSymlinks(dir); err == nil && filepath.IsAbs(real) {
		canonical = real
	}
	sum := sha256.Sum256([]byte(canonical))
	return keyringAccountPrefix + hex.EncodeToString(sum[:])[:keyringAccountHexLen]
}

// KeyringEntry is the keyring item Codex would read for h. Disk · Read.
func (a *Agent) KeyringEntry(h Home) KeyringEntry {
	return KeyringEntry{Service: KeyringService, Account: KeyringAccount(h.Dir())}
}

// ErrKeyringValue means the keyring item exists but its value is not the
// auth.json JSON Codex stores there. The value itself is never quoted.
var ErrKeyringValue = errors.New("value is not Codex auth JSON")

// KeyringError is a failed keyring read for one entry (lookup failure,
// timeout, or ErrKeyringValue). It names the entry, never the value.
type KeyringError struct {
	Entry KeyringEntry
	Err   error
}

// Error names the entry and the cause. Pure.
func (e *KeyringError) Error() string {
	return fmt.Sprintf("codex: keyring %s: %v", e.Entry, e.Err)
}

// Unwrap returns the cause. Pure.
func (e *KeyringError) Unwrap() error { return e.Err }

// storedAuth is one login as Codex stores it: auth.json bytes or the
// keyring value (the same JSON, storage.rs:315-319). raw holds secrets and
// never leaves this file except through ParseAuth / decodeAccess.
type storedAuth struct {
	raw   []byte
	found bool
	// store is where the login was found, or the configured store when
	// it was found nowhere.
	store   CredentialStore
	source  string       // auth.json path when read from the file
	keyring KeyringEntry // the entry consulted (keyring and auto stores)
}

// credentialStore is cli_auth_credentials_store from config.toml; absent
// or unreadable config means file, upstream's default (config/src/types.rs:136-146).
func (a *Agent) credentialStore(h Home) CredentialStore {
	c, _, err := readConfigPath(a.Paths(h).Config)
	if err != nil || !c.CLIAuthCredentialsStore.Set || !c.CLIAuthCredentialsStore.V.IsKnown() {
		return StoreFile
	}
	return c.CLIAuthCredentialsStore.V
}

// loadAuth reads the login the way upstream create_auth_storage
// (storage.rs:586-614) picks a backend: file → auth.json only; keyring →
// the keyring only (DirectKeyringAuthStorage, storage.rs:309-313); auto →
// the keyring, then auth.json when the keyring has no entry or fails
// (AutoAuthStorage::load, storage.rs:478-505); ephemeral → process memory
// of the running codex, unreadable from outside (ErrUnsupported).
// Disk · Read, plus Process · Read via Host.Secrets for keyring and auto.
func (a *Agent) loadAuth(ctx context.Context, h Home) (storedAuth, error) {
	mode := a.credentialStore(h)
	switch mode {
	case StoreKeyring:
		return a.loadKeyring(ctx, h)
	case StoreAuto:
		k, kerr := a.loadKeyring(ctx, h)
		if kerr == nil && k.found {
			return k, nil
		}
		f, ferr := a.loadFile(h)
		if ferr != nil {
			return f, ferr
		}
		f.keyring = k.keyring
		if !f.found {
			// Upstream only warns about a failed keyring read before using
			// the file; with no file either the failure is the answer, so
			// a keychain prompt that timed out is not reported as logged out.
			f.store = StoreAuto
			return f, kerr
		}
		return f, nil
	case StoreEphemeral:
		return storedAuth{store: StoreEphemeral},
			agentio.UnsupportedError(ID, "ephemeral login", "cli_auth_credentials_store = \"ephemeral\" keeps the login in the running codex process's memory only (storage.rs:543-584)")
	}
	return a.loadFile(h)
}

// loadFile reads <home>/auth.json. Disk · Read.
func (a *Agent) loadFile(h Home) (storedAuth, error) {
	p := a.Paths(h).Auth
	out := storedAuth{store: StoreFile, source: p}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("codex: read %s: %w", p, err)
	}
	out.raw, out.found = b, true
	return out, nil
}

// loadKeyring reads the home's keyring item through Host.Secrets with the
// exact account Codex uses (never a service-only lookup: every home has
// its own account under the same service). A nil Secrets store reads as
// no entry, like Claude's keychain lookup. Process · Read via Host.Secrets.
func (a *Agent) loadKeyring(ctx context.Context, h Home) (storedAuth, error) {
	e := a.KeyringEntry(h)
	out := storedAuth{store: StoreKeyring, keyring: e}
	if a.host.Secrets == nil {
		return out, nil
	}
	tok, ok, err := a.host.Secrets.Get(ctx, e.Service, e.Account)
	if err != nil {
		return out, &KeyringError{Entry: e, Err: err}
	}
	if !ok || tok.IsZero() {
		return out, nil
	}
	raw := keyringBytes(tok.Reveal())
	if !json.Valid(raw) {
		return out, &KeyringError{Entry: e, Err: ErrKeyringValue}
	}
	out.raw, out.found = raw, true
	return out, nil
}

// keyringBytes undoes `security find-generic-password -w`'s hex output:
// macOS prints a password as bare hex when it holds bytes it will not
// print as text, so a value that is not JSON but decodes as hex is
// decoded. Pure.
func keyringBytes(v string) []byte {
	if strings.HasPrefix(v, "{") {
		return []byte(v)
	}
	if b, err := hex.DecodeString(v); err == nil {
		return b
	}
	return []byte(v)
}

// Identity reads h's login from where cli_auth_credentials_store says
// Codex keeps it (auth.json, the OS keyring, or both for auto) and decodes
// its claims locally: never verified, never refreshed, never a network
// call. No stored login is a logged-out Identity with a nil error; an
// ephemeral store returns ErrUnsupported; a failed keyring read returns a
// *KeyringError. Disk · Read, plus Process · Read via Host.Secrets
// (MayTouchLogin false) for keyring and auto homes.
func (a *Agent) Identity(h Home) (Identity, error) {
	return a.IdentityContext(context.Background(), h)
}

// IdentityContext is Identity with a context for the keyring lookup.
// Disk · Read / Process · Read via Host.Secrets.
func (a *Agent) IdentityContext(ctx context.Context, h Home) (Identity, error) {
	s, err := a.loadAuth(ctx, h)
	if err != nil {
		return Identity{CredentialStore: s.store, Source: s.source, Keyring: s.keyring}, err
	}
	if !s.found {
		return Identity{CredentialStore: s.store, Source: s.source, Keyring: s.keyring}, nil
	}
	id, err := ParseAuth(s.raw, s.source)
	if err != nil && s.store == StoreKeyring {
		// The decode error may quote bytes of the value; replace it.
		return Identity{CredentialStore: s.store, Keyring: s.keyring}, &KeyringError{Entry: s.keyring, Err: ErrKeyringValue}
	}
	if err != nil {
		return Identity{}, err
	}
	id.CredentialStore, id.Keyring = s.store, s.keyring
	return id, nil
}

// Neutral projects onto agentio.Identity: plan display name, PerToken for
// API-key logins, file credential, access-token expiry. Pure.
func (i Identity) Neutral() agentio.Identity {
	out := agentio.Identity{
		LoggedIn:   i.LoggedIn,
		Email:      i.Email,
		PlanLabel:  i.Plan.DisplayName(),
		Billing:    agentio.BillingUnknown,
		Credential: agentio.CredentialNone,
	}
	switch {
	case i.Mode == AuthAPIKey:
		out.Billing = agentio.BillingPerToken
	case i.Mode == AuthChatGPT || i.Plan != "":
		out.Billing = agentio.BillingSubscription
	}
	if i.LoggedIn {
		out.Credential = agentio.CredentialFile
		if i.CredentialStore == StoreKeyring {
			out.Credential = agentio.CredentialKeyring
		}
	}
	if !i.AccessExpiresAt.IsZero() {
		t := i.AccessExpiresAt
		out.ExpiresAt = &t
	}
	return out
}

// readAccess returns the bearer and account id for a usage request from
// wherever the home keeps its login (loadAuth). Package-private: the token
// never leaves this package except on the request header of a GET the
// caller asked for. No stored login is ErrNotLoggedIn. Disk · Read /
// Process · Read via Host.Secrets.
func (a *Agent) readAccess(ctx context.Context, h Home) (agentio.Token, string, AuthMode, bool, error) {
	s, err := a.loadAuth(ctx, h)
	if err != nil {
		return agentio.Token{}, "", AuthNone, false, err
	}
	if !s.found {
		return agentio.Token{}, "", AuthNone, false, agentio.ErrNotLoggedIn
	}
	var w authFileWire
	if err := json.Unmarshal(s.raw, &w); err != nil {
		if s.store == StoreKeyring {
			return agentio.Token{}, "", AuthNone, false, &KeyringError{Entry: s.keyring, Err: ErrKeyringValue}
		}
		return agentio.Token{}, "", AuthNone, false, fmt.Errorf("codex: decode %s: %w", fileAuth, err)
	}
	apiKey := w.OpenAIAPIKey != nil && *w.OpenAIAPIKey != ""
	if w.Tokens == nil || w.Tokens.AccessToken == "" {
		return agentio.Token{}, "", w.AuthMode, apiKey, nil
	}
	return agentio.NewToken(w.Tokens.AccessToken), w.Tokens.AccountID, w.AuthMode, apiKey, nil
}

// login flags (`codex login --help` at 0.162.1).
const (
	subLogin            = "login"
	subLogout           = "logout"
	flagDeviceAuth      = "--device-auth"
	flagWithAPIKey      = "--with-api-key"
	flagWithAccessToken = "--with-access-token"
)

// LoginCommand builds `codex login [--device-auth|--with-api-key|
// --with-access-token]` for h. Secrets are read by Codex from stdin, never
// argv. An unknown flow returns ErrUnsupported. Pure.
func (a *Agent) LoginCommand(h Home, f agentio.LoginFlow) (agentio.Command, error) {
	args := []string{subLogin}
	switch f {
	case agentio.LoginBrowser:
	case agentio.LoginDevice:
		args = append(args, flagDeviceAuth)
	case agentio.LoginAPIKeyStdin:
		args = append(args, flagWithAPIKey)
	case agentio.LoginAccessTokenStdin:
		args = append(args, flagWithAccessToken)
	default:
		return agentio.Command{}, agentio.UnsupportedError(ID, "login flow "+f.String(), "codex login has no such flow")
	}
	return a.plainCommand(h, args), nil
}

// LogoutCommand builds `codex logout` for h. Pure.
func (a *Agent) LogoutCommand(h Home) agentio.Command {
	return a.plainCommand(h, []string{subLogout})
}
