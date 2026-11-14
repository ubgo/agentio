package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Fixture locations inside the repository; the example copies them into a
// temporary user home (Claude's user home plus Codex as ~/.codex) so
// nothing it does can change the checked-in files.
const (
	claudeFixture = "claude/testdata/homes/2.1.325/userhome"
	codexFixture  = "codex/testdata/homes/0.162.1"
	codexHomeDir  = ".codex"
	workHomeDir   = ".claude-work"
	credsFile     = ".credentials.json"
	fixtureUser   = "user"
	dirPerm       = 0o700
	filePerm      = 0o600
	noBinaries    = "/nonexistent-bin"
)

// workToken is the placeholder login the example gives the work home (the
// fixture's work home is logged out). A shape only, never a real value.
const workToken = "sk-ant-" + "oat01-EXAMPLE-WORK-NOT-A-TOKEN"

// Fake usage endpoint: the recorded /api/oauth/usage shape, answering per
// login so the two Claude homes differ (personal is busy, work is not).
const (
	usagePath     = "/api/oauth/usage"
	personalUsage = `{"limits":[{"kind":"session","group":"session","percent":42,"severity":"normal","resets_at":"2026-11-13T15:00:00Z","scope":null,"is_active":true},{"kind":"weekly_all","group":"weekly","percent":77,"severity":"normal","resets_at":"2026-11-18T09:00:00Z","scope":null,"is_active":false}]}`
	workUsage     = `{"limits":[{"kind":"session","group":"session","percent":9,"severity":"normal","resets_at":"2026-11-13T15:00:00Z","scope":null,"is_active":true},{"kind":"weekly_all","group":"weekly","percent":5,"severity":"normal","resets_at":"2026-11-18T09:00:00Z","scope":null,"is_active":false}]}`
)

// fixtureNow is the fixed clock: the Claude logins are valid, the Codex
// one has expired, and every reading is stable across runs.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// fakeAPI serves the Claude usage endpoint only: the work token gets the
// quiet body, any other login the busy one. Anything else is a 404.
func fakeAPI() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != usagePath {
			http.NotFound(w, r)
			return
		}
		body := personalUsage
		if strings.HasSuffix(r.Header.Get("Authorization"), workToken) {
			body = workUsage
		}
		// A failed write means the client hung up; nothing to report it to.
		_, _ = w.Write([]byte(body))
	}))
}

// setup copies the fixture homes into tmp, logs the work home in with the
// placeholder token, and returns the Host on them with client as HTTP.
// Disk · Write.
func setup(tmp string, client *http.Client) (agentio.Host, error) {
	root, err := repoRoot()
	if err != nil {
		return agentio.Host{}, err
	}
	if err := copyTree(filepath.Join(root, claudeFixture), tmp); err != nil {
		return agentio.Host{}, err
	}
	if err := copyTree(filepath.Join(root, codexFixture), filepath.Join(tmp, codexHomeDir)); err != nil {
		return agentio.Host{}, err
	}
	if err := writeWorkLogin(filepath.Join(tmp, workHomeDir, credsFile)); err != nil {
		return agentio.Host{}, err
	}
	return agentio.Host{
		UserHome: tmp,
		User:     fixtureUser,
		Env:      []string{"HOME=" + tmp, "PATH=" + noBinaries},
		Now:      fixtureNow,
		HTTP:     client,
		Secrets:  noSecrets{},
	}, nil
}

// writeWorkLogin writes a .credentials.json in the recorded shape with the
// placeholder token, encoded with json.Marshal (never by string
// formatting). Disk · Write.
func writeWorkLogin(path string) error {
	type oauth struct {
		AccessToken      string   `json:"accessToken"`
		ExpiresAt        int64    `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		SubscriptionType string   `json:"subscriptionType"`
	}
	b, err := json.Marshal(map[string]oauth{"claudeAiOauth": {
		AccessToken: workToken, ExpiresAt: fixtureNow().Add(24 * time.Hour).UnixMilli(),
		Scopes: []string{"user:inference", "user:profile"}, SubscriptionType: "team",
	}})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, filePerm)
}

// noSecrets is a keychain with no entries.
type noSecrets struct{}

// Get finds nothing. Pure.
func (noSecrets) Get(context.Context, string, string) (agentio.Token, bool, error) {
	return agentio.Token{}, false, nil
}

// repoRoot walks up from the working directory to the folder holding the
// fixtures. Disk · Read.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, claudeFixture)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("example: %s not found above the working directory", claudeFixture)
		}
		dir = parent
	}
}

// copyTree copies the files and directories of src into dst. Disk · Write.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, filePerm)
	})
}
