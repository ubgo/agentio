// Package pick chooses, among several agent homes, the one with the most
// headroom: given each home's usage (live or cached), it scores every
// eligible home with a Policy and returns the lowest score, plus everything
// needed to explain the choice.
//
// It is policy, not facts, so it lives beside the core and the core never
// imports it (an architecture test guards that). It is pure: no I/O, no
// clock (pass WithNow to describe the age of a cached reading). Reading
// the usage is the caller's job (agentio.UsageOf costs one network request
// per home and may 429; agentio.CachedUsageOf reads disk only), so a loop
// over homes never spends quota by accident.
//
// The default policy, MaxPercent, ports agx's auto-pick: a home's score is
// the highest percent across its windows, because any single window at its
// cap blocks work; the lowest score wins; ties keep input order.
package pick

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Candidate is one home with the result of reading its usage.
type Candidate struct {
	// Home is the home the usage belongs to; the chosen one is what to
	// launch with (Home.Target()).
	Home agentio.Home
	// Usage is the reading (agentio.UsageOf or CachedUsageOf). Its Source
	// and ObservedAt are kept on the result, so Explain can say "cached
	// 4m ago".
	Usage agentio.Usage
	// Err is the usage read error; a non-nil Err skips the candidate with
	// SkipUsageError (agentio.ErrBilledPerToken included: an API-key login
	// has no windows to compare).
	Err error
	// Billing is what the caller knows about how the home is paid for
	// (a profile setting, Target.Billing, the identity's billing). It, or
	// Usage.Billing, being agentio.BillingPerToken skips the candidate with
	// SkipPerToken: per-token billing has no headroom to compare.
	Billing agentio.Billing
}

// Policy scores a candidate: lower is better. ok false skips the candidate
// with SkipPolicy and the reason (which should say why in a few words,
// for Explain and the error). Score is only called for candidates without
// a usage error or per-token billing. Implementations must be pure.
type Policy interface {
	// Name labels the policy in Explain when it is not the default.
	Name() string
	// Score is the candidate's score, or ok=false with a reason.
	Score(c Candidate) (score float64, ok bool, reason string)
}

// MaxPercent is the default policy: the highest percent across the
// candidate's windows (Usage.MaxPercent), because any window at its cap
// blocks work. A candidate with no windows scores 0. It never skips.
type MaxPercent struct{}

// policyMaxName is MaxPercent's name.
const policyMaxName = "max"

// Name is "max". Pure.
func (MaxPercent) Name() string { return policyMaxName }

// Score is Usage.MaxPercent(). Pure.
func (MaxPercent) Score(c Candidate) (float64, bool, string) { return c.Usage.MaxPercent(), true, "" }

// SkipReason says why a candidate was not eligible. Closed set.
type SkipReason uint8

// SkipReason values.
const (
	// SkipUsageError: reading the usage failed (Candidate.Err).
	SkipUsageError SkipReason = iota + 1
	// SkipPerToken: the home is billed per token (no windows).
	SkipPerToken
	// SkipPolicy: the policy refused the candidate (Score ok=false).
	SkipPolicy
)

// String names the reason. Pure.
func (r SkipReason) String() string {
	switch r {
	case SkipUsageError:
		return "usage-error"
	case SkipPerToken:
		return "per-token"
	case SkipPolicy:
		return "policy"
	}
	return "SkipReason(" + strconv.Itoa(int(r)) + ")"
}

// Skip is one ineligible candidate.
type Skip struct {
	Home   agentio.Home
	Reason SkipReason
	// Err is the cause: Candidate.Err, agentio.ErrBilledPerToken, or for
	// SkipPolicy an error matching ErrPolicySkip that carries the reason.
	Err error
}

// Scored is an eligible candidate with its score; Usage is kept for
// Explain and for the caller (windows, source, age).
type Scored struct {
	Home  agentio.Home
	Score float64
	Usage agentio.Usage
}

// Result is the decision and everything needed to explain it.
type Result struct {
	// Chosen is the eligible candidate with the lowest score (the first
	// in input order on a tie).
	Chosen Scored
	// Others are the remaining eligible candidates, best first; ties keep
	// input order.
	Others []Scored
	// Skipped are the ineligible candidates, in input order.
	Skipped []Skip
	// Policy is the name of the policy that scored them.
	Policy string
	// now is the WithNow time, for "cached 4m ago" in Explain.
	now time.Time
	// qualify is true when the candidates span several agents, so
	// Explain labels homes "claude:personal" instead of "personal".
	qualify bool
}

// Errors.
var (
	// ErrNoCandidate means no candidate was eligible; the error lists every
	// candidate's reason in input order.
	ErrNoCandidate = errors.New("pick: no eligible home")
	// ErrPolicySkip matches a Skip.Err made from a policy's refusal.
	ErrPolicySkip = errors.New("pick: refused by policy")
)

// Option adjusts Best.
type Option func(*options)

// options are Best's settings.
type options struct {
	policy Policy
	now    time.Time
}

// WithPolicy scores with p instead of MaxPercent.
func WithPolicy(p Policy) Option { return func(o *options) { o.policy = p } }

// WithNow gives the current time (Host.Now()), so Explain can describe the
// age of a cached reading; without it no age is shown. Best reads no clock.
func WithNow(now time.Time) Option { return func(o *options) { o.now = now } }

// Best applies the policy to cands (in the caller's order, which decides
// ties): candidates with a usage error or per-token billing are skipped
// (the error wins when both apply), the policy may skip more, and the
// lowest score wins. ErrNoCandidate, wrapping every reason, when none is
// eligible. Pure.
func Best(cands []Candidate, opts ...Option) (Result, error) {
	o := options{policy: MaxPercent{}}
	for _, opt := range opts {
		opt(&o)
	}
	res := Result{Policy: o.policy.Name(), now: o.now, qualify: spansAgents(cands)}
	var eligible []Scored
	for _, c := range cands {
		switch {
		case c.Err != nil:
			res.Skipped = append(res.Skipped, Skip{Home: c.Home, Reason: SkipUsageError, Err: c.Err})
		case c.Billing == agentio.BillingPerToken || c.Usage.Billing == agentio.BillingPerToken:
			res.Skipped = append(res.Skipped, Skip{Home: c.Home, Reason: SkipPerToken, Err: agentio.ErrBilledPerToken})
		default:
			score, ok, reason := o.policy.Score(c)
			if !ok {
				res.Skipped = append(res.Skipped, Skip{Home: c.Home, Reason: SkipPolicy, Err: fmt.Errorf("%w: %s", ErrPolicySkip, reason)})
				continue
			}
			eligible = append(eligible, Scored{Home: c.Home, Score: score, Usage: c.Usage})
		}
	}
	if len(eligible) == 0 {
		return res, fmt.Errorf("%w: %s", ErrNoCandidate, res.reasons())
	}
	// Stable selection: only a strictly lower score replaces the best, so
	// equal scores keep input order.
	best := 0
	for i := 1; i < len(eligible); i++ {
		if eligible[i].Score < eligible[best].Score {
			best = i
		}
	}
	res.Chosen = eligible[best]
	for i, e := range eligible {
		if i != best {
			res.Others = append(res.Others, e)
		}
	}
	sortScored(res.Others)
	return res, nil
}

// Explain renders the decision in one line, e.g.
// "auto → work (max 5%) over personal (max 77%)". A cached reading adds
// its age when WithNow was given ("max 5%, cached 4m ago"); a policy other
// than MaxPercent is named ("auto [reserve] → work (reserve 5) …").
// Pure.
func (r Result) Explain() string {
	var b strings.Builder
	b.WriteString("auto")
	if r.Policy != policyMaxName {
		b.WriteString(" [" + r.Policy + "]")
	}
	b.WriteString(" → " + r.describe(r.Chosen))
	if len(r.Others) > 0 {
		parts := make([]string, 0, len(r.Others))
		for _, o := range r.Others {
			parts = append(parts, r.describe(o))
		}
		b.WriteString(" over " + strings.Join(parts, ", "))
	}
	return b.String()
}

// describe is "work (max 5%)", "work (max 5%, cached 4m ago)" or, for
// another policy, "work (reserve 5)".
func (r Result) describe(s Scored) string {
	score := fmt.Sprintf("%s %.0f", r.Policy, s.Score)
	if r.Policy == policyMaxName {
		score += "%"
	}
	if age := cachedAge(s.Usage, r.now); age != "" {
		score += ", cached " + age + " ago"
	}
	return r.label(s.Home) + " (" + score + ")"
}

// label is the home's name, qualified by its agent when the candidates
// span several agents ("codex:personal").
func (r Result) label(h agentio.Home) string {
	if r.qualify {
		return string(h.Agent()) + ":" + h.Name()
	}
	return h.Name()
}

// reasons lists the skipped candidates in input order, "name: reason; …".
func (r Result) reasons() string {
	if len(r.Skipped) == 0 {
		return "no candidates"
	}
	parts := make([]string, 0, len(r.Skipped))
	for _, s := range r.Skipped {
		parts = append(parts, r.label(s.Home)+": "+s.Err.Error())
	}
	return strings.Join(parts, "; ")
}

// spansAgents reports candidates of more than one agent.
func spansAgents(cands []Candidate) bool {
	for _, c := range cands[min(1, len(cands)):] {
		if c.Home.Agent() != cands[0].Home.Agent() {
			return true
		}
	}
	return false
}

// cachedAge is "4m", "2h" or "3d" for a reading from a disk cache or a
// transcript when now is known; "" for a live reading or no clock.
func cachedAge(u agentio.Usage, now time.Time) string {
	if now.IsZero() || u.ObservedAt.IsZero() || (u.Source != agentio.UsageCache && u.Source != agentio.UsageTranscript) {
		return ""
	}
	d := now.Sub(u.ObservedAt)
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < day:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	}
	return strconv.Itoa(int(d/day)) + "d"
}

// day is a calendar-free 24h, enough for an age label.
const day = 24 * time.Hour

// sortScored orders by score ascending, keeping input order on ties
// (insertion sort: a handful of homes).
func sortScored(s []Scored) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Score < s[j-1].Score; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
