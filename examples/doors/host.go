package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/agentio"
)

// Fixture locations; the example copies the Claude user home into a temp
// dir because the Writer and HomeLinker steps change files.
const (
	claudeFixture = "claude/testdata/homes/2.1.325/userhome"
	fixtureUser   = "user"
	dirPerm       = 0o700
	filePerm      = 0o600
	noBinaries    = "/nonexistent-bin"
)

// Canned process answers: `claude --version` and `codex --version` print
// what the real binaries print; anything else exits 1.
const (
	versionFlag      = "--version"
	claudeVersionOut = "2.1.325 (Claude Code)\n"
	codexVersionOut  = "codex-cli 0.162.1\n"
	aiderVersionOut  = "aider 0.86.1\n"
)

// Fake probe answer: anthropic-ratelimit-unified-* headers as a real
// /v1/messages reply carries them.
const (
	messagesPath = "/v1/messages"
	probeHeader  = "anthropic-ratelimit-unified-"
	probeUtil5h  = "0.64"
)

// errUnknownProgram is the canned runner's answer for an unknown program.
var errUnknownProgram = errors.New("example: no canned answer for this program")

// fixtureNow is the fixed clock; it also names the backup files.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// recordingExec is a ProcessRunner with canned `--version` answers that
// records every command it is asked to run, so the example can show that
// only Runner calls reach Host.Exec.
type recordingExec struct {
	mu    sync.Mutex
	calls []string
}

// Run records c and returns the canned answer for its binary. Pure.
func (r *recordingExec) Run(_ context.Context, c agentio.Command, _ io.Reader) (agentio.ProcessResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c.String())
	if len(c.Args) != 1 || c.Args[0] != versionFlag {
		return agentio.ProcessResult{ExitCode: 1, Stderr: []byte("unknown command")}, nil
	}
	switch filepath.Base(c.Path) {
	case "claude":
		return agentio.ProcessResult{Stdout: []byte(claudeVersionOut)}, nil
	case "codex":
		return agentio.ProcessResult{Stdout: []byte(codexVersionOut)}, nil
	case "aider":
		return agentio.ProcessResult{Stdout: []byte(aiderVersionOut)}, nil
	}
	return agentio.ProcessResult{}, errUnknownProgram
}

// Calls returns the recorded command lines. Pure.
func (r *recordingExec) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// fakeProbeAPI answers POST /v1/messages with rate-limit headers; anything
// else is a 404.
func fakeProbeAPI() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != messagesPath {
			http.NotFound(w, r)
			return
		}
		reset := strconv.FormatInt(time.Date(2026, 11, 13, 16, 0, 0, 0, time.UTC).Unix(), 10)
		w.Header().Set(probeHeader+"5h-utilization", probeUtil5h)
		w.Header().Set(probeHeader+"5h-reset", reset)
		// A failed write means the client hung up; a handler has no caller to
		// report it to, so the result is dropped.
		_, _ = w.Write([]byte(`{}`))
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

// setup copies the Claude fixture user home into tmp and returns the Host:
// exec is the recording runner, client the fake API's client. Disk · Write.
func setup(tmp string, exec *recordingExec, client *http.Client) (agentio.Host, error) {
	root, err := repoRoot()
	if err != nil {
		return agentio.Host{}, err
	}
	if err := copyTree(filepath.Join(root, claudeFixture), tmp); err != nil {
		return agentio.Host{}, err
	}
	return agentio.Host{
		UserHome: tmp,
		User:     fixtureUser,
		Env:      []string{"HOME=" + tmp, "PATH=" + noBinaries},
		Now:      fixtureNow,
		HTTP:     client,
		Exec:     exec,
	}, nil
}

// tidy replaces the temporary user home with ~ in printed paths. Pure.
func tidy(h agentio.Host, s string) string { return strings.ReplaceAll(s, h.UserHome, "~") }
