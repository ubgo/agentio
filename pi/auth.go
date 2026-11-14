package pi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/ubgo/agentio"
)

// LoginType is an auth.json entry's "type" (auth-storage.js:188-210).
type LoginType string

// LoginType values.
const (
	// LoginAPIKey is {"type":"api_key","key"?,"env"?}; key may be
	// "!<command>", run once per process (PKG/docs/providers.md:71-84).
	LoginAPIKey LoginType = "api_key"
	// LoginOAuth is {"type":"oauth","access","refresh","expires",...}; pi
	// refreshes it itself and rewrites auth.json (pi-ai auth/resolve.js).
	LoginOAuth LoginType = "oauth"
)

// LoginTypeValues lists every type. Pure.
func LoginTypeValues() []LoginType { return []LoginType{LoginAPIKey, LoginOAuth} }

// Login is one provider entry of auth.json, described without its secret:
// no key, access or refresh value ever leaves the decoder. The OAuth field
// names are source-derived (no logged-in recording yet): pi-ai
// dist/auth/oauth/*.js at 1.5.2.
type Login struct {
	// Provider is the auth.json key ("anthropic", "openai-codex").
	Provider string
	Type     LoginType
	// FromCommand is an api_key whose key is "!<command>".
	FromCommand bool
	// EnvNames are the names of the entry's per-provider env overrides.
	EnvNames []string
	// ExpiresAt is an OAuth entry's access expiry; nil for api_key.
	ExpiresAt *time.Time
}

// loginWire is the part of an auth.json entry read here; secrets stay raw
// and are only inspected for the "!" command prefix.
type loginWire struct {
	Type    LoginType                  `json:"type"`
	Key     json.RawMessage            `json:"key"`
	Env     map[string]json.RawMessage `json:"env"`
	Expires millis                     `json:"expires"`
}

// commandKeyPrefix is the JSON form of a key that starts with "!".
const commandKeyPrefix = `"!`

// Logins lists auth.json entries, sorted by provider. auth.json is written
// as {} on every start, so an empty list is the normal fresh state.
// Disk · Read; never a token value.
func (a *Agent) Logins(h Home) ([]Login, error) {
	p := a.Paths(h).Auth
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil {
		return nil, fmt.Errorf("pi: read %s: %w", p, err)
	}
	if !found {
		return nil, nil
	}
	var raw map[string]loginWire
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("pi: decode %s: %w", p, err)
	}
	out := make([]Login, 0, len(raw))
	for provider, w := range raw {
		l := Login{Provider: provider, Type: w.Type, FromCommand: bytes.HasPrefix(bytes.TrimSpace(w.Key), []byte(commandKeyPrefix))}
		for n := range w.Env {
			l.EnvNames = append(l.EnvNames, n)
		}
		slices.Sort(l.EnvNames)
		if t := w.Expires.Time(); w.Type == LoginOAuth && !t.IsZero() {
			l.ExpiresAt = &t
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out, nil
}

// EnvLogins lists the provider key variables set in Host.Env (names only),
// in tokenEnv order: pi uses them when auth.json has no entry for that
// provider. Pure.
func (a *Agent) EnvLogins() []string {
	var out []string
	for _, e := range tokenEnv() {
		if v, ok := a.host.Getenv(e.Env); ok && v != "" && !slices.Contains(out, e.Env) {
			out = append(out, e.Env)
		}
	}
	return out
}

// Identity reports whether h can authenticate without a prompt: an
// auth.json entry (Credential file) or a provider key variable in the
// base env (Credential env). pi has no account email, plan or org on
// disk, so those stay empty; Billing is unknown because one home can hold
// subscription OAuth for one provider and a per-token key for another.
// Disk · Read.
func (a *Agent) Identity(h Home) (agentio.Identity, error) {
	logins, err := a.Logins(h)
	if err != nil {
		return agentio.Identity{}, err
	}
	switch {
	case len(logins) > 0:
		return agentio.Identity{LoggedIn: true, Credential: agentio.CredentialFile}, nil
	case len(a.EnvLogins()) > 0:
		return agentio.Identity{LoggedIn: true, Credential: agentio.CredentialEnv}, nil
	}
	return agentio.Identity{Credential: agentio.CredentialNone}, nil
}
