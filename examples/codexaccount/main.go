// Command codexaccount reads a Codex login without ever printing a token:
// the identity decoded from auth.json (and from a fake OS keyring), live
// usage from a local httptest server standing in for the vendor endpoint,
// the subscription, cached usage from the newest rollout, and the login
// and logout commands.
//
// It runs on a copy of the checked-in fixture home; nothing reaches the
// real machine, the network or the real keyring.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/codex"
)

// defaultFixture is the checked-in Codex home, relative to the examples module.
const defaultFixture = "../codex/testdata/homes/0.162.1"

// Layout and fixture facts.
const (
	codexDir    = ".codex"       // the default home under the user home
	keyringDir  = "work-codex"   // a second home that keeps its login in the keyring
	brokenDir   = "broken-codex" // a keyring home whose keyring value is not auth JSON
	fixtureUser = "user"
	filePerm    = os.FileMode(0o600)
	dirPerm     = os.FileMode(0o700)
	headerAuth  = "Authorization" // the server only checks presence; the value is never printed
)

// fixtureNow is a clock before the fixture access token expires
// (2026-09-21T14:13:20Z); Usage refuses an expired token with ErrExpired.
func fixtureNow() time.Time { return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) }

// usageBody is a wham/usage response recorded from the vendor (values replaced).
const usageBody = `{"email":"user@example.test","plan_type":"plus","rate_limit":{"limit_reached":false,
"primary_window":{"used_percent":42.5,"limit_window_seconds":18000,"reset_at":1790000000},
"secondary_window":{"used_percent":10,"limit_window_seconds":604800}}}`

func main() {
	fixture := flag.String("fixture", defaultFixture, "Codex fixture home to copy")
	flag.Parse()
	if err := run(os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "codexaccount:", err)
		os.Exit(1)
	}
}

// run builds the fake machine and prints each section.
func run(w io.Writer, fixture string) error {
	userHome, err := os.MkdirTemp("", "codexaccount-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()

	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		sawAuth = strings.HasPrefix(r.Header.Get(headerAuth), "Bearer ")
		// A failed write means the client hung up; a handler has no caller to
		// report it to, so the result is dropped.
		_, _ = io.WriteString(rw, usageBody)
	}))
	defer srv.Close()

	secrets := agenttest.NewFakeSecrets()
	host, err := buildHost(userHome, fixture, srv, secrets)
	if err != nil {
		return err
	}
	if err := readIdentity(w, host); err != nil {
		return err
	}
	if err := readKeyringIdentity(w, host, secrets); err != nil {
		return err
	}
	if err := readUsage(w, host, srv.URL); err != nil {
		return err
	}
	fmt.Fprintf(w, "server saw a bearer token: %v\n", sawAuth)
	if err := readCachedUsage(w, host); err != nil {
		return err
	}
	return loginCommands(w, host)
}

// buildHost copies the fixture to ~/.codex and uses srv's client as
// Host.HTTP; readUsage points the agent at srv with WithBaseURL.
func buildHost(userHome, fixture string, srv *httptest.Server, secrets *agenttest.FakeSecrets) (agentio.Host, error) {
	if err := agentio.CopyTree(fixture, filepath.Join(userHome, codexDir)); err != nil {
		return agentio.Host{}, err
	}
	return agentio.Host{
		UserHome:  userHome,
		User:      fixtureUser,
		Env:       []string{"HOME=" + userHome, "PATH=" + filepath.Join(userHome, "no-bin")},
		Now:       fixtureNow,
		HTTP:      srv.Client(),
		Secrets:   secrets,
		Processes: &agenttest.FakeProcesses{},
	}, nil
}

// snippet:start identity
// readIdentity decodes the login of the default home from auth.json:
// claims are read locally, never verified, refreshed or sent anywhere.
// Personal values (email, ids) print redacted.
func readIdentity(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	id, err := a.Identity(a.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "logged-in=%v api-key=%v plan=%s (%s) workspace=%v\n",
		id.LoggedIn, id.Mode == codex.AuthAPIKey, id.Plan, id.Plan.DisplayName(), id.Plan.IsWorkspace())
	fmt.Fprintf(w, "email=%v account=%v store=%s\n", id.Email, id.AccountID, id.CredentialStore)
	fmt.Fprintf(w, "access token expires %s\n", id.AccessExpiresAt.UTC().Format(time.RFC3339))

	n := id.Neutral()
	fmt.Fprintf(w, "neutral plan=%q billing=%s\n", n.PlanLabel, n.Billing)

	// ParsePlan normalises raw plan strings the way Codex does.
	fmt.Fprintf(w, "ParsePlan(hc)=%s ParsePlan(Education)=%s\n", codex.ParsePlan("hc"), codex.ParsePlan("Education"))
	return nil
}

// snippet:end identity

// snippet:start keyring
// readKeyringIdentity reads a home whose config.toml says
// cli_auth_credentials_store = "keyring": the login comes from the OS
// keyring (here a fake) under an account derived from the home path.
func readKeyringIdentity(w io.Writer, host agentio.Host, secrets *agenttest.FakeSecrets) error {
	a := codex.New(host)
	auth, err := os.ReadFile(a.Paths(a.DefaultHome()).Auth)
	if err != nil {
		return err
	}
	work, err := newKeyringHome(a, filepath.Join(host.UserHome, keyringDir))
	if err != nil {
		return err
	}
	entry := a.KeyringEntry(work)
	fmt.Fprintf(w, "keyring service=%q account matches KeyringAccount: %v\n",
		entry.Service, entry.Account == codex.KeyringAccount(work.Dir()))
	secrets.Put(entry.Service, entry.Account, string(auth))

	id, err := a.IdentityContext(context.Background(), work)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "keyring login: logged-in=%v store=%s source=%q\n", id.LoggedIn, id.CredentialStore, id.Source)

	// A keyring item that is not auth JSON is a *KeyringError naming the
	// entry, never the value.
	broken, err := newKeyringHome(a, filepath.Join(host.UserHome, brokenDir))
	if err != nil {
		return err
	}
	be := a.KeyringEntry(broken)
	secrets.Put(be.Service, be.Account, "not json")
	_, err = a.Identity(broken)
	var ke *codex.KeyringError
	if errors.As(err, &ke) {
		fmt.Fprintf(w, "keyring error: value is not auth JSON: %v\n", errors.Is(err, codex.ErrKeyringValue))
	}
	return nil
}

// newKeyringHome creates a home whose config.toml selects the keyring store.
func newKeyringHome(a *codex.Agent, dir string) (codex.Home, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return codex.Home{}, err
	}
	cfg := []byte("cli_auth_credentials_store = \"keyring\"\n")
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), cfg, filePerm); err != nil {
		return codex.Home{}, err
	}
	return a.HomeAt(dir)
}

// snippet:end keyring

// snippet:start usage
// readUsage makes one GET to the usage endpoint with the home's access
// token. WithBaseURL sends it to baseURL (here the local server) instead of
// chatgpt.com. Call it once; never poll.
func readUsage(w io.Writer, host agentio.Host, baseURL string) error {
	a := codex.New(host).WithBaseURL(baseURL)
	ctx := context.Background()
	u, err := a.Usage(ctx, a.DefaultHome())
	switch {
	case errors.Is(err, agentio.ErrExpired):
		return fmt.Errorf("token expired: run codex once to refresh it: %w", err)
	case errors.Is(err, agentio.ErrRateLimited):
		return fmt.Errorf("rate limited: try later: %w", err)
	case err != nil:
		return err
	}
	fmt.Fprintf(w, "usage plan=%s limit-reached=%v source=%s\n", u.Plan, u.LimitReached, u.Source)
	for _, win := range u.Windows {
		fmt.Fprintf(w, "window %s length=%s used=%.1f%%\n", win.Vendor, win.Length, win.Percent)
	}

	// ParseUsage decodes a body you fetched yourself.
	if _, err := codex.ParseUsage([]byte(`{"rate_limit":{}}`)); errors.Is(err, codex.ErrNoWindows) {
		fmt.Fprintln(w, "a body without windows is ErrNoWindows")
	}

	// Subscription answers from the id_token claims; only a login without
	// a plan claim costs a usage fetch.
	sub, err := a.Subscription(ctx, a.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "subscription plan=%s source=%s until=%s\n", sub.Plan, sub.Source, sub.ActiveUntil.UTC().Format("2006-01-02"))
	return nil
}

// snippet:end usage

// snippet:start cached-usage
// readCachedUsage reads the last rate-limit snapshot from the newest
// rollout: zero network calls, timestamped by its record.
func readCachedUsage(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	u, found, err := a.CachedUsage(a.DefaultHome())
	if err != nil || !found {
		return err
	}
	fmt.Fprintf(w, "cached source=%s observed=%s\n", u.Source, u.ObservedAt.UTC().Format(time.RFC3339))
	for _, win := range u.Windows {
		fmt.Fprintf(w, "cached window %s limit=%s length=%s used=%.0f%%\n", win.Vendor, win.LimitID, win.Length, win.Percent)
	}
	return nil
}

// snippet:end cached-usage

// snippet:start login
// loginCommands builds `codex login` and `codex logout`. Secrets for the
// stdin flows are written to the process's stdin, never put on argv.
func loginCommands(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	h := a.DefaultHome()
	for _, f := range agentio.LoginFlowValues() {
		c, err := a.LoginCommand(h, f)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-18s codex %s\n", f, strings.Join(c.Args, " "))
	}
	fmt.Fprintf(w, "logout             codex %s\n", strings.Join(a.LogoutCommand(h).Args, " "))
	return nil
}

// snippet:end login
