package builtin

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/opencode"
	"github.com/ubgo/agentio/pi"
)

// TestIsHome_SearchedFolders: a folder found outside every agent's own
// names (a --search hit) is a home only of the agents whose distinctive
// markers it holds. A file name several tools write (settings.json,
// sessions/, auth.json, projects/, tmp/, config.toml, version.json) never
// makes it a home on its own, so one folder is never several agents'
// home because of one shared name.
func TestIsHome_SearchedFolders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	agents, err := New(agenttest.NewHost(t))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		entries []string // files; a trailing "/" makes a dir
		want    []agentio.ID
	}{
		"settings.json only": {[]string{"settings.json"}, nil},
		"sessions/ only":     {[]string{"sessions/"}, nil},
		"auth.json only":     {[]string{"auth.json"}, nil},
		"projects/ only":     {[]string{"projects/"}, nil},
		"tmp/ only":          {[]string{"tmp/"}, nil},
		"config.toml only":   {[]string{"config.toml"}, nil},
		"version.json only":  {[]string{"version.json"}, nil},
		"all generic names":  {[]string{"settings.json", "sessions/", "auth.json", "projects/", "tmp/", "config.toml", "version.json"}, nil},
		// Layouts of the fixture homes, found by search.
		"claude home":   {[]string{".claude.json", "projects/", "sessions/", "settings.json"}, []agentio.ID{claude.ID}},
		"claude linux":  {[]string{".credentials.json", "settings.json"}, []agentio.ID{claude.ID}},
		"codex home":    {[]string{"config.toml", "auth.json", "sessions/", "installation_id", "version.json"}, []agentio.ID{codex.ID}},
		"codex state":   {[]string{"state_5.sqlite"}, []agentio.ID{codex.ID}},
		"gemini home":   {[]string{"projects.json", "settings.json", "tmp/"}, []agentio.ID{gemini.ID}},
		"gemini oauth":  {[]string{"oauth_creds.json", "google_accounts.json"}, []agentio.ID{gemini.ID}},
		"pi home":       {[]string{"auth.json", "models-store.json", "sessions/"}, []agentio.ID{pi.ID}},
		"opencode home": {[]string{"opencode.db", "storage/", "auth.json", "log/"}, []agentio.ID{opencode.ID}},
		"storage/ only": {[]string{"storage/"}, nil},
		"two distinct":  {[]string{".claude.json", "models-store.json"}, []agentio.ID{claude.ID, pi.ID}},
		"no marker":     {[]string{"README.md"}, nil},
		"empty folder":  {nil, nil},
		"unrelated dir": {[]string{"other/"}, nil},
	} {
		dir := filepath.Join(root, name)
		makeEntries(t, dir, tc.entries)
		var got []agentio.ID
		for _, a := range agents.All() {
			ok, err := a.IsHome(dir)
			if err != nil {
				t.Fatalf("%s: %s: %v", name, a.ID(), err)
			}
			if ok {
				got = append(got, a.ID())
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: homes of %v, want %v", name, got, tc.want)
		}
	}
}

// TestIsHome_NamedFoldersKeepGenericMarkers: in the folders an agent's own
// names select, a generic marker alone still makes a home (a user who
// wrote settings.json before the first run has a home); a caller-named
// Codex home with only auth.json is accepted by HomeAt.
func TestIsHome_NamedFoldersKeepGenericMarkers(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t)
	for _, rel := range []string{".claude/settings.json", ".claude-work/settings.json", ".gemini/settings.json", ".pi/agent/settings.json"} {
		agenttest.WriteFile(t, host.UserHome, rel, "{}")
	}
	agents, err := New(host)
	if err != nil {
		t.Fatal(err)
	}
	for id, rel := range map[agentio.ID]string{claude.ID: ".claude", gemini.ID: ".gemini", pi.ID: ".pi/agent"} {
		a, _ := agents.ByID(id)
		if ok, err := a.IsHome(filepath.Join(host.UserHome, rel)); !ok || err != nil {
			t.Errorf("%s default home with settings.json only: %v %v", id, ok, err)
		}
	}
	a, _ := agents.ByID(claude.ID)
	if ok, err := a.IsHome(filepath.Join(host.UserHome, ".claude-work")); !ok || err != nil {
		t.Errorf("claude extra home with settings.json only: %v %v", ok, err)
	}
	named := filepath.Join(t.TempDir(), "codex-work")
	agenttest.WriteFile(t, named, "auth.json", "{}")
	if _, err := codex.New(host).HomeAt(named); err != nil {
		t.Errorf("caller-named codex home with auth.json only: %v", err)
	}
}

// makeEntries creates dir and the given files and dirs (trailing "/") in it.
func makeEntries(t *testing.T, dir string, entries []string) {
	t.Helper()
	agenttest.WriteFile(t, dir, ".keep-dir", "")
	for _, e := range entries {
		if d, ok := strings.CutSuffix(e, "/"); ok {
			agenttest.WriteFile(t, filepath.Join(dir, d), ".keep", "")
			continue
		}
		agenttest.WriteFile(t, dir, e, "{}")
	}
}
