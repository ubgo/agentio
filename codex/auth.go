package codex

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/khanakia/agentcli"
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
	Email     agentcli.Personal
	Name      agentcli.Personal
	AccountID agentcli.Personal // tokens.account_id / chatgpt_account_id
	UserID    agentcli.Personal
	Plan      Plan

	SubscriptionActiveStart *time.Time
	SubscriptionActiveUntil *time.Time
	SubscriptionLastChecked *time.Time

	// Organizations: element shape unseen (empty list on the sample home).
	Organizations []agentcli.Raw

	AccessExpiresAt  time.Time // access_token exp; zero when not a JWT
	IDTokenExpiresAt time.Time
	LastRefresh      time.Time
	CredentialStore  CredentialStore // file (auth.json) | keyring
	Source           string          // auth.json path
	HasAPIKey        bool            // OPENAI_API_KEY present (value never loaded)

	Unknown agentcli.Unknown
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
	id.Unknown, _ = agentcli.ExtractUnknown(b, authKnownKeys())
	id.HasAPIKey = w.OpenAIAPIKey != nil && *w.OpenAIAPIKey != ""
	if t, err := time.Parse(time.RFC3339Nano, w.LastRefresh); err == nil {
		id.LastRefresh = t.UTC()
	}
	if w.Tokens != nil {
		id.AccountID = agentcli.NewPersonal(w.Tokens.AccountID)
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
	id.Email = agentcli.NewPersonal(email)
	id.Name = agentcli.NewPersonal(c.Name)
	if c.Auth == nil {
		return
	}
	a := c.Auth
	if id.AccountID.IsZero() {
		id.AccountID = agentcli.NewPersonal(a.ChatGPTAccountID)
	}
	uid := a.ChatGPTUserID
	if uid == "" {
		uid = a.UserID
	}
	id.UserID = agentcli.NewPersonal(uid)
	if a.ChatGPTPlanType != "" {
		id.Plan = ParsePlan(a.ChatGPTPlanType)
	}
	id.SubscriptionActiveStart = claimTime(a.SubscriptionStart)
	id.SubscriptionActiveUntil = claimTime(a.SubscriptionUntil)
	id.SubscriptionLastChecked = claimTime(a.SubscriptionChecked)
	for _, o := range a.Organizations {
		id.Organizations = append(id.Organizations, agentcli.Raw(o))
	}
}

// Identity reads <home>/auth.json and decodes its claims locally: never
// verified, never refreshed, never a network call. A home with no
// auth.json whose config stores credentials in the OS keyring returns
// ErrUnsupported (OD-12: keyring service names are not modelled yet);
// otherwise a missing file is a logged-out Identity. Disk · Read.
func (a *Agent) Identity(h Home) (Identity, error) {
	p := a.Paths(h)
	b, err := os.ReadFile(p.Auth)
	if errors.Is(err, fs.ErrNotExist) {
		c, _, cerr := readConfigPath(p.Config)
		if cerr == nil && c.CLIAuthCredentialsStore.Set {
			switch c.CLIAuthCredentialsStore.V {
			case StoreKeyring, StoreAuto:
				return Identity{CredentialStore: c.CLIAuthCredentialsStore.V},
					agentcli.UnsupportedError(ID, "keyring identity", "Codex stores this login in the OS keyring; the keyring service names are not modelled yet (OD-12)")
			}
		}
		return Identity{Source: p.Auth, CredentialStore: StoreFile}, nil
	}
	if err != nil {
		return Identity{}, fmt.Errorf("codex: read %s: %w", p.Auth, err)
	}
	return ParseAuth(b, p.Auth)
}

// Neutral projects onto agentcli.Identity: plan display name, PerToken for
// API-key logins, file credential, access-token expiry. Pure.
func (i Identity) Neutral() agentcli.Identity {
	out := agentcli.Identity{
		LoggedIn:   i.LoggedIn,
		Email:      i.Email,
		PlanLabel:  i.Plan.DisplayName(),
		Billing:    agentcli.BillingUnknown,
		Credential: agentcli.CredentialNone,
	}
	switch {
	case i.Mode == AuthAPIKey:
		out.Billing = agentcli.BillingPerToken
	case i.Mode == AuthChatGPT || i.Plan != "":
		out.Billing = agentcli.BillingSubscription
	}
	if i.LoggedIn {
		out.Credential = agentcli.CredentialFile
		if i.CredentialStore == StoreKeyring {
			out.Credential = agentcli.CredentialKeyring
		}
	}
	if !i.AccessExpiresAt.IsZero() {
		t := i.AccessExpiresAt
		out.ExpiresAt = &t
	}
	return out
}

// accessToken returns the bearer and account id for a usage request.
// Package-private: the token never leaves this package except on the
// request header of a GET the caller asked for.
func readAccess(path string) (agentcli.Token, string, AuthMode, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return agentcli.Token{}, "", AuthNone, false, agentcli.ErrNotLoggedIn
	}
	if err != nil {
		return agentcli.Token{}, "", AuthNone, false, fmt.Errorf("codex: read %s: %w", path, err)
	}
	var w authFileWire
	if err := json.Unmarshal(b, &w); err != nil {
		return agentcli.Token{}, "", AuthNone, false, fmt.Errorf("codex: decode %s: %w", fileAuth, err)
	}
	apiKey := w.OpenAIAPIKey != nil && *w.OpenAIAPIKey != ""
	if w.Tokens == nil || w.Tokens.AccessToken == "" {
		return agentcli.Token{}, "", w.AuthMode, apiKey, nil
	}
	return agentcli.NewToken(w.Tokens.AccessToken), w.Tokens.AccountID, w.AuthMode, apiKey, nil
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
func (a *Agent) LoginCommand(h Home, f agentcli.LoginFlow) (agentcli.Command, error) {
	args := []string{subLogin}
	switch f {
	case agentcli.LoginBrowser:
	case agentcli.LoginDevice:
		args = append(args, flagDeviceAuth)
	case agentcli.LoginAPIKeyStdin:
		args = append(args, flagWithAPIKey)
	case agentcli.LoginAccessTokenStdin:
		args = append(args, flagWithAccessToken)
	default:
		return agentcli.Command{}, agentcli.UnsupportedError(ID, "login flow "+f.String(), "codex login has no such flow")
	}
	return a.plainCommand(h, args), nil
}

// LogoutCommand builds `codex logout` for h. Pure.
func (a *Agent) LogoutCommand(h Home) agentcli.Command {
	return a.plainCommand(h, []string{subLogout})
}
