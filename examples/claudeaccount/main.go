// Command claudeaccount shows how agentio reads a Claude Code login and
// account without touching the real machine: the credential of each home
// (file store and keychain), token kind detection, the token environment,
// login commands, the identity, the OAuth account record and
// `claude auth status` through the Runner door with a fake process runner.
//
// It reads the checked-in 2.1.325 fixture home; the keychain, the process
// runner and the clock are in-memory fakes. No token value is ever printed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultFixture is the fixture user home, relative to the examples module.
const defaultFixture = "../claude/testdata/homes/2.1.325/userhome"

// fixtureUser is the OS login the fake keychain stores the work login under.
const fixtureUser = "tester"

// workHomeName is the second Claude home in the fixture (~/.claude-work).
const workHomeName = ".claude-work"

// workLoginLifetime is how long the fake keychain login of the work home
// stays valid after the fixture clock.
const workLoginLifetime = 6 * time.Hour

// fixtureNow is the fixed clock: before the fixture login expires.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "Claude fixture user home to read")
	flag.Parse()
	if err := run(context.Background(), os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "claudeaccount:", err)
		os.Exit(1)
	}
}

// secretKey addresses one entry of memSecrets.
type secretKey struct{ service, account string }

// memSecrets is an in-memory keychain: Get answers the exact (service,
// account) pair, and account "" any entry of the service, like
// `security find-generic-password -s`.
type memSecrets map[secretKey]agentio.Token

// Get is the agentio.SecretStore lookup. Pure.
func (m memSecrets) Get(_ context.Context, service, account string) (agentio.Token, bool, error) {
	if t, ok := m[secretKey{service, account}]; ok {
		return t, true, nil
	}
	if account == "" {
		for k, t := range m {
			if k.service == service {
				return t, true, nil
			}
		}
	}
	return agentio.Token{}, false, nil
}

// cannedExec is a process runner that answers known argv lines with fixed
// stdout and never starts a process.
type cannedExec map[string]string

// errUnexpectedCommand reports an argv line cannedExec has no answer for.
var errUnexpectedCommand = errors.New("unexpected command")

// Run returns the canned stdout for the command line. Pure.
func (c cannedExec) Run(_ context.Context, cmd agentio.Command, _ io.Reader) (agentio.ProcessResult, error) {
	out, ok := c[cmd.String()]
	if !ok {
		return agentio.ProcessResult{}, fmt.Errorf("%w: %s", errUnexpectedCommand, cmd.String())
	}
	return agentio.ProcessResult{Stdout: []byte(out)}, nil
}

// authStatusJSON is `claude auth status --json` in its recorded shape with
// placeholder values.
const authStatusJSON = `{"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "firstParty", "analyticsDisabled": false,
 "projectsDirectory": "/home/user/.claude/projects", "configDirectory": "/home/user/.claude",
 "loginExpiresAt": "2026-11-13T20:56:01Z", "email": "user@example.com", "orgId": "00000000-0000-4000-8000-000000000000", "orgName": "Example org",
 "subscriptionType": "max", "storedClaudeAiLogin": true, "planTier": "max_20x"}`

// profileJSON is an /api/oauth/profile body reduced to the organization
// fields ParseProfile reads.
const profileJSON = `{"organization": {"organization_type": "claude_max", "rate_limit_tier": "default_claude_max_20x",
 "billing_type": "stripe_subscription", "subscription_status": "active", "subscription_created_at": "2025-01-02T00:00:00Z"}}`

// newHost builds a Host over the fixture: no binaries on PATH, a fake
// keychain holding the work home's login, and the canned runner.
func newHost(fixture string) agentio.Host {
	return agentio.Host{
		UserHome: fixture,
		User:     fixtureUser,
		Env:      []string{"HOME=" + fixture, "PATH="},
		Now:      fixtureNow,
		Secrets:  memSecrets{},
		Exec:     cannedExec{"claude auth status --json": authStatusJSON},
	}
}

// workKeychainLogin is the JSON Claude Code stores in the keychain, with a
// placeholder token; it never leaves the fake store.
func workKeychainLogin(expires time.Time) agentio.Token {
	return agentio.NewToken(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"%s","expiresAt":%d,"scopes":["user:inference","user:profile"],"subscriptionType":"pro","rateLimitTier":"default_claude_pro"}}`, "sk-ant-"+"oat01-EXAMPLE-NOT-A-TOKEN", expires.UnixMilli()))
}

// run prints every account step for the fixture user home. The Host's
// secrets and process runner are in-memory fakes, so no keychain is opened
// and no claude binary runs; tokens print only in redacted form.
func run(ctx context.Context, w io.Writer, fixture string) error {
	// Homes are absolute paths; resolve the fixture once.
	abs, err := filepath.Abs(fixture)
	if err != nil {
		return err
	}
	host := newHost(abs)
	a := claude.New(host)
	def, work := a.DefaultHome(), a.HomeAt(workHomeName)
	// The work home keeps its login in the keychain under the hashed name.
	secrets := host.Secrets.(memSecrets)
	secrets[secretKey{claude.KeychainServices(work.Dir(), work.IsDefault())[0], fixtureUser}] = workKeychainLogin(fixtureNow().Add(workLoginLifetime))

	for _, step := range []func() error{
		func() error { return readCredential(ctx, w, a, def) },
		func() error { return readCredential(ctx, w, a, work) },
		func() error { return detectKinds(w, a) },
		func() error { return tokenEnv(w, a) },
		func() error { loginCommands(w, a, work); return nil },
		func() error { return readIdentity(w, a, def) },
		func() error { return readAccount(w, a, def) },
		func() error { return readAuthStatus(ctx, w, a, def) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// tilde shows a path under the user home as ~/…, so output carries no
// machine path.
func tilde(a *claude.Agent, p string) string {
	return strings.Replace(p, a.Host().UserHome, "~", 1)
}

// snippet:start credential
// readCredential prints where a home's login lives and when it expires. The
// access token itself is an agentio.Token: printing it shows [redacted].
func readCredential(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) error {
	c, err := a.CredentialContext(ctx, h)
	if errors.Is(err, agentio.ErrNotLoggedIn) {
		fmt.Fprintf(w, "%s: not logged in\n", h.Name())
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: login in %s (%s)\n", h.Name(), c.Source, location(a, c))
	fmt.Fprintf(w, "  expires %s, expired now: %t\n", c.ExpiresAt.Format(time.RFC3339), c.Expired(a.Host().Now()))
	fmt.Fprintf(w, "  plan %s / %s, %d scopes, token %v\n", c.SubscriptionType, c.RateLimitTier, len(c.Scopes), c.AccessToken)
	fmt.Fprintf(w, "  keychain names tried: %d\n", len(claude.KeychainServices(h.Dir(), h.IsDefault())))
	return nil
}

// location shows a file path under ~ and a keychain service name with its
// per-directory hash elided, so the output is the same on every machine.
func location(a *claude.Agent, c claude.Credential) string {
	if c.Source == claude.SourceFile {
		return tilde(a, c.Location)
	}
	if i := strings.LastIndex(c.Location, "-"); i > 0 {
		return c.Location[:i] + "-<hash>"
	}
	return c.Location
}

// snippet:end credential

// snippet:start kinds
// detectKinds classifies tokens by prefix. The samples are placeholders; a
// refresh token or Admin key is refused with ErrUnusableToken.
func detectKinds(w io.Writer, a *claude.Agent) error {
	fmt.Fprintf(w, "token logins: %v\n", a.TokenKinds())
	for _, sample := range []string{"sk-ant-oat01-EXAMPLE", "sk-ant-api03-EXAMPLE", "sk-ant-ort01-EXAMPLE", "sk-ant-admin01-EXAMPLE", "not-a-claude-token"} {
		kind, known, err := a.DetectTokenKind(agentio.NewToken(sample))
		switch {
		case errors.Is(err, claude.ErrUnusableToken):
			fmt.Fprintf(w, "  %-14s refused: %v\n", sample[:min(len(sample), 14)], err)
		case err != nil:
			return err
		case !known:
			fmt.Fprintf(w, "  %-14s unknown prefix\n", sample[:min(len(sample), 14)])
		default:
			fmt.Fprintf(w, "  %-14s %s, valid for %s\n", sample[:min(len(sample), 14)], kind, lifetime(a.TokenLifetime(kind)))
		}
	}
	return nil
}

// lifetime renders a token lifetime; 0 means until revoked.
func lifetime(d time.Duration) string {
	if d == 0 {
		return "until revoked"
	}
	return d.String()
}

// snippet:end kinds

// snippet:start tokenenv
// tokenEnv shows which variable carries each token kind. The value is a
// secret EnvChange: String() prints [redacted], never the token.
func tokenEnv(w io.Writer, a *claude.Agent) error {
	for _, kind := range a.TokenKinds() {
		changes, err := a.TokenEnv(agentio.TokenCredential{Kind: kind, Token: agentio.NewToken("sk-ant-EXAMPLE")})
		if err != nil {
			return err
		}
		for _, c := range changes {
			fmt.Fprintf(w, "token env for %s: %s\n", kind, c)
		}
	}
	return nil
}

// snippet:end tokenenv

// snippet:start login
// loginCommands builds (never runs) the login, logout and setup-token
// commands. A non-default home carries CLAUDE_CONFIG_DIR in Command.Env.
func loginCommands(w io.Writer, a *claude.Agent, work claude.Home) {
	for _, c := range []agentio.Command{
		a.LoginCommand(a.DefaultHome(), claude.LoginClaudeAI, ""),
		a.LoginCommand(work, claude.LoginConsole, "user@example.com"),
		a.LogoutCommand(work),
		a.SetupTokenCommand(a.DefaultHome()),
	} {
		_, homeSet := agentio.EnvLookup(c.Env, claude.ConfigDirEnv)
		fmt.Fprintf(w, "command: %-50s %s set: %t\n", c.String(), claude.ConfigDirEnv, homeSet)
	}
}

// snippet:end login

// snippet:start identity
// readIdentity prints who is logged into h. Email and Org are
// agentio.Personal: they print redacted unless revealed on purpose.
func readIdentity(w io.Writer, a *claude.Agent, h claude.Home) error {
	id, err := a.Identity(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "identity: logged in %t, plan %q, billing %s, credential %s\n", id.LoggedIn, id.PlanLabel, id.Billing, id.Neutral().Credential)
	fmt.Fprintf(w, "  email %v, org %v\n", id.Email, id.Org)
	if s := id.Credential; s != nil {
		fmt.Fprintf(w, "  summary: %s, expires %s\n", s.Source, s.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

// snippet:end identity

// snippet:start account
// readAccount reads the oauthAccount record of .claude.json and decodes a
// profile body; a body without an organization is ErrNoOrganization.
func readAccount(w io.Writer, a *claude.Agent, h claude.Home) error {
	acct, err := a.Account(h)
	if err != nil {
		return err
	}
	if acct == nil {
		fmt.Fprintln(w, "account: none recorded")
		return nil
	}
	tier := ""
	if acct.OrganizationRateLimitTier != nil {
		tier = *acct.OrganizationRateLimitTier
	}
	fmt.Fprintf(w, "account: org type %s, billing %s, plan %s\n", acct.OrganizationType, acct.BillingType, claude.PlanLabel(string(acct.OrganizationType), tier))
	sub, err := claude.ParseProfile([]byte(profileJSON))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "profile: plan %s, status %s, billing %s, since %s\n", sub.Plan, sub.Status, sub.BillingType, sub.StartedAt.Format(time.DateOnly))
	if _, err := claude.ParseProfile([]byte(`{}`)); errors.Is(err, claude.ErrNoOrganization) {
		fmt.Fprintln(w, "profile without organization:", err)
	}
	return nil
}

// snippet:end account

// snippet:start authstatus
// readAuthStatus asks the claude binary through the Runner door. The door
// runs `claude auth status --json` via Host.Exec, here a canned fake.
func readAuthStatus(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) error {
	st, err := a.Runner().AuthStatus(ctx, h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "auth status: logged in %t via %s (%s), plan tier %s, email %v\n", st.LoggedIn, st.AuthMethod, st.APIProvider, st.PlanTier, st.Email)
	again, err := claude.ParseAuthStatus([]byte(authStatusJSON))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed offline: subscription %s, unknown keys %d\n", again.SubscriptionType, len(again.Unknown))
	return nil
}

// snippet:end authstatus
