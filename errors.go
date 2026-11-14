package agentio

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Neutral sentinels (SPEC §9.1). Callers branch on them with errors.Is;
// vendor packages wrap them with vendor wording (fmt.Errorf("%w: …")) and
// never replace them with look-alike messages.
var (
	// ErrNotLoggedIn means the home holds no usable credential.
	ErrNotLoggedIn = errors.New("agentio: not logged in")
	// ErrExpired means the local credential is past its expiry. It is never
	// refreshed: refreshing rotates the refresh token and logs the vendor
	// CLI out; one interactive vendor run refreshes it.
	ErrExpired = errors.New("agentio: login expired")
	// ErrUnauthorized means the server refused a credential that looked valid.
	ErrUnauthorized = errors.New("agentio: login rejected by the server")
	// ErrScopeLimited means the credential can run the agent but cannot read
	// account, plan or usage (was agx ErrInferenceOnly). Not a login problem:
	// callers must not suggest logging in again.
	ErrScopeLimited = errors.New("agentio: credential cannot read account or usage")
	// ErrBilledPerToken means no usage windows exist; an answer, not a failure.
	ErrBilledPerToken = errors.New("agentio: billed per token: no usage windows")
	// ErrRateLimited means a transient 429; never a login problem. See RateLimitError.
	ErrRateLimited = errors.New("agentio: rate-limited; try again shortly")
	// ErrUnsupported means the agent lacks the capability or launch field;
	// always wrapped with its name and reason.
	ErrUnsupported = errors.New("agentio: unsupported")
	// ErrForeignHome means a home of another agent was passed.
	ErrForeignHome = errors.New("agentio: home belongs to another agent")
	// ErrUnknownVersion means a vendor file declares a format version the
	// library has no sample for; only that reader fails, raw bytes stay reachable.
	ErrUnknownVersion = errors.New("agentio: unknown vendor format version")
	// ErrFeatureUnavailable means the installed vendor version provably lacks
	// a flag or file the call needs; vendor packages return a typed error
	// matching it.
	ErrFeatureUnavailable = errors.New("agentio: feature unavailable in the installed version")
	// ErrMoveBlocked means a move plan has blockers.
	ErrMoveBlocked = errors.New("agentio: move blocked")
	// ErrUnknownAgent means an ID lookup failed.
	ErrUnknownAgent = errors.New("agentio: unknown agent")
	// ErrNotInstalled means the agent's binary was not found on PATH.
	// Added beyond SPEC §9.1 so LookPath has a sentinel to branch on.
	ErrNotInstalled = errors.New("agentio: binary not found on PATH")
	// ErrDangerousNotAllowed means AutonomyFull was asked for without
	// Target.AllowDangerous. Added beyond SPEC §9.1: refusing is a policy
	// answer, not a missing capability, so it is not ErrUnsupported.
	ErrDangerousNotAllowed = errors.New("agentio: full autonomy needs Target.AllowDangerous")
	// ErrInvalidSpec means FromSpec / NewCore rejected a Spec.
	ErrInvalidSpec = errors.New("agentio: invalid spec")
	// ErrInvalidID means ParseID rejected a string.
	ErrInvalidID = errors.New("agentio: invalid agent id")
	// ErrNotFound means a named item (conversation, session, file) does not exist.
	ErrNotFound = errors.New("agentio: not found")
)

// UnsupportedError wraps ErrUnsupported with what is missing and why, so a
// caller can show the agent's own reason instead of a generic message.
// Pure.
func UnsupportedError(agent ID, what, reason string) error {
	if reason == "" {
		return fmt.Errorf("%w: %s: %s", ErrUnsupported, agent, what)
	}
	return fmt.Errorf("%w: %s: %s: %s", ErrUnsupported, agent, what, reason)
}

// retryAfterHeader is the standard HTTP header carrying a 429's back-off.
const retryAfterHeader = "Retry-After"

// RateLimitError wraps ErrRateLimited with the server's Retry-After hint
// when it sent one, so every vendor client words a 429 the same way. now is
// the caller's Host.Now() (an HTTP-date Retry-After is relative to it).
// Pure.
func RateLimitError(h http.Header, now time.Time) error {
	if d, ok := RetryAfter(h.Get(retryAfterHeader), now); ok {
		return fmt.Errorf("%w (retry after %s)", ErrRateLimited, d)
	}
	return ErrRateLimited
}

// RetryAfter parses a Retry-After value: delta-seconds or an HTTP date.
// Pure.
func RetryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now).Round(time.Second); d > 0 {
			return d, true
		}
	}
	return 0, false
}
