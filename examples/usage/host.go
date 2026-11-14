package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
	fixtureUser   = "user"
	dirPerm       = 0o700
	filePerm      = 0o600
	noBinaries    = "/nonexistent-bin"
)

// Fake vendor API. The bodies follow the recorded shapes of
// /api/oauth/usage and /api/oauth/profile; the probe answer carries the
// anthropic-ratelimit-unified-* headers a real /v1/messages reply has.
const (
	usagePath    = "/api/oauth/usage"
	profilePath  = "/api/oauth/profile"
	messagesPath = "/v1/messages"
	usageBody    = `{"limits":[{"kind":"session","group":"session","percent":42,"severity":"normal","resets_at":"2026-11-13T15:00:00Z","scope":null,"is_active":true},{"kind":"weekly_all","group":"weekly","percent":18,"severity":"normal","resets_at":"2026-11-18T09:00:00Z","scope":null,"is_active":false}],"extra_usage":{"is_enabled":false}}`
	profileBody  = `{"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x","billing_type":"stripe_subscription","subscription_status":"active"}}`
	probeHeader  = "anthropic-ratelimit-unified-"
	probeUtil5h  = "0.37"
	probeUtil7d  = "0.21"
)

// probeReset is the unix reset time the fake probe reports for both windows.
func probeReset() int64 { return time.Date(2026, 11, 13, 17, 0, 0, 0, time.UTC).Unix() }

// fixtureNow is the fixed clock: every reading is stable across runs.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// fakeAPI serves the three endpoints the Claude agent calls. Anything else
// is a 404, so a request that should not happen shows up as an error.
func fakeAPI() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case usagePath:
			// A failed write means the client hung up; a handler has no caller to
			// report it to, so the result is dropped.
			_, _ = w.Write([]byte(usageBody))
		case profilePath:
			_, _ = w.Write([]byte(profileBody))
		case messagesPath:
			reset := strconv.FormatInt(probeReset(), 10)
			w.Header().Set(probeHeader+"5h-utilization", probeUtil5h)
			w.Header().Set(probeHeader+"5h-reset", reset)
			w.Header().Set(probeHeader+"7d-utilization", probeUtil7d)
			w.Header().Set(probeHeader+"7d-reset", reset)
			w.Header().Set(probeHeader+"representative-claim", "five_hour")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
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

// setup copies the fixture homes into tmp and returns the Host on them,
// with client as Host.HTTP. Disk · Write.
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
	return agentio.Host{
		UserHome: tmp,
		User:     fixtureUser,
		Env:      []string{"HOME=" + tmp, "PATH=" + noBinaries},
		Now:      fixtureNow,
		HTTP:     client,
		Secrets:  noSecrets{},
	}, nil
}

// noSecrets is a keychain with no entries.
type noSecrets struct{}

// Get finds nothing. Pure.
func (noSecrets) Get(context.Context, string, string) (agentio.Token, bool, error) {
	return agentio.Token{}, false, nil
}
