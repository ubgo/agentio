package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/ubgo/agentio"
)

// Variables a Vertex AI login reads besides GOOGLE_API_KEY (core
// contentGenerator); names only, never values.
const (
	cloudProjectEnv  = "GOOGLE_CLOUD_PROJECT"
	cloudLocationEnv = "GOOGLE_CLOUD_LOCATION"
)

// ErrUnknownProvider means a token credential names a provider Gemini has
// no variable for.
var ErrUnknownProvider = errors.New("gemini: unknown token provider")

// oauthCreds is the part of oauth_creds.json agentio reads (google-auth
// library Credentials written by core cacheCredentials; source-derived:
// field names from the code, no logged-in file recorded). The tokens are
// agentio.Token so they redact when printed; agentio never sends,
// refreshes or prints them.
type oauthCreds struct {
	AccessToken  agentio.Token `json:"access_token"`
	RefreshToken agentio.Token `json:"refresh_token"`
	// ExpiryDate is the access token's expiry in Unix milliseconds.
	ExpiryDate int64 `json:"expiry_date"`
}

// googleAccounts is google_accounts.json: {"active": "<email>"|null,
// "old": ["<email>", ...]} (core UserAccountManager; source-derived).
type googleAccounts struct {
	Active *string  `json:"active"`
	Old    []string `json:"old"`
}

// Identity is what the home says about its login, without the network.
type Identity struct {
	agentio.Identity
	// AuthType is security.auth.selectedType ("" = none chosen yet).
	AuthType AuthType
}

// Identity reads the login of h from disk and Host.Env names:
//   - oauth-personal (or no auth type with a cached Google login): logged
//     in when oauth_creds.json holds a token; the email comes from
//     google_accounts.json, the expiry from expiry_date. With
//     GEMINI_FORCE_ENCRYPTED_FILE_STORAGE the CLI keeps the login in the
//     keychain instead, which this does not read;
//   - gemini-api-key: logged in when GEMINI_API_KEY is in Host.Env (a key
//     saved in the keychain is not read); billed per token;
//   - vertex-ai: GOOGLE_API_KEY, or GOOGLE_CLOUD_PROJECT plus
//     GOOGLE_CLOUD_LOCATION, in Host.Env; billed per token;
//   - other types (compute ADC, gateway, cloud shell): not decidable from
//     disk, LoggedIn false.
//
// cost: Disk · Read.
func (a *Agent) Identity(h Home) (Identity, error) {
	user, err := ReadSettingsFile(agentio.ConfigUser, h.UserSettingsPath())
	if err != nil {
		return Identity{}, err
	}
	var id Identity
	if sec := user.Settings.Security; sec != nil && sec.Auth != nil {
		id.AuthType = sec.Auth.SelectedType
	}
	switch id.AuthType {
	case AuthGeminiKey:
		id.Billing = agentio.BillingPerToken
		if a.hasEnv(APIKeyEnv) {
			id.LoggedIn, id.Credential = true, agentio.CredentialEnv
		}
		return id, nil
	case AuthVertex:
		id.Billing = agentio.BillingPerToken
		if a.hasEnv(GoogleAPIKeyEnv) || (a.hasEnv(cloudProjectEnv) && a.hasEnv(cloudLocationEnv)) {
			id.LoggedIn, id.Credential = true, agentio.CredentialEnv
		}
		return id, nil
	case AuthGoogleLogin, "":
		return a.googleLogin(h, id)
	}
	return id, nil
}

// googleLogin fills id from the cached Google login. cost: Disk · Read.
func (a *Agent) googleLogin(h Home, id Identity) (Identity, error) {
	data, err := os.ReadFile(h.Path(oauthCredsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return id, nil
	}
	if err != nil {
		return id, fmt.Errorf("gemini: read login: %w", err)
	}
	var c oauthCreds
	if err := json.Unmarshal(agentio.StripBOM(data), &c); err != nil {
		return id, fmt.Errorf("gemini: decode %s: %w", h.Path(oauthCredsFile), err)
	}
	if c.AccessToken.IsZero() && c.RefreshToken.IsZero() {
		return id, nil
	}
	id.LoggedIn, id.Credential = true, agentio.CredentialFile
	if id.AuthType == "" {
		id.AuthType = AuthGoogleLogin
	}
	if c.ExpiryDate > 0 {
		t := time.UnixMilli(c.ExpiryDate).UTC()
		id.ExpiresAt = &t
	}
	if acc, err := readGoogleAccounts(h.Path(googleAccountsFile)); err == nil && acc.Active != nil {
		id.Email = agentio.NewPersonal(*acc.Active)
	}
	return id, nil
}

// readGoogleAccounts reads google_accounts.json. cost: Disk · Read.
func readGoogleAccounts(p string) (googleAccounts, error) {
	var acc googleAccounts
	data, err := os.ReadFile(p)
	if err != nil {
		return acc, err
	}
	err = json.Unmarshal(agentio.StripBOM(data), &acc)
	return acc, err
}

// hasEnv reports a non-empty variable in Host.Env (the name only).
func (a *Agent) hasEnv(name string) bool {
	v, ok := a.host.Getenv(name)
	return ok && v != ""
}

// TokenKinds lists the token logins Gemini takes: an API key. cost: Pure.
func (a *Agent) TokenKinds() []agentio.TokenKind { return []agentio.TokenKind{agentio.TokenAPIKey} }

// DetectTokenKind returns TokenAPIKey for any non-empty token: it is the
// only kind Gemini takes, so the value is never inspected (only the root
// env builder reads a token's value). cost: Pure.
func (a *Agent) DetectTokenKind(t agentio.Token) (agentio.TokenKind, bool) {
	if t.IsZero() {
		return agentio.TokenKindUnknown, false
	}
	return agentio.TokenAPIKey, true
}

// TokenEnv maps an API key onto GEMINI_API_KEY (provider ""), or
// GOOGLE_API_KEY for provider "vertex" (Vertex AI express mode). A token
// launch still needs the home's selectedType to match (or be unset): the
// setting wins over the variables. cost: Pure.
func (a *Agent) TokenEnv(c agentio.TokenCredential) ([]agentio.EnvChange, error) {
	if c.Kind != agentio.TokenAPIKey {
		return nil, agentio.UnsupportedError(ID, "token login "+c.Kind.String(), "Gemini CLI takes an API key only")
	}
	for _, e := range tokenEnv() {
		if e.Provider == c.Provider {
			return []agentio.EnvChange{agentio.EnvSetSecret(e.Env, c.Token)}, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, c.Provider)
}
