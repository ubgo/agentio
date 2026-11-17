package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// snippet:start match-sentinel
// explain maps an agentio error to what a caller tells the user. Branch
// with errors.Is: vendor packages wrap the sentinels with their own
// wording, so the message text is not stable but the sentinel is.
func explain(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, agentio.ErrNotInstalled):
		return "install the CLI first"
	case errors.Is(err, agentio.ErrNotLoggedIn):
		return "log in with the vendor CLI"
	case errors.Is(err, agentio.ErrExpired):
		return "run the vendor CLI once to refresh its login"
	case errors.Is(err, agentio.ErrScopeLimited):
		return "this credential can run the agent but not read usage (not a login problem)"
	case errors.Is(err, agentio.ErrBilledPerToken):
		return "billed per token: there are no usage windows"
	case errors.Is(err, agentio.ErrRateLimited):
		return "rate-limited: try again later"
	case errors.Is(err, agentio.ErrUnsupported):
		return "not supported by this agent"
	case errors.Is(err, agentio.ErrInvalidSpec), errors.Is(err, agentio.ErrInvalidID):
		return "fix the agent config"
	case errors.Is(err, agentio.ErrNotFound):
		return "no such item"
	}
	return "unexpected: " + err.Error()
}

// matchSentinels produces real errors from the library and explains each.
func matchSentinels(p printer) error {
	_, notInstalled := agentio.LookPath("", "claude")
	_, badID := agentio.ParseID("My Agent")
	_, badSpec := agentio.NewCore(agentio.Spec{ID: "toy"}, agentio.Host{})
	// A config agent whose Spec has no PromptArgs refuses a prompt with
	// ErrUnsupported instead of dropping it.
	toy, err := agentio.FromSpec(agentio.Spec{ID: "toy", Binary: "toy", NewArgs: []string{"{args}"}}, agentio.Host{})
	if err != nil {
		return err
	}
	_, unsupported := toy.Launch(toy.DefaultHome().Target(), agentio.LaunchRequest{Prompt: "hi"})
	byHand := agentio.UnsupportedError(codex.ID, "history move", "conversations are indexed in SQLite")
	expired := fmt.Errorf("claude: credentials: %w", agentio.ErrExpired)
	for _, err := range []error{notInstalled, badID, badSpec, unsupported, byHand, expired} {
		p.line(fmt.Sprintf("error: %v\n  -> %s", err, explain(err)))
	}
	return nil
}

// snippet:end match-sentinel

// snippet:start rate-limits
// rateLimits words a 429 the way every vendor client does: RateLimitError
// reads Retry-After (seconds or an HTTP date relative to now) and wraps
// ErrRateLimited, never a login error. Pure.
func rateLimits(p printer) error {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, retry := range []string{"120", now.Add(90 * time.Second).UTC().Format(http.TimeFormat), ""} {
		h := http.Header{}
		if retry != "" {
			h.Set("Retry-After", retry)
		}
		err := agentio.RateLimitError(h, now)
		p.line(fmt.Sprintf("429 retry-after=%q: %v (rate-limited=%t)", retry, err, errors.Is(err, agentio.ErrRateLimited)))
	}
	if d, ok := agentio.RetryAfter("30", now); ok {
		p.line(fmt.Sprintf("RetryAfter(30) = %s", d))
	}
	return nil
}

// snippet:end rate-limits
