package codex

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/codex/internal/featuregen"
)

// fixture ids (redacted sample home, testdata/homes/0.162.1).
const (
	fixtureVersion = "0.162.1"
	convNew        = "0199e1a0-0000-7000-8000-000000000001"
	convOld        = "0199e1a0-0000-7000-8000-000000000002"
	fakeSecretEnv  = "fake-secret-value"
)

// setup copies the fixture home to <tmp>/.codex and returns the agent and
// its default home. Rollout mtimes are set so the newer one sorts first.
func setup(t *testing.T, opts ...agenttest.HostOption) (*Agent, Home) {
	t.Helper()
	host := agenttest.NewHost(t, opts...)
	dir := filepath.Join(host.UserHome, defaultHomeDir)
	agenttest.CopyFixture(t, filepath.Join("testdata", "homes", fixtureVersion), dir)
	old := filepath.Join(dir, "sessions/2026/03/01/rollout-2026-03-01T08-00-00-"+convOld+".jsonl")
	newer := filepath.Join(dir, "sessions/2026/10/14/rollout-2026-10-14T10-00-00-"+convNew+".jsonl")
	mustChtimes(t, old, time.Date(2026, 3, 1, 8, 0, 5, 0, time.UTC))
	mustChtimes(t, newer, time.Date(2026, 10, 14, 10, 0, 12, 0, time.UTC))
	a := New(host)
	return a, a.DefaultHome()
}

func mustChtimes(t *testing.T, p string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func TestSpecCompatFeatures(t *testing.T) {
	if err := Spec().Validate(); err != nil {
		t.Fatalf("spec: %v", err)
	}
	c := Compat()
	if c.Min.String() != TestedMin || c.Max.String() != TestedMax {
		t.Fatalf("compat %v", c)
	}
	tsv, err := os.ReadFile("features.tsv")
	if err != nil {
		t.Fatal(err)
	}
	want, err := featuregen.Generate(tsv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("features_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatal("features_gen.go is stale: run go generate ./codex")
	}
	span := FeatureInfo(FeatureFlagNoDaemon)
	if span.Since.String() != "0.156.0" || FeatureSource(FeatureFlagNoDaemon) == "" {
		t.Fatalf("no-daemon span %v", span)
	}
	if FeatureInfo(FeatureFlagAddDir).Known() {
		t.Fatal("add-dir bound must stay unknown")
	}
}

func TestConfigFile(t *testing.T) {
	a, h := setup(t)
	c, warns, err := a.ConfigFileWarnings(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	kc := c.KeyCount()
	if kc.Total != kc.Typed+kc.Raw+kc.Unknown || kc.Unknown != 3 || kc.Raw != 3 || kc.Total < 50 {
		t.Fatalf("key count %+v", kc)
	}
	if c.Model.V != "gpt-5.6-terra" || c.ModelReasoningEffort.V != EffortMedium || c.Personality.V != PersonalityPragmatic {
		t.Fatalf("scalars: %+v", c)
	}
	if c.SandboxMode.Set || c.ApprovalPolicy.Set {
		t.Fatal("absent keys must be unset")
	}
	if !slices.Equal(c.Notify, []string{"/opt/test/notify", "--flag"}) {
		t.Fatalf("notify %v", c.Notify)
	}
	if !c.Features["hooks"].Enabled || c.Features["js_repl"].Enabled {
		t.Fatalf("features %v", c.Features)
	}
	if lvl, ok := c.TrustOf("/work/proj/sub"); !ok || lvl != TrustTrusted {
		t.Fatalf("trust %v %v", lvl, ok)
	}
	lp := c.Profiles["ollama-launch"]
	if lp.ModelProvider.V != "ollama-launch" {
		t.Fatalf("profile %+v", lp)
	}
	if !lp.OpenAIBaseURL.Set {
		t.Fatalf("root-only key in a profile must be typed: %+v", lp)
	}
	s := c.MCPServers["node_repl"]
	if s.Command.V != "/opt/test/node_repl" || s.StartupTimeout.V != 120*time.Second || !slices.Contains(s.EnvKeys, "SECRET_TOKEN") {
		t.Fatalf("mcp %+v", s)
	}
	if s.Args == nil || len(s.Args) != 0 {
		t.Fatal("args = [] must be present and empty")
	}
	if strings.Contains(fmt.Sprintf("%+v", c), fakeSecretEnv) {
		t.Fatal("an MCP env value leaked into Config")
	}
	cu := c.MCPServers["computer-use"]
	if cu.Enabled.V || !cu.Enabled.Set || cu.Transport() != agentio.MCPStdio {
		t.Fatalf("computer-use %+v", cu)
	}
	if !c.Plugins["github@openai-curated"].Enabled.V {
		t.Fatal("plugin enablement")
	}
	if c.Marketplaces["openai-bundled"].SourceType.V != "local" {
		t.Fatal("marketplace")
	}
	g := c.Hooks.Events[HookSessionStart]
	if len(g) != 1 || len(g[0].Hooks) != 1 || g[0].Hooks[0].Command.V != "/opt/test/hook.sh" || g[0].Hooks[0].Timeout.V != 30*time.Second {
		t.Fatalf("hooks %+v", c.Hooks)
	}
	if !strings.Contains(string(c.TUI), `"gpt-5.5" = 4`) || !strings.Contains(string(c.Desktop), "STEPS_COMMANDS") {
		t.Fatalf("raw sections: tui=%q desktop=%q", c.TUI, c.Desktop)
	}
	if got := c.Unknown["some_future_key.nested"]; got != "true" {
		t.Fatalf("unknown key kept as source text: %v", c.Unknown)
	}
	if !slices.Equal(c.ShellEnvironmentPolicy.SetKeys, []string{"NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S"}) {
		t.Fatal("shell env set keys")
	}
	if err := c.ProfileConflict("ollama-launch"); !errors.Is(err, ErrProfileConflict) {
		t.Fatalf("profile conflict: %v", err)
	}
	if c.ProfileConflict("work") != nil {
		t.Fatal("no conflict for work")
	}
	env, err := a.MCPServerEnvValue(h, "node_repl", "SECRET_TOKEN")
	if err != nil || env.Reveal() != fakeSecretEnv || strings.Contains(fmt.Sprint(env), fakeSecretEnv) {
		t.Fatalf("env value: %v", err)
	}
}

func TestConfigTypeChangeDegradesOneKey(t *testing.T) {
	src := []byte("model = 5\nmodel_provider = \"openai\"\n[mcp_servers.x]\ncommand = [\"a\"]\nargs = [\"b\"]\n")
	c, warns, err := ParseConfig(src, "config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if c.Model.Set || c.ModelProvider.V != "openai" {
		t.Fatalf("model %+v", c)
	}
	if !warns.Has(agentio.WarnTypeChanged) || len(warns) != 2 {
		t.Fatalf("warnings %v", warns)
	}
	if c.Unknown["model"] != "5" || c.MCPServers["x"].Unknown["command"] != `["a"]` {
		t.Fatalf("raw kept: %v %v", c.Unknown, c.MCPServers["x"].Unknown)
	}
	if !slices.Equal(c.MCPServers["x"].Args, []string{"b"}) {
		t.Fatal("rest of the table still parsed")
	}
}

func TestEffective(t *testing.T) {
	a, h := setup(t)
	proj := t.TempDir()
	agenttest.WriteFile(t, proj, ".git/HEAD", "ref")
	agenttest.WriteFile(t, proj, ".codex/config.toml", "model = \"proj-model\"\nnotify = [\"denied\"]\n")
	eff, err := a.Effective(h, EffectiveQuery{Dir: proj, Profile: "work"})
	if err != nil {
		t.Fatal(err)
	}
	// untrusted project (not in [projects]) → disabled; profile wins over user
	if eff.Config.Model.V != "gpt-6-luna" || eff.Origins["model"] != LayerUserProfile {
		t.Fatalf("model %v from %v", eff.Config.Model, eff.Origins["model"])
	}
	cfgPath := a.Paths(h).Config
	trusted := fmt.Sprintf("\n[projects.%q]\ntrust_level = \"trusted\"\n", proj)
	b, _ := os.ReadFile(cfgPath)
	agenttest.WriteFile(t, h.Dir(), fileConfig, string(b)+trusted)
	eff, err = a.Effective(h, EffectiveQuery{Dir: proj, Overrides: []Override{OverrideString("model_reasoning_effort", "low")}})
	if err != nil {
		t.Fatal(err)
	}
	if eff.Config.Model.V != "proj-model" || eff.Origins["model"] != LayerProject {
		t.Fatalf("project layer: %v", eff.Config.Model)
	}
	if slices.Contains(eff.Config.Notify, "denied") {
		t.Fatal("notify is denied in project layers")
	}
	if eff.Config.ModelReasoningEffort.V != EffortLow || eff.Origins["model_reasoning_effort"] != LayerSession {
		t.Fatal("override layer")
	}
	names, err := a.ProfileFiles(h)
	if err != nil || !slices.Equal(names, []string{"work"}) {
		t.Fatalf("profile files %v %v", names, err)
	}
	ov, err := ParseOverride(`sandbox_mode=read-only`)
	if err != nil || ov.Value != `"read-only"` {
		t.Fatalf("literal override %v", ov)
	}
	ov, _ = ParseOverride(`model_context_window=1000`)
	if ov.Value != "1000" {
		t.Fatalf("toml override %v", ov)
	}
}

func TestIdentity(t *testing.T) {
	a, h := setup(t)
	id, err := a.Identity(h)
	if err != nil {
		t.Fatal(err)
	}
	if !id.LoggedIn || id.Mode != AuthChatGPT || id.Plan != PlanPlus || id.Email.Reveal() != "user@example.test" {
		t.Fatalf("identity %+v", id)
	}
	if id.SubscriptionActiveUntil == nil || id.AccessExpiresAt.Unix() != 1790000000 || id.AccountID.Reveal() != "acct-0000" {
		t.Fatalf("claims %+v", id)
	}
	out := fmt.Sprintf("%+v %v", id, id.Neutral())
	if strings.Contains(out, "user@example.test") || strings.Contains(out, "acct-0000") {
		t.Fatal("personal value printed")
	}
	n := id.Neutral()
	if n.PlanLabel != "Plus" || n.Billing != agentio.BillingSubscription || n.Credential != agentio.CredentialFile {
		t.Fatalf("neutral %+v", n)
	}
	if ParsePlan("hc") != PlanEnterprise || PlanPro.DisplayName() != "Pro (More)" || !PlanTeam.IsWorkspace() || Plan("zzz").IsKnown() {
		t.Fatal("plan table")
	}
}

// upstreamKeyringVector is login/src/auth/storage_tests.rs:481-487: a path
// that cannot be canonicalized hashes as given.
const (
	upstreamKeyringPath    = "~/.codex"
	upstreamKeyringAccount = "cli|940db7b1d0e4eb40"
	fakeRefreshToken       = "rt_fake_refresh"
)

// keyringSetup is setup with cli_auth_credentials_store = store, the
// fixture auth.json moved into the fake keyring when inKeyring, and
// auth.json removed when !keepFile.
func keyringSetup(t *testing.T, store CredentialStore, inKeyring, keepFile bool) (*Agent, Home, *agenttest.FakeSecrets, []byte) {
	t.Helper()
	secrets := agenttest.NewFakeSecrets()
	a, h := setup(t, agenttest.WithSecrets(secrets))
	raw, err := os.ReadFile(a.Paths(h).Auth)
	if err != nil {
		t.Fatal(err)
	}
	if inKeyring {
		secrets.Put(KeyringService, KeyringAccount(h.Dir()), string(raw))
	}
	if !keepFile {
		if err := os.Remove(a.Paths(h).Auth); err != nil {
			t.Fatal(err)
		}
	}
	agenttest.WriteFile(t, h.Dir(), fileConfig, "cli_auth_credentials_store = \""+string(store)+"\"\n")
	return a, h, secrets, raw
}

// assertNoSecret fails when any token of the fixture login shows up in out.
func assertNoSecret(t *testing.T, out string, raw []byte) {
	t.Helper()
	var w struct {
		Tokens struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{w.Tokens.IDToken, w.Tokens.AccessToken, w.Tokens.RefreshToken, fakeRefreshToken, "user@example.test"} {
		if v != "" && strings.Contains(out, v) {
			t.Fatalf("secret or personal value printed: %q…", v[:min(8, len(v))])
		}
	}
}

func TestKeyringAccount(t *testing.T) {
	if got := KeyringAccount(upstreamKeyringPath); got != upstreamKeyringAccount {
		t.Fatalf("upstream vector: %s", got)
	}
	a, h := setup(t)
	if got := a.KeyringEntry(h); got.Service != "Codex Auth" || !strings.HasPrefix(got.Account, "cli|") || len(got.Account) != len(upstreamKeyringAccount) {
		t.Fatalf("default home entry %+v", got)
	}
	other := filepath.Join(t.TempDir(), "work-codex")
	agenttest.WriteFile(t, other, fileConfig, "")
	oh, err := a.HomeAt(other)
	if err != nil {
		t.Fatal(err)
	}
	if KeyringAccount(oh.Dir()) == KeyringAccount(h.Dir()) {
		t.Fatal("non-default home must have its own account")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	if KeyringAccount(link) != KeyringAccount(other) {
		t.Fatal("a symlinked home path must hash like its target (canonicalize)")
	}
}

func TestIdentityKeyring(t *testing.T) {
	a, h, secrets, raw := keyringSetup(t, StoreKeyring, true, false)
	id, err := a.Identity(h)
	if err != nil || !id.LoggedIn || id.CredentialStore != StoreKeyring || id.Plan != PlanPlus || id.Source != "" {
		t.Fatalf("keyring identity %+v %v", id, err)
	}
	want := agenttest.SecretKey{Service: KeyringService, Account: KeyringAccount(h.Dir())}
	if !slices.Equal(secrets.Calls, []agenttest.SecretKey{want}) || id.Keyring.Account != want.Account {
		t.Fatalf("lookups %v (named account only, never service-only)", secrets.Calls)
	}
	if n := id.Neutral(); n.Credential != agentio.CredentialKeyring || !n.LoggedIn {
		t.Fatalf("neutral %+v", n)
	}
	assertNoSecret(t, fmt.Sprintf("%+v %v %#v", id, id.Neutral(), id), raw)
	checks := a.Checks(context.Background(), h)
	assertNoSecret(t, fmt.Sprintf("%+v", checks), raw)
}

func TestIdentityKeyringIgnoresFile(t *testing.T) {
	// keyring mode reads only the keyring (storage.rs:309-313): a leftover
	// auth.json does not make the home logged in.
	a, h, _, _ := keyringSetup(t, StoreKeyring, false, true)
	id, err := a.Identity(h)
	if err != nil || id.LoggedIn || id.CredentialStore != StoreKeyring || id.Keyring.Service != KeyringService {
		t.Fatalf("keyring miss %+v %v", id, err)
	}
	var login agentio.Check
	for _, c := range a.Checks(context.Background(), h) {
		if c.ID == agentio.CheckLogin {
			login = c
		}
	}
	if login.Status != agentio.CheckWarn || !strings.Contains(login.Message, id.Keyring.Account) || login.Fix == nil {
		t.Fatalf("login check %+v", login)
	}
	if _, err := a.Usage(context.Background(), h); !errors.Is(err, agentio.ErrNotLoggedIn) {
		t.Fatalf("usage without a keyring login: %v", err)
	}
}

func TestIdentityAutoFallback(t *testing.T) {
	t.Run("keyring hit", func(t *testing.T) {
		a, h, _, _ := keyringSetup(t, StoreAuto, true, false)
		if id, err := a.Identity(h); err != nil || !id.LoggedIn || id.CredentialStore != StoreKeyring {
			t.Fatalf("%+v %v", id, err)
		}
	})
	t.Run("no entry falls back to auth.json", func(t *testing.T) {
		a, h, secrets, _ := keyringSetup(t, StoreAuto, false, true)
		id, err := a.Identity(h)
		if err != nil || !id.LoggedIn || id.CredentialStore != StoreFile || id.Source != a.Paths(h).Auth || len(secrets.Calls) != 1 {
			t.Fatalf("%+v %v", id, err)
		}
	})
	t.Run("keyring error falls back to auth.json", func(t *testing.T) {
		a, h, secrets, _ := keyringSetup(t, StoreAuto, false, true)
		secrets.Err = errors.New("keychain locked")
		if id, err := a.Identity(h); err != nil || !id.LoggedIn || id.CredentialStore != StoreFile {
			t.Fatalf("%+v %v", id, err)
		}
	})
	t.Run("keyring error and no file", func(t *testing.T) {
		a, h, secrets, _ := keyringSetup(t, StoreAuto, false, false)
		cause := errors.New("keychain locked")
		secrets.Err = cause
		id, err := a.Identity(h)
		var kerr *KeyringError
		if !errors.Is(err, cause) || !errors.As(err, &kerr) || kerr.Entry.Account != KeyringAccount(h.Dir()) || id.CredentialStore != StoreAuto {
			t.Fatalf("%+v %v", id, err)
		}
		var keyring agentio.Check
		for _, c := range a.Checks(context.Background(), h) {
			if c.ID == CheckKeyring {
				keyring = c
			}
		}
		if keyring.Status != agentio.CheckWarn {
			t.Fatalf("doctor %+v", keyring)
		}
	})
	t.Run("found nowhere", func(t *testing.T) {
		a, h, _, _ := keyringSetup(t, StoreAuto, false, false)
		if id, err := a.Identity(h); err != nil || id.LoggedIn || id.CredentialStore != StoreAuto {
			t.Fatalf("%+v %v", id, err)
		}
	})
}

// TestIdentityKeyringStoreUnsupported: a Host whose keychain cannot look on
// this OS (HostFromOS off darwin) makes a keyring home ErrUnsupported, never
// a logged-out identity.
func TestIdentityKeyringStoreUnsupported(t *testing.T) {
	a, h, secrets, _ := keyringSetup(t, StoreKeyring, false, false)
	secrets.Err = agentio.UnsupportedError("", "keychain", "no keychain reader for test")
	id, err := a.Identity(h)
	if !errors.Is(err, agentio.ErrUnsupported) || id.LoggedIn {
		t.Fatalf("unsupported store: %+v %v", id, err)
	}
}

func TestIdentityKeyringBadValueAndEphemeral(t *testing.T) {
	a, h, secrets, raw := keyringSetup(t, StoreKeyring, false, false)
	bad := "not-json " + fakeRefreshToken
	secrets.Put(KeyringService, KeyringAccount(h.Dir()), bad)
	_, err := a.Identity(h)
	if !errors.Is(err, ErrKeyringValue) || strings.Contains(err.Error(), fakeRefreshToken) {
		t.Fatalf("bad value: %v", err)
	}
	_, uerr := a.Usage(context.Background(), h)
	if !errors.Is(uerr, ErrKeyringValue) {
		t.Fatalf("usage bad value: %v", uerr)
	}
	assertNoSecret(t, err.Error()+uerr.Error(), raw)
	// `security -w` prints a value with unprintable bytes as hex.
	secrets.Put(KeyringService, KeyringAccount(h.Dir()), hex.EncodeToString(raw))
	if id, err := a.Identity(h); err != nil || !id.LoggedIn {
		t.Fatalf("hex value %+v %v", id, err)
	}

	eph, eh, esecrets, _ := keyringSetup(t, StoreEphemeral, false, true)
	if _, err := eph.Identity(eh); !errors.Is(err, agentio.ErrUnsupported) || len(esecrets.Calls) != 0 {
		t.Fatalf("ephemeral: %v", err)
	}
}

const usageBody = `{"email":"user@example.test","plan_type":"plus","rate_limit":{"limit_reached":false,
"primary_window":{"used_percent":42.5,"limit_window_seconds":18000,"reset_at":1790000000},
"secondary_window":{"used_percent":10,"limit_window_seconds":604800}}}`

func TestUsageHTTP(t *testing.T) {
	var gotAuth, gotAcct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotAcct = r.Header.Get(headerAuth), r.Header.Get(headerAccountID)
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		_, _ = w.Write([]byte(usageBody))
	}))
	defer srv.Close()
	before := func() time.Time { return time.Unix(1789995000, 0) }
	a, h := setup(t, agenttest.WithHTTP(srv.Client()), agenttest.WithNow(before))
	u, err := a.usageAt(context.Background(), h, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotAuth, bearerPrefix) || gotAcct != "acct-0000" {
		t.Fatal("request headers")
	}
	n := u.Neutral()
	if len(n.Windows) != 2 || n.Windows[0].Class() != agentio.WindowShort || n.Windows[1].Class() != agentio.WindowWeek || n.Windows[0].Percent != 42.5 {
		t.Fatalf("windows %+v", n.Windows)
	}
	if n.Windows[0].Label != "5h" || n.Windows[1].ResetsAt != nil {
		t.Fatalf("labels %+v", n.Windows)
	}

	after := func() time.Time { return time.Unix(1790000001, 0) }
	a2, h2 := setup(t, agenttest.WithHTTP(agenttest.FailingHTTP(t)), agenttest.WithNow(after))
	if _, err := a2.usageAt(context.Background(), h2, srv.URL); !errors.Is(err, agentio.ErrExpired) {
		t.Fatalf("expired: %v", err)
	}

	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer limited.Close()
	a3, h3 := setup(t, agenttest.WithHTTP(limited.Client()), agenttest.WithNow(before))
	if _, err := a3.usageAt(context.Background(), h3, limited.URL); !errors.Is(err, agentio.ErrRateLimited) {
		t.Fatalf("429: %v", err)
	}
	if _, err := ParseUsage([]byte(`{"rate_limit":{}}`)); !errors.Is(err, ErrNoWindows) {
		t.Fatal("no windows")
	}
}

func TestCachedUsageAndSubscription(t *testing.T) {
	a, h := setup(t)
	u, ok, err := a.CachedUsage(h)
	if err != nil || !ok {
		t.Fatalf("cached %v %v", ok, err)
	}
	if len(u.Windows) != 1 || u.Windows[0].Length != 7*24*time.Hour || u.Windows[0].Percent != 12 || u.Source != agentio.UsageTranscript {
		t.Fatalf("cached %+v", u)
	}
	if u.ObservedAt.IsZero() || u.Plan != PlanPlus {
		t.Fatal("observed at / plan")
	}
	s, err := a.Subscription(context.Background(), h)
	if err != nil || s.Plan != PlanPlus || s.Source != agentio.UsageCache || s.ActiveStart == nil {
		t.Fatalf("subscription from claims %+v %v", s, err)
	}
}

func TestConversationsAndTurns(t *testing.T) {
	a, h := setup(t)
	cs, err := a.Conversations(context.Background(), h, ConversationQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].ID != convNew || cs[0].Title != "List the files" || cs[0].Dir != "/work/proj" {
		t.Fatalf("conversations %+v", cs)
	}
	if cs[0].Source != SourceCLI || cs[0].CLIVersion.String() != "0.162.1" || cs[0].Git == nil || cs[1].Source != SourceVSCode || cs[1].Title != "hello" {
		t.Fatalf("meta %+v", cs[0])
	}
	only, _ := a.Conversations(context.Background(), h, ConversationQuery{Dir: "/work/other"})
	if len(only) != 1 || only[0].ID != convOld {
		t.Fatal("dir filter")
	}
	c, err := a.Conversation(h, convOld)
	if err != nil || c.Dir != "/work/other" {
		t.Fatalf("by id %v", err)
	}
	if ok, _ := a.HasHistory(h, "/work/proj"); !ok {
		t.Fatal("has history")
	}
	if ok, _ := a.HasHistory(h, "/nowhere"); ok {
		t.Fatal("no history")
	}

	var turns []agentio.Turn
	off, err := a.Turns(h, convNew, 0, func(tn agentio.Turn) error { turns = append(turns, tn); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Role != agentio.RoleUser || turns[0].Text != "list the files" || turns[1].Role != agentio.RoleAssistant {
		t.Fatalf("turns %+v", turns)
	}
	if turns[1].Tokens == nil || turns[1].Tokens.Reasoning != 20 || turns[1].Model != "gpt-5.5" {
		t.Fatalf("assistant turn %+v", turns[1])
	}
	again, err := a.Turns(h, convNew, off, func(agentio.Turn) error { t.Fatal("no new turns expected"); return nil })
	if err != nil || again != off {
		t.Fatalf("resume at offset: %v %v", again, err)
	}
	var old []agentio.Turn
	if _, err := a.Turns(h, convOld, 0, func(tn agentio.Turn) error { old = append(old, tn); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(old) != 2 || old[1].Text != "hi there" {
		t.Fatalf("legacy events %+v", old)
	}
}

func TestRolloutReader(t *testing.T) {
	a, h := setup(t)
	c, err := a.Conversation(h, convNew)
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.OpenRollout(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	kinds := map[RecordKind]int{}
	for {
		rec, err := r.Next()
		if err != nil {
			break
		}
		kinds[rec.Kind]++
		if len(rec.Raw) == 0 {
			t.Fatal("raw must always be set")
		}
		if rec.SessionMeta != nil && rec.SessionMeta.Unknown["brand_new_meta_key"] == nil {
			t.Fatal("unknown session_meta key dropped")
		}
		if rec.TurnContext != nil && (rec.TurnContext.ApprovalPolicy != ApprovalOnRequest || rec.TurnContext.SandboxPolicy.Type != ModeWorkspaceWrite) {
			t.Fatalf("turn context %+v", rec.TurnContext)
		}
	}
	if kinds["brand_new_record_type"] != 1 || kinds[KindWorldState] != 1 || kinds[KindEventMsg] != 5 {
		t.Fatalf("kinds %v", kinds)
	}
	tok, err := a.ConversationTokens(c.Path)
	if err != nil || tok.Total != 1280 || tok.Neutral().CacheWrite != 0 || tok.Neutral().Reasoning != 20 {
		t.Fatalf("tokens %+v %v", tok, err)
	}
	tail, err := a.RolloutTail(c.Path, 400)
	if err != nil || len(tail) == 0 || tail[len(tail)-1].Kind != "brand_new_record_type" {
		t.Fatalf("tail %v", err)
	}

	// an incomplete trailing line is never consumed
	part := filepath.Join(t.TempDir(), "rollout-x.jsonl")
	agenttest.WriteFile(t, filepath.Dir(part), filepath.Base(part), `{"type":"compacted","payload":{"message":"m"}}`+"\n"+`{"type":"event_msg","pay`)
	pr, err := a.OpenRollout(part)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()
	if rec, err := pr.Next(); err != nil || rec.Compacted == nil {
		t.Fatalf("first line %v", err)
	}
	if _, err := pr.Next(); err == nil {
		t.Fatal("partial line must not decode")
	}
	if pr.Offset() != agentio.Offset(len(`{"type":"compacted","payload":{"message":"m"}}`)+1) {
		t.Fatalf("offset %d", pr.Offset())
	}
}

func TestPromptHistory(t *testing.T) {
	a, h := setup(t)
	es, err := a.PromptHistory(h, agentio.PromptQuery{Limit: 2})
	if err != nil || len(es) != 2 || es[0].Text != "and the sizes" {
		t.Fatalf("history %+v %v", es, err)
	}
	es, _ = a.PromptHistory(h, agentio.PromptQuery{ConversationID: convOld})
	if len(es) != 1 || es[0].Text != "hello" {
		t.Fatalf("by session %+v", es)
	}
}

func TestCatalogSkillsPluginsRules(t *testing.T) {
	a, h := setup(t)
	cat, err := a.Models(h)
	if err != nil || len(cat.Models) != 2 || !cat.Models[0].Visible || cat.Models[1].Visible {
		t.Fatalf("models %+v %v", cat, err)
	}
	if cat.Models[0].Unknown["shell_type"] == nil || cat.Models[0].ContextWindow != 272000 {
		t.Fatal("model unknown keys kept")
	}
	if cat.Validate("gpt-6-luna", EffortLow) != nil || !errors.Is(cat.Validate("gpt-6-luna", EffortXHigh), ErrUnsupportedEffort) || !errors.Is(cat.Validate("nope", ""), ErrUnknownModel) {
		t.Fatal("validate")
	}
	skills, err := a.Skills(h, "")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Skill{}
	for _, s := range skills {
		byName[s.Name] = s
	}
	if byName["squad"].Scope != agentio.SkillUser || byName["squad"].Extra["squad-version"] != "0.7.6" {
		t.Fatalf("user skill %+v", byName["squad"])
	}
	if byName["imagegen"].Scope != agentio.SkillSystem || byName["imagegen"].Description != "Generate images from prompts." {
		t.Fatalf("system skill %+v", byName["imagegen"])
	}
	if byName["pr-review"].Plugin != "github" || byName["pr-review"].Scope != agentio.SkillPlugin {
		t.Fatal("plugin skill")
	}
	plugins, err := a.Plugins(h)
	if err != nil || len(plugins) != 1 || !plugins[0].Enabled || plugins[0].Version != "26.928.31416" || plugins[0].Interface.DisplayName != "GitHub" {
		t.Fatalf("plugins %+v %v", plugins, err)
	}
	if plugins[0].Unknown["author"] == nil {
		t.Fatal("manifest unknown keys")
	}
	rules, issues, err := a.Rules(h, "")
	if err != nil || len(rules) != 3 || len(issues) != 1 {
		t.Fatalf("rules %+v issues %+v %v", rules, issues, err)
	}
	if !slices.Equal(rules[2].Pattern, []string{"git", "status"}) || rules[2].Decision != DecisionPrompt || rules[2].Line != 5 {
		t.Fatalf("multi-line rule %+v", rules[2])
	}
	uc, ok, err := a.UpdateCache(h)
	if err != nil || !ok || uc.LatestVersion.String() != "0.156.1" || uc.DismissedVersion != nil {
		t.Fatalf("update cache %+v", uc)
	}
}

func TestInstructionFiles(t *testing.T) {
	a, h := setup(t)
	proj := t.TempDir()
	agenttest.WriteFile(t, proj, ".git/HEAD", "ref")
	agenttest.WriteFile(t, proj, "AGENTS.md", "root rules")
	agenttest.WriteFile(t, proj, "sub/AGENTS.override.md", "override")
	agenttest.WriteFile(t, proj, "sub/AGENTS.md", "shadowed by override")
	files, err := a.InstructionFiles(h, filepath.Join(proj, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || files[0].Scope != agentio.InstructionUser || files[1].Kind != fileAgentsMD || files[2].Kind != fileAgentsOverride {
		t.Fatalf("files %+v", files)
	}
	root, ok, err := a.ProjectRoot(h, filepath.Join(proj, "sub"))
	if err != nil || !ok || root != proj {
		t.Fatalf("root %v %v", root, err)
	}
}

func TestLaunch(t *testing.T) {
	a, h := setup(t, agenttest.WithEnv("PATH=/usr/bin", EnvHome+"=/elsewhere"))
	n := a.Neutral()
	cmd, err := n.Launch(h.Target(), agentio.LaunchRequest{Prompt: "hi", Model: "o3", AddDirs: []string{"/x"}, Autonomy: agentio.AutonomyAsk})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-m", "o3", "-s", "workspace-write", "-a", "on-request", "--add-dir", "/x", "hi"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args %v", cmd.Args)
	}
	if _, ok := agentio.EnvLookup(cmd.Env, EnvHome); ok {
		t.Fatal("CODEX_HOME must be absent for the default home")
	}
	other := a.core.HomeAt(filepath.Join(t.TempDir(), "other"))
	cmd, _ = n.Launch(other.Target(), agentio.LaunchRequest{ResumeID: convNew, Headless: true, Autonomy: agentio.AutonomyAsk})
	if !slices.Equal(cmd.Args[:2], []string{"exec", "resume"}) || !slices.Contains(cmd.Args, "--json") || slices.Contains(cmd.Args, "-a") || cmd.Args[len(cmd.Args)-1] != convNew {
		t.Fatalf("exec resume %v", cmd.Args)
	}
	if v, _ := agentio.EnvLookup(cmd.Env, EnvHome); v != other.Dir() {
		t.Fatal("CODEX_HOME set for a non-default home")
	}
	if _, err := n.Launch(h.Target(), agentio.LaunchRequest{Autonomy: agentio.AutonomyAcceptEdits}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Fatalf("accept-edits: %v", err)
	}
	if _, err := n.Launch(h.Target(), agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); !errors.Is(err, agentio.ErrDangerousNotAllowed) {
		t.Fatalf("full without allow: %v", err)
	}
	old := h.Target()
	old.Version = agentio.MustVersion("0.150.0")
	_, err = a.New(old, LaunchOptions{NoDaemon: true})
	var fe *FeatureUnavailableError
	if !errors.As(err, &fe) || !errors.Is(err, agentio.ErrFeatureUnavailable) || fe.Feature != FeatureFlagNoDaemon {
		t.Fatalf("feature gate: %v", err)
	}
	cfg, _ := a.ConfigFile(h)
	if _, err := a.New(h.Target(), LaunchOptions{Profile: "ollama-launch", ProfileCheck: &cfg}); !errors.Is(err, ErrProfileConflict) {
		t.Fatalf("profile conflict: %v", err)
	}
	cur := h.Target()
	cur.Version = agentio.MustVersion(fixtureVersion)
	cmd, err = a.Exec(cur, ExecOptions{LaunchOptions: LaunchOptions{PromptFromStdin: true, Approval: ApprovalNever}, JSON: true})
	if err != nil || !slices.Equal(cmd.Args, []string{"exec", "-c", `approval_policy="never"`, "--json", "-"}) || len(cmd.Warnings) != 0 {
		t.Fatalf("exec %v %v %v", cmd.Args, cmd.Warnings, err)
	}
	cmd, _ = a.New(h.Target(), LaunchOptions{})
	if !cmd.Warnings.Has(agentio.WarnVersionUnknown) {
		t.Fatal("unknown version warns")
	}
	if _, err := a.MCPAddCommand(h, "x", MCPAddSpec{Command: "srv", Env: map[string]agentio.EnvValue{"K": agentio.NewEnvValue("v")}}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Fatal("mcp add with env values must be refused")
	}
	lc, err := a.LoginCommand(h, agentio.LoginAPIKeyStdin)
	if err != nil || !slices.Equal(lc.Args, []string{"login", "--with-api-key"}) {
		t.Fatalf("login %v", lc.Args)
	}
}

const execStream = `{"type":"thread.started","thread_id":"th-1"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"done"}}
{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"ls","aggregated_output":"a","exit_code":0,"status":"completed"}}
warning: not json
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":3,"reasoning_output_tokens":1}}
{"type":"future.event","x":1}
`

func TestExecDecoder(t *testing.T) {
	a := New(agenttest.NewHost(t))
	d := a.NewRunDecoder(strings.NewReader(execStream))
	var types []agentio.RunEventType
	var last agentio.RunEvent
	for {
		ev, err := d.Next()
		if err != nil {
			break
		}
		types = append(types, ev.Type)
		last = ev
		if ev.Type == agentio.RunEventDone && (ev.Tokens == nil || ev.Tokens.Reasoning != 1) {
			t.Fatal("usage tokens")
		}
	}
	want := []agentio.RunEventType{agentio.RunEventStarted, agentio.RunEventOther, agentio.RunEventMessage, agentio.RunEventTool, agentio.RunEventDone, agentio.RunEventOther}
	if !slices.Equal(types, want) || !strings.Contains(string(last.Raw), "future.event") {
		t.Fatalf("types %v", types)
	}
	ev, _ := NewExecDecoder(strings.NewReader(execStream)).Next()
	if ev.ThreadID != "th-1" {
		t.Fatal("thread id")
	}
}

func TestClassifyAndProcesses(t *testing.T) {
	procs := &agenttest.FakeProcesses{List: []agentio.Process{
		{PID: 1, Exe: "/opt/x/codex", Argv: []string{"codex"}},
		{PID: 2, Exe: "/opt/x/codex", Argv: []string{"codex", "exec", "--json"}},
		{PID: 3, Exe: "/usr/bin/node", Argv: []string{"node", "/lib/node_modules/@openai/codex/bin/codex.js"}},
		{PID: 4, Exe: "/opt/x/codex", Argv: []string{"codex", "app-server", "daemon"}},
		{PID: 5, Exe: "/usr/bin/vim", Argv: []string{"vim"}},
	}, Cwds: map[int]string{1: "/work/proj"}}
	a := New(agenttest.NewHost(t, agenttest.WithProcesses(procs)))
	ps, err := a.Processes(context.Background())
	if err != nil || len(ps) != 4 {
		t.Fatalf("processes %v %v", ps, err)
	}
	kinds := []agentio.ProcessKind{ps[0].Kind, ps[1].Kind, ps[2].Kind, ps[3].Kind}
	if !slices.Equal(kinds, []agentio.ProcessKind{agentio.ProcessCLI, agentio.ProcessExec, agentio.ProcessLauncher, agentio.ProcessAppServer}) || ps[0].Cwd != "/work/proj" {
		t.Fatalf("kinds %v", kinds)
	}
	if info, _ := a.ClassifyProcess(procs.List[3]); info.Session {
		t.Fatal("app-server is not a session")
	}
}

// liveThread is the thread id of the capture (CODEX_API.md §15).
const liveThread = "01a1d83a-e1cb-70d1-bee7-7aebab05bc2b"

func TestParseThreadLockName(t *testing.T) {
	if id, ok := ParseThreadLockName(liveThread + ".lock"); !ok || id != liveThread {
		t.Fatalf("thread lock %q %v", id, ok)
	}
	for _, n := range []string{
		".coordination.lock", liveThread, liveThread + ".lock.tmp", strings.ToUpper(liveThread) + ".lock",
		"01a1d83ae1cb70d1bee77aebab05bc2b.lock", "{" + liveThread + "}.lock", "01a1d83a-e1cb-70d1-bee7-7aebab05bc2g.lock",
		"01a1d83a+e1cb-70d1-bee7-7aebab05bc2b.lock", "junk.lock", "",
	} {
		if _, ok := ParseThreadLockName(n); ok {
			t.Errorf("%q accepted", n)
		}
	}
}

func TestLiveSessionsFromThreadLocks(t *testing.T) {
	lockTime := time.Date(2026, 11, 14, 4, 5, 1, 0, time.UTC)
	procs := &agenttest.FakeProcesses{}
	a, h := setup(t, agenttest.WithProcesses(procs))
	locks := a.Paths(h).ThreadWriterLocks
	// The fixture carries the permanent empty .coordination.lock; add a
	// thread lock (the captured empty file), a junk name and a directory.
	sample, err := os.ReadFile(filepath.Join("testdata", "homes", fixtureVersion, dirThreadWriterLocks, coordinationLockFile))
	if err != nil || len(sample) != 0 {
		t.Fatalf("fixture coordination lock %v", err)
	}
	agenttest.WriteFile(t, locks, liveThread+threadLockSuffix, "")
	agenttest.WriteFile(t, locks, "notes.lock", "")
	if err := os.Mkdir(filepath.Join(locks, convNew+threadLockSuffix), 0o700); err != nil {
		t.Fatal(err)
	}
	mustChtimes(t, filepath.Join(locks, liveThread+threadLockSuffix), lockTime)

	tl, err := a.ThreadLocks(h)
	if err != nil || len(tl) != 1 || tl[0].ThreadID != liveThread || !tl[0].ModTime.Equal(lockTime) {
		t.Fatalf("thread locks %+v %v", tl, err)
	}

	// No Codex process: the lock is a crash leftover.
	procs.List = []agentio.Process{{PID: 9, Exe: "/usr/bin/vim", Argv: []string{"vim"}, Start: lockTime.Add(-time.Hour)}}
	ls, err := a.LiveSessions(h)
	if err != nil || len(ls) != 1 || ls[0].Alive || ls[0].ConversationID != liveThread || ls[0].PID != 0 || !ls[0].StartedAt.Equal(lockTime) || ls[0].Home != h.Neutral() {
		t.Fatalf("stale %+v %v", ls, err)
	}
	if live, err := a.IsLive(h, liveThread); err != nil || live {
		t.Fatalf("stale IsLive %v %v", live, err)
	}
	if exists, err := a.LiveConversation(h, liveThread); err != nil || !exists {
		t.Fatalf("LiveConversation is the unchecked file test: %v %v", exists, err)
	}

	// Only the npm launcher, or a codex that started after the lock: still stale.
	procs.List = []agentio.Process{
		{PID: 10, Exe: "/usr/bin/node", Argv: []string{"node", "/lib/node_modules/@openai/codex/bin/codex.js"}, Start: lockTime.Add(-time.Minute)},
		{PID: 11, Exe: "/opt/x/codex", Argv: []string{"codex", "exec", "hi"}, Start: lockTime.Add(time.Minute)},
	}
	if live, _ := a.IsLive(h, liveThread); live {
		t.Fatal("launcher or later codex must not keep the lock alive")
	}

	// A codex exec that started before the lock: alive.
	procs.List = append(procs.List, agentio.Process{PID: 12, Exe: "/opt/x/codex", Argv: []string{"codex", "exec", "hi"}, Start: lockTime.Add(-time.Second)})
	if live, err := a.IsLive(h, liveThread); err != nil || !live {
		t.Fatalf("alive %v %v", live, err)
	}
	for _, id := range []string{convNew, "", "notes", "../" + liveThread} {
		if live, _ := a.IsLive(h, id); live {
			t.Errorf("IsLive(%q)", id)
		}
		if exists, _ := a.LiveConversation(h, id); exists {
			t.Errorf("LiveConversation(%q)", id)
		}
	}

	// A lock for a thread whose rollout exists fills Dir from session_meta.
	agenttest.WriteFile(t, locks, convNew+"x", "")
	agenttest.WriteFile(t, locks, convOld+threadLockSuffix, "")
	ls, _ = a.LiveSessions(h)
	if len(ls) != 2 || ls[0].ConversationID != convOld || ls[0].Dir == "" || ls[1].Dir != "" {
		t.Fatalf("dir from rollout %+v", ls)
	}

	// The neutral lister answers the same; FindLiveSession cannot match a pid.
	n := a.Neutral()
	if lister, ok := agentio.LiveSessionsOf(n); !ok {
		t.Fatal("lister")
	} else if got, err := lister.LiveSessions(h.Neutral()); err != nil || len(got) != 2 {
		t.Fatalf("neutral %v %v", got, err)
	}
	if _, _, found, _ := agentio.FindLiveSession([]agentio.Agent{n}, 12); found {
		t.Fatal("lock files name no pid")
	}
}

func TestDoctorInventory(t *testing.T) {
	a, h := setup(t)
	checks := a.Checks(context.Background(), h)
	ids := map[agentio.CheckID]agentio.CheckStatus{}
	for _, c := range checks {
		ids[c.ID] = c.Status
	}
	if ids[agentio.CheckVersionTested] != agentio.CheckWarn || ids[CheckConfigParse] != agentio.CheckOK || ids[agentio.CheckHome] != agentio.CheckOK {
		t.Fatalf("checks %+v", checks)
	}
	inv, err := a.Inventory(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Unknown) != 0 {
		t.Fatalf("unknown entries %v", inv.Unknown)
	}
}

func TestNeutral(t *testing.T) {
	a, h := setup(t)
	n := a.Neutral()
	agenttest.Conformance(t, n, h.Neutral())
	if r, ok := agentio.Unsupported(n, agentio.CapConversationMover); !ok || !strings.Contains(r, "SQLite") {
		t.Fatal("mover reason")
	}
	if _, ok := agentio.HistoryMoverOf(n); ok {
		t.Fatal("no history mover")
	}
	if found, err := a.LiveSessions(h); err != nil || found != nil {
		t.Fatal("the fixture holds only .coordination.lock: no live sessions")
	}
	claudeHome := agentio.NewHome("claude", h.Dir(), true, "default")
	if _, err := agentio.IdentityOf(n); !err {
		t.Fatal("identity capability")
	}
	idr, _ := agentio.IdentityOf(n)
	if _, err := idr.Identity(claudeHome); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatal("foreign home")
	}
}

func TestTOMLFlatten(t *testing.T) {
	src := []byte(`a.b."c.d" = 1
arr = [
  "x", # comment
  "y",
]
tbls = [{ name = "n1" }, { name = "n2", source = "s" }]
[[outer]]
k = 1
[[outer.inner]]
v = "first"
[[outer]]
k = 2
[[outer.inner]]
v = "second"
[t]
inline = { p = { q = 'lit' } }
empty = {}
`)
	entries, err := parseTOML(src)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.key()] = e.val.raw
	}
	want := map[string]string{
		`a.b."c.d"`:         "1",
		"tbls.0.name":       `"n1"`,
		"tbls.1.source":     `"s"`,
		"outer.0.k":         "1",
		"outer.0.inner.0.v": `"first"`,
		"outer.1.inner.0.v": `"second"`,
		"t.inline.p.q":      "'lit'",
		"t.empty":           "{}",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if !strings.Contains(got["arr"], "# comment") {
		t.Errorf("array raw text keeps the source: %q", got["arr"])
	}
	if _, err := parseTOML([]byte("a = ")); err == nil {
		t.Error("syntax error must fail")
	}
}

func TestHooks(t *testing.T) {
	a, h := setup(t)
	hk, err := a.HookConfig(h)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []HookEvent{HookPermissionRequest, HookPreToolUse, HookSessionStart, HookStop, HookUserPromptSubmit} {
		if len(hk.Events[ev]) != 1 {
			t.Fatalf("event %s: %+v", ev, hk.Events[ev])
		}
	}
	pre := hk.Events[HookPreToolUse][0]
	if pre.Matcher.V != "shell" || len(pre.Hooks) != 2 || pre.Hooks[0].StatusMessage.V != "checking command" {
		t.Fatalf("pre tool %+v", pre)
	}
	mcp := pre.Hooks[1]
	if mcp.Type != HookTypeMCPTool || mcp.Server.V != "audit" || mcp.Tool.V != "record" || !strings.Contains(string(mcp.Input), `level = "info"`) {
		t.Fatalf("mcp_tool handler %+v", mcp)
	}
	stop := hk.Events[HookStop][0].Hooks[0]
	if !stop.Async.V || stop.Unknown["futureHandlerKey"] != `"x"` {
		t.Fatalf("stop handler %+v", stop)
	}
	if ups := hk.Events[HookUserPromptSubmit][0].Hooks[0]; !ups.AdditionalContextLimit.Set || ups.AdditionalContextLimit.V != 0 {
		t.Fatal("additionalContextLimit = 0 is set, not absent")
	}
	if pr := hk.Events[HookPermissionRequest][0]; pr.Matcher.V != "Bash" || pr.Hooks[0].Timeout.V != 10*time.Second {
		t.Fatalf("permission request %+v", pr)
	}
	st, err := a.HookState(h)
	if err != nil || len(st) != 2 {
		t.Fatalf("state %v %v", st, err)
	}
	if s := st["/home/test/.codex/hooks.json:stop:0:0"]; !s.Enabled.Set || s.Enabled.V || s.TrustedHash.V != "sha256:test2" {
		t.Fatalf("stop state %+v", s)
	}
	hf, ok, err := a.HooksFile(h)
	if err != nil || !ok || hf.Description.V != "test hooks" {
		t.Fatalf("hooks.json %+v %v", hf, err)
	}
	lint := hf.Events[HookPreToolUse][0]
	if lint.Matcher.V != "apply_patch" || lint.Hooks[0].CommandWindows.V != `C:\test\lint.cmd` || lint.Hooks[0].UnknownJSON["newKey"] == nil {
		t.Fatalf("hooks.json handler %+v", lint)
	}
	if hf.Events[HookSessionStart][0].Hooks[0].Timeout.V != 5*time.Second {
		t.Fatal("hooks.json timeout")
	}
}

func TestProfilesAndConflictCheck(t *testing.T) {
	a, h := setup(t)
	ps, err := a.Profiles(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].Name != "ollama-launch" || ps[0].Kind != ProfileLegacy || !ps[0].HasTable || ps[1].Name != "work" || ps[1].Kind != ProfileV2 {
		t.Fatalf("profiles %+v", ps)
	}
	cfg, _ := a.ConfigFile(h)
	lp := cfg.Profiles["ollama-launch"]
	if lp.OpenAIBaseURL.V != "http://localhost:11434/v1" || lp.ForcedLoginMethod.V != LoginAPI || len(lp.Unknown) != 0 {
		t.Fatalf("legacy profile keys %+v", lp)
	}
	mp := cfg.ModelProviders["ollama-launch"]
	if mp.Name.V != "Ollama" || mp.BaseURL.V != "http://localhost:11434/v1" {
		t.Fatalf("model provider %+v", mp)
	}
	var found bool
	for _, c := range a.Checks(context.Background(), h) {
		if c.ID == CheckProfileConflict {
			found = c.Status == agentio.CheckWarn && strings.Contains(c.Message, "ollama-launch") && strings.Contains(c.Message, "codex -p NAME") && strings.Contains(c.Message, "0.131.0")
		}
	}
	if !found {
		t.Fatal("profile-conflict warn check missing")
	}
}

// TestHomeLoginScrubsCodexAPIKey guards invariant 9 for Codex: an inherited
// CODEX_API_KEY beats the home's stored login in `codex exec`, so every
// launch removes it. OPENAI_API_KEY is kept: Codex auth never reads it and
// a model provider may name it as its key.
func TestHomeLoginScrubsCodexAPIKey(t *testing.T) {
	a, h := setup(t, agenttest.WithEnv("PATH=/usr/bin", EnvCodexAPIKey+"=inherited", EnvAPIKey+"=provider-key"))
	cur := h.Target()
	cur.Version = agentio.MustVersion(fixtureVersion)
	execCmd, err := a.Exec(cur, ExecOptions{LaunchOptions: LaunchOptions{PromptFromStdin: true}})
	if err != nil {
		t.Fatal(err)
	}
	tui, err := a.New(cur, LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for label, c := range map[string]agentio.Command{"exec": execCmd, "tui": tui} {
		if v, ok := agentio.EnvLookup(c.Env, EnvCodexAPIKey); ok {
			t.Errorf("%s kept inherited %s=%q", label, EnvCodexAPIKey, v)
		}
		if v, _ := agentio.EnvLookup(c.Env, EnvAPIKey); v != "provider-key" {
			t.Errorf("%s dropped %s", label, EnvAPIKey)
		}
	}
}

// TestSystemConfigFollowsHost guards that /etc/codex goes through Host: a
// fixture host (no SystemRoot) lists no system layers, so the test
// machine's /etc/codex cannot change fixture output, and a host whose
// SystemRoot is a temp dir reads the system config.toml placed there.
func TestSystemConfigFollowsHost(t *testing.T) {
	a, h := setup(t)
	layers, err := a.ConfigLayers(h, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range layers {
		if l.Kind == LayerSystem || l.Kind == LayerLegacyManaged {
			t.Errorf("fixture host listed system layer %s %s", l.Kind, l.File)
		}
	}
	root := t.TempDir()
	agenttest.WriteFile(t, root, systemConfigRel, "model = \"from-etc\"\n")
	a, h = setup(t, agenttest.WithSystem("linux", root))
	eff, err := a.Effective(h, EffectiveQuery{IncludeSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	last := len(eff.Layers) - 1
	if last < 0 || eff.Layers[last].Kind != LayerSystem || !eff.Layers[last].Exists || eff.Layers[last].File != filepath.Join(root, "etc", "codex", "config.toml") {
		t.Fatalf("system layer (lowest precedence, last) %+v", eff.Layers)
	}
	win := New(agentio.Host{GOOS: "windows", SystemRoot: "R"})
	if sys, managed := win.systemConfigPaths(); sys != filepath.Join("R", "ProgramData", "OpenAI", "Codex", "config.toml") || managed != "" {
		t.Errorf("windows system paths %q %q", sys, managed)
	}
}

// TestConfigLayersHighestFirst guards the one layer order shared with
// Claude and agentio.ConfigReader: highest precedence first, so the legacy
// managed file (wins over everything Codex reads) comes first and the
// system config.toml (loses to everything) comes last, in ConfigLayers,
// EffectiveConfig.Layers and the neutral view.
func TestConfigLayersHighestFirst(t *testing.T) {
	root := t.TempDir()
	a, h := setup(t, agenttest.WithSystem("linux", root))
	layers, err := a.ConfigLayers(h, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) < 3 || layers[0].Kind != LayerLegacyManaged || layers[len(layers)-1].Kind != LayerSystem {
		t.Fatalf("ConfigLayers order %+v, want legacy-managed first and system last", layers)
	}
	eff, err := a.Effective(h, EffectiveQuery{IncludeSystem: true, Overrides: []Override{{Path: "model", Value: TOMLRaw(`"cli"`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if eff.Layers[0].Kind != LayerLegacyManaged || eff.Layers[1].Kind != LayerSession || eff.Config.Model.V != "cli" {
		t.Fatalf("Effective order %+v model %q", eff.Layers, eff.Config.Model.V)
	}
	n, err := a.Neutral().(agentio.ConfigReader).ConfigLayers(h.Neutral(), "")
	if err != nil {
		t.Fatal(err)
	}
	if n[len(n)-1].Path != filepath.Join(root, "etc", "codex", "config.toml") {
		t.Fatalf("neutral order %+v, want the system file last", n)
	}
}
