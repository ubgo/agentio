package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestNilHTTPIsUnsupported guards "no hidden state": every Claude network
// path on a Host without an HTTP client returns ErrUnsupported instead of
// building a private client that would reach the real network.
func TestNilHTTPIsUnsupported(t *testing.T) {
	t.Parallel()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithHTTP(nil))
	ctx := context.Background()
	calls := map[string]func() error{
		"usage": func() error { _, err := a.Usage(ctx, a.DefaultHome()); return err },
		"subscription": func() error {
			_, err := a.Subscription(ctx, a.DefaultHome())
			return err
		},
		"probe": func() error {
			_, err := a.Prober().TokenUsage(ctx, agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("sk-ant-oat01-x")})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, agentio.ErrUnsupported) {
			t.Errorf("%s with nil Host.HTTP: %v, want ErrUnsupported", name, err)
		}
	}
}

// TestHomeLoginScrubsCredentialEnv guards invariant 9 for a home login: an
// inherited ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN or
// CLAUDE_CODE_OAUTH_TOKEN would silently override the home's stored login,
// so Launch and LoginCommand remove every one of them.
func TestHomeLoginScrubsCredentialEnv(t *testing.T) {
	t.Parallel()
	env := []string{"PATH=/bin", "KEEP=1"}
	for _, name := range credentialEnv() {
		env = append(env, name+"=inherited")
	}
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithEnv(env...))
	cmd, err := a.Launch(a.DefaultHome().Target(), LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	login := a.LoginCommand(a.DefaultHome(), LoginClaudeAI, "")
	for label, c := range map[string]agentio.Command{"launch": cmd, "login": login} {
		for _, name := range []string{APIKeyEnv, AuthTokenEnv, OAuthTokenEnv} {
			if v, ok := agentio.EnvLookup(c.Env, name); ok {
				t.Errorf("%s kept inherited %s=%q", label, name, v)
			}
		}
		if v, _ := agentio.EnvLookup(c.Env, "KEEP"); v != "1" {
			t.Errorf("%s dropped an unrelated variable", label)
		}
	}
}

// TestFixtureHostHasNoSystemPaths guards that system and managed paths go
// through Host: a fixture host (no GOOS, no SystemRoot) computes no managed
// path at all, so a managed-settings.json, CLAUDE.md or managed-mcp.json in
// the test machine's /etc/claude-code cannot change fixture output.
func TestFixtureHostHasNoSystemPaths(t *testing.T) {
	t.Parallel()
	a, dir := fixtureAgent(t, fixtureVersions(t)[0])
	if sp := a.SystemPaths(); sp != (SystemPaths{}) {
		t.Fatalf("fixture host SystemPaths = %+v, want zero", sp)
	}
	h := a.DefaultHome()
	layers, _ := a.ManagedSettings(h)
	for _, l := range layers {
		if l.Source.Kind != SourceRemote {
			t.Errorf("fixture host read managed layer %s", l.Source.Path)
		}
	}
	mem, err := a.MemoryFiles(h, Project{Dir: filepath.Join(dir, "project")}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mem {
		if m.Kind == MemoryManaged {
			t.Errorf("fixture host read managed memory %s", m.Path)
		}
	}
	servers, err := a.MCPServers(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range servers {
		if s.Scope == MCPScopeManaged {
			t.Errorf("fixture host read managed MCP server %s", s.Name)
		}
	}
}

// TestSystemPathsFollowHost: SystemPaths come from Host.GOOS and
// Host.SystemRoot (never runtime.GOOS), and the managed readers read the
// files a test places under that root.
func TestSystemPathsFollowHost(t *testing.T) {
	t.Parallel()
	for goos, want := range map[string]string{
		"linux":   filepath.Join("R", "etc", "claude-code"),
		"darwin":  filepath.Join("R", "Library", "Application Support", "ClaudeCode"),
		"windows": filepath.Join("R", "Program Files", "ClaudeCode"),
	} {
		a := New(agentio.Host{GOOS: goos, SystemRoot: "R"})
		if got := a.SystemPaths().ManagedDir; got != want {
			t.Errorf("%s ManagedDir = %q, want %q", goos, got, want)
		}
	}
	root := t.TempDir()
	managed := filepath.Join(root, "etc", "claude-code")
	agenttest.WriteFile(t, managed, managedSettingsFile, `{"model":"opus"}`)
	agenttest.WriteFile(t, managed, memoryFileName, "managed rules")
	agenttest.WriteFile(t, managed, managedMCPFile, `{"mcpServers":{"corp":{"command":"corp-mcp"}}}`)
	a, dir := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithSystem("linux", root))
	h := a.DefaultHome()
	layers, _ := a.ManagedSettings(h)
	found := false
	for _, l := range layers {
		found = found || (l.Source.Kind == SourceManagedFile && l.Settings.Model == "opus")
	}
	if !found {
		t.Errorf("managed settings under SystemRoot not read: %+v", layers)
	}
	mem, err := a.MemoryFiles(h, Project{Dir: filepath.Join(dir, "project")}, false)
	if err != nil || len(mem) == 0 || mem[0].Kind != MemoryManaged {
		t.Errorf("managed CLAUDE.md under SystemRoot not first: %+v %v", mem, err)
	}
	servers, err := a.MCPServers(h, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundMCP := false
	for _, s := range servers {
		foundMCP = foundMCP || (s.Scope == MCPScopeManaged && s.Name == "corp")
	}
	if !foundMCP {
		t.Errorf("managed MCP under SystemRoot not read")
	}
}

// TestRetentionClockSkew: a .last-cleanup stamp after the Host clock (the
// fixture's is 19:30, FixedNow is 12:00 the same day) reports clock skew as
// a warning, never a negative age such as "-7 h ago".
func TestRetentionClockSkew(t *testing.T) {
	t.Parallel()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0])
	var msg string
	var status agentio.CheckStatus
	for _, c := range a.Doctor(context.Background(), a.DefaultHome(), nil, DoctorOptions{}) {
		if c.ID == CheckRetention {
			msg, status = c.Message, c.Status
		}
	}
	if strings.Contains(msg, "-") && strings.Contains(msg, "h ago") {
		t.Fatalf("negative age in %q", msg)
	}
	if !strings.Contains(msg, "clock skew") || status != agentio.CheckWarn {
		t.Fatalf("retention = %s %q, want a clock skew warning", status, msg)
	}
}

// TestConfigLayersHighestFirst guards the layer order shared with Codex
// and agentio.ConfigReader: highest precedence first (local, project, user
// for a project folder).
func TestConfigLayersHighestFirst(t *testing.T) {
	t.Parallel()
	a, dir := fixtureAgent(t, fixtureVersions(t)[0])
	ls, err := a.Neutral().(agentio.ConfigReader).ConfigLayers(a.DefaultHome().Neutral(), filepath.Join(dir, "project"))
	if err != nil {
		t.Fatal(err)
	}
	var scopes []agentio.ConfigScope
	for _, l := range ls {
		scopes = append(scopes, l.Scope)
	}
	want := []agentio.ConfigScope{agentio.ConfigLocal, agentio.ConfigProject, agentio.ConfigUser}
	if len(scopes) < len(want) || !slices.Equal(scopes[len(scopes)-len(want):], want) {
		t.Fatalf("scopes %v, want ... %v", scopes, want)
	}
}

// TestUpdateCommandResolvesBinary: UpdateCommand resolves claude on Host
// PATH (a walk, never a shell, never the bare name) and is
// agentio.ErrNotInstalled when PATH has none.
func TestUpdateCommandResolvesBinary(t *testing.T) {
	t.Parallel()
	a, dir := fixtureAgent(t, fixtureVersions(t)[0])
	if _, err := a.UpdateCommand(); !errors.Is(err, agentio.ErrNotInstalled) {
		t.Fatalf("no claude on PATH: %v", err)
	}
	exe := agenttest.ExeName(Binary)
	agenttest.WriteFile(t, filepath.Join(dir, "bin"), exe, "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "bin", exe), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd, err := a.UpdateCommand()
	if err != nil || cmd.Path != filepath.Join(dir, "bin", exe) || !slices.Equal(cmd.Args, []string{subUpdate}) {
		t.Fatalf("update %s %v %v", cmd.Path, cmd.Args, err)
	}
}
