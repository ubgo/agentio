// Package agenttest holds fakes for every Host input, a fixture Host
// builder and the conformance suite every agent must pass (SPEC §10). It
// is test support: it imports testing and is never used by library code.
package agenttest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ubgo/agentio"
)

// ErrFakeNotFound is returned by fakes for a missing entry (e.g. a cwd).
var ErrFakeNotFound = errors.New("agenttest: not found")

// ErrForbidden is returned by fakes that fail the test on use, after
// reporting the failure, so the code under test still sees an error.
var ErrForbidden = errors.New("agenttest: forbidden call")

// SecretKey addresses one FakeSecrets entry.
type SecretKey struct{ Service, Account string }

// FakeSecrets is an in-memory SecretStore. Get records every lookup so a
// test can check the named-account-first rule (invariant 3).
type FakeSecrets struct {
	mu      sync.Mutex
	Entries map[SecretKey]agentio.Token
	// Err, when set, is returned by every Get.
	Err   error
	Calls []SecretKey
}

var _ agentio.SecretStore = (*FakeSecrets)(nil)

// NewFakeSecrets returns an empty store. Pure.
func NewFakeSecrets() *FakeSecrets {
	return &FakeSecrets{Entries: map[SecretKey]agentio.Token{}}
}

// Put stores a value under (service, account). Pure.
func (f *FakeSecrets) Put(service, account, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Entries[SecretKey{service, account}] = agentio.NewToken(value)
}

// Get looks up the exact key; account "" matches an entry stored with
// account "" first, else any entry of the service (like `security -s`).
// Pure.
func (f *FakeSecrets) Get(_ context.Context, service, account string) (agentio.Token, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, SecretKey{service, account})
	if f.Err != nil {
		return agentio.Token{}, false, f.Err
	}
	if t, ok := f.Entries[SecretKey{service, account}]; ok {
		return t, true, nil
	}
	if account == "" {
		for k, t := range f.Entries {
			if k.Service == service {
				return t, true, nil
			}
		}
	}
	return agentio.Token{}, false, nil
}

// FakeProcesses is a fixed process table.
type FakeProcesses struct {
	List []agentio.Process
	Cwds map[int]string
	// Err, when set, is returned by Processes.
	Err error
}

var _ agentio.ProcessLister = (*FakeProcesses)(nil)

// Processes returns List. Pure.
func (f *FakeProcesses) Processes(context.Context) ([]agentio.Process, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return append([]agentio.Process(nil), f.List...), nil
}

// Cwd returns Cwds[pid]. Pure.
func (f *FakeProcesses) Cwd(_ context.Context, pid int) (string, error) {
	if d, ok := f.Cwds[pid]; ok {
		return d, nil
	}
	return "", fmt.Errorf("%w: cwd of %d", ErrFakeNotFound, pid)
}

// FailExec is a ProcessRunner that fails the test on use: read paths must
// never reach Host.Exec (invariant 1, SPEC §10).
type FailExec struct{ T testing.TB }

var _ agentio.ProcessRunner = FailExec{}

// Run reports a test error and returns ErrForbidden. Pure.
func (f FailExec) Run(_ context.Context, c agentio.Command, _ io.Reader) (agentio.ProcessResult, error) {
	f.T.Helper()
	f.T.Errorf("agenttest: unexpected process run: %s", c.String())
	return agentio.ProcessResult{}, ErrForbidden
}

// FakeExec is a ProcessRunner returning canned results keyed by argv
// (Command.String()), recording every call. Unmatched calls get Default.
type FakeExec struct {
	mu      sync.Mutex
	Results map[string]agentio.ProcessResult
	Default agentio.ProcessResult
	Calls   []agentio.Command
}

var _ agentio.ProcessRunner = (*FakeExec)(nil)

// Run returns the canned result. Pure.
func (f *FakeExec) Run(_ context.Context, c agentio.Command, _ io.Reader) (agentio.ProcessResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, c)
	if r, ok := f.Results[c.String()]; ok {
		return r, nil
	}
	return f.Default, nil
}

// failingTransport fails the test on any HTTP request.
type failingTransport struct{ t testing.TB }

// RoundTrip reports a test error. Pure.
func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Helper()
	f.t.Errorf("agenttest: unexpected HTTP %s %s", r.Method, r.URL.Host)
	return nil, ErrForbidden
}

// FailingHTTP returns a client that fails the test on any request: Disk
// paths (Identity, cached usage) must never reach the network. Pure.
func FailingHTTP(t testing.TB) *http.Client {
	return &http.Client{Transport: failingTransport{t: t}}
}

// FixedNow is the default fixture clock.
func FixedNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// Fixture host defaults.
const (
	fixtureUser = "tester"
	fixturePath = "/nonexistent-agenttest-bin"
)

// HostOption adjusts a fixture Host.
type HostOption func(*agentio.Host)

// WithUserHome sets the user home. Pure.
func WithUserHome(dir string) HostOption { return func(h *agentio.Host) { h.UserHome = dir } }

// WithEnv replaces the base env (HOME is not added for you). Pure.
func WithEnv(env ...string) HostOption { return func(h *agentio.Host) { h.Env = env } }

// WithNow sets the clock. Pure.
func WithNow(now func() time.Time) HostOption { return func(h *agentio.Host) { h.Now = now } }

// WithHTTP sets the HTTP client (an httptest server's client). Pure.
func WithHTTP(c *http.Client) HostOption { return func(h *agentio.Host) { h.HTTP = c } }

// WithSecrets sets the secret store. Pure.
func WithSecrets(s agentio.SecretStore) HostOption { return func(h *agentio.Host) { h.Secrets = s } }

// WithProcesses sets the process lister. Pure.
func WithProcesses(p agentio.ProcessLister) HostOption {
	return func(h *agentio.Host) { h.Processes = p }
}

// WithSystem sets Host.GOOS and Host.SystemRoot, so a test models system
// and managed locations (/etc/claude-code, /etc/codex) under root, a temp
// dir. NewHost leaves both empty: a fixture host has no system locations
// and never reads the test machine's /etc. Pure.
func WithSystem(goos, root string) HostOption {
	return func(h *agentio.Host) { h.GOOS, h.SystemRoot = goos, root }
}

// WithExec sets the process runner. Pure.
func WithExec(r agentio.ProcessRunner) HostOption { return func(h *agentio.Host) { h.Exec = r } }

// NewHost builds a fixture Host: UserHome = t.TempDir(), Env = HOME + a
// PATH with no binaries, User "tester", FixedNow, FailingHTTP, empty
// FakeSecrets and FakeProcesses, FailExec, and no GOOS or SystemRoot (no
// system locations; see WithSystem). Options override. Disk · Write
// (creates the temp dir).
func NewHost(t testing.TB, opts ...HostOption) agentio.Host {
	t.Helper()
	home := t.TempDir()
	h := agentio.Host{
		UserHome:  home,
		User:      fixtureUser,
		Env:       []string{"HOME=" + home, "PATH=" + fixturePath},
		Now:       FixedNow,
		HTTP:      FailingHTTP(t),
		Secrets:   NewFakeSecrets(),
		Processes: &FakeProcesses{},
		Exec:      FailExec{T: t},
	}
	for _, o := range opts {
		o(&h)
	}
	return h
}

// fixtureDirPerm and fixtureFilePerm are used when copying fixtures.
const (
	fixtureDirPerm  = 0o700
	fixtureFilePerm = 0o600
)

// CopyFixture copies the tree src (a testdata home) into dst, so tests
// that write never touch checked-in fixtures. Disk · Write.
func CopyFixture(t testing.TB, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, fixtureDirPerm)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, fixtureFilePerm)
	})
	if err != nil {
		t.Fatalf("agenttest: copy fixture %s: %v", src, err)
	}
}

// WriteFile writes data at dir/rel, creating parents. Disk · Write.
func WriteFile(t testing.TB, dir, rel, data string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), fixtureDirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), fixtureFilePerm); err != nil {
		t.Fatal(err)
	}
	return p
}
