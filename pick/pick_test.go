package pick

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
)

// The cases ported from agx's pick_test.go and policy_matrix_test.go keep
// agx's names and inputs; a profile is a home (named by Home.Name()) and
// agx's BillingAPI is agentio.BillingPerToken.

// testAgent is the agent the ported cases use.
const testAgent agentio.ID = "claude"

// cand builds a subscription candidate whose windows have pcts.
func cand(name string, err error, pcts ...float64) Candidate {
	return candFor(testAgent, name, err, pcts...)
}

// candFor is cand for another agent.
func candFor(agent agentio.ID, name string, err error, pcts ...float64) Candidate {
	u := agentio.Usage{Agent: agent, Billing: agentio.BillingSubscription, Source: agentio.UsageServer}
	for _, p := range pcts {
		u.Windows = append(u.Windows, agentio.Window{Percent: p})
	}
	return Candidate{Home: agentio.NewHome(agent, "/homes/"+string(agent)+"/"+name, false, name), Usage: u, Err: err}
}

// apiCand is a per-token candidate (agx BillingAPI).
func apiCand(name string, err error, pcts ...float64) Candidate {
	c := cand(name, err, pcts...)
	c.Billing = agentio.BillingPerToken
	return c
}

// names lists the homes' names.
func names(s []Scored) string {
	var out []string
	for _, x := range s {
		out = append(out, x.Home.Name())
	}
	return strings.Join(out, ",")
}

// skipNames lists the skipped homes' names in result order.
func skipNames(s []Skip) string {
	var out []string
	for _, x := range s {
		out = append(out, x.Home.Name())
	}
	return strings.Join(out, ",")
}

// agx TestBest.
func TestBest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cands []Candidate
		want  string
		score float64
	}{
		// Score is the MAX window: personal's 77% weekly beats its 5% session.
		{"highest window decides", []Candidate{cand("personal", nil, 5, 77), cand("work", nil, 9, 3)}, "work", 9},
		{"tie keeps config order", []Candidate{cand("a", nil, 10), cand("b", nil, 10)}, "a", 10},
		{"errored candidate skipped", []Candidate{cand("a", errors.New("expired"), 0), cand("b", nil, 50)}, "b", 50},
		{"no windows scores zero", []Candidate{cand("a", nil, 1), cand("b", nil)}, "b", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Best(tc.cands)
			if err != nil {
				t.Fatal(err)
			}
			if res.Chosen.Home.Name() != tc.want || res.Chosen.Score != tc.score {
				t.Errorf("chose %s (%v), want %s (%v)", res.Chosen.Home.Name(), res.Chosen.Score, tc.want, tc.score)
			}
		})
	}
}

// agx TestBest_APIBilledNeverChosen: a per-token home is skipped with
// agentio.ErrBilledPerToken, whether the caller or the usage says so.
func TestBestPerTokenNeverChosen(t *testing.T) {
	t.Parallel()
	byUsage := cand("codex-key", nil)
	byUsage.Usage.Billing = agentio.BillingPerToken
	res, err := Best([]Candidate{apiCand("kimi", nil), byUsage, cand("work", nil, 90)})
	if err != nil || res.Chosen.Home.Name() != "work" {
		t.Fatalf("chose %+v, %v", res.Chosen, err)
	}
	if len(res.Skipped) != 2 || res.Skipped[0].Reason != SkipPerToken || !errors.Is(res.Skipped[0].Err, agentio.ErrBilledPerToken) || res.Skipped[1].Reason != SkipPerToken {
		t.Errorf("skipped = %+v", res.Skipped)
	}
}

// agx TestBest_NoCandidate.
func TestBestNoCandidate(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("a", errors.New("expired")), cand("b", errors.New("offline"))})
	if !errors.Is(err, ErrNoCandidate) || !strings.Contains(err.Error(), "a: expired; b: offline") {
		t.Errorf("err = %v", err)
	}
	if skipNames(res.Skipped) != "a,b" || res.Skipped[0].Reason != SkipUsageError {
		t.Errorf("skipped = %+v", res.Skipped)
	}
	if _, err := Best(nil); !errors.Is(err, ErrNoCandidate) || !strings.Contains(err.Error(), "no candidates") {
		t.Errorf("empty err = %v", err)
	}
}

// agx TestExplain.
func TestExplain(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("personal", nil, 78), cand("x", nil, 40), cand("work", nil, 9)})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Explain(); got != "auto → work (max 9%) over x (max 40%), personal (max 78%)" {
		t.Errorf("Explain = %q", got)
	}
	solo, _ := Best([]Candidate{cand("only", nil, 1)})
	if solo.Explain() != "auto → only (max 1%)" {
		t.Errorf("solo Explain = %q", solo.Explain())
	}
}

// agx TestBest_Matrix: window count × ties × exhaustion × skips.
func TestBestMatrix(t *testing.T) {
	t.Parallel()
	errExpired := errors.New("expired")
	for _, tc := range []struct {
		name    string
		cands   []Candidate
		chosen  string
		score   float64
		others  string // names, best first
		skipped string // names, input order
	}{
		{"single", []Candidate{cand("only", nil, 42)}, "only", 42, "", ""},
		{"single exhausted still chosen", []Candidate{cand("only", nil, 100)}, "only", 100, "", ""},
		{"all exhausted: config order", []Candidate{cand("a", nil, 100, 100), cand("b", nil, 100)}, "a", 100, "b", ""},
		{"over 100 counts as worse", []Candidate{cand("a", nil, 130), cand("b", nil, 100)}, "b", 100, "a", ""},
		{"last wins when strictly lower", []Candidate{cand("a", nil, 3), cand("b", nil, 2), cand("c", nil, 1)}, "c", 1, "b,a", ""},
		{"tie among later keeps order", []Candidate{cand("a", nil, 90), cand("b", nil, 5), cand("c", nil, 5)}, "b", 5, "c,a", ""},
		{"others ties keep order", []Candidate{cand("a", nil, 1), cand("b", nil, 50), cand("c", nil, 50), cand("d", nil, 20)}, "a", 1, "d,b,c", ""},
		{"many windows: max decides", []Candidate{cand("a", nil, 0, 0, 99), cand("b", nil, 50, 50, 50)}, "b", 50, "a", ""},
		{"zero windows ties zero windows", []Candidate{cand("a", nil), cand("b", nil)}, "a", 0, "b", ""},
		{"errored and api skipped", []Candidate{cand("e", errExpired, 0), apiCand("k", nil, 0), cand("w", nil, 99)}, "w", 99, "", "e,k"},
		{"unicode names", []Candidate{cand("wörk", nil, 7), cand("日本", nil, 3)}, "日本", 3, "wörk", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Best(tc.cands)
			if err != nil {
				t.Fatal(err)
			}
			if res.Chosen.Home.Name() != tc.chosen || res.Chosen.Score != tc.score {
				t.Errorf("chose %s (%v), want %s (%v)", res.Chosen.Home.Name(), res.Chosen.Score, tc.chosen, tc.score)
			}
			if got := names(res.Others); got != tc.others {
				t.Errorf("others %s, want %s", got, tc.others)
			}
			if got := skipNames(res.Skipped); got != tc.skipped {
				t.Errorf("skipped %s, want %s", got, tc.skipped)
			}
		})
	}
}

// agx TestBest_ErrorBeatsAPIReason: a fetch error is the reported reason
// even for a per-token home, and all-skipped lists every reason in order.
func TestBestErrorBeatsPerTokenReason(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{apiCand("k", errors.New("offline")), apiCand("j", nil), cand("p", errors.New("429"))})
	if !errors.Is(err, ErrNoCandidate) {
		t.Fatalf("err = %v", err)
	}
	want := "k: offline; j: " + agentio.ErrBilledPerToken.Error() + "; p: 429"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to contain %q", err, want)
	}
	if res.Skipped[0].Reason != SkipUsageError || res.Skipped[1].Reason != SkipPerToken {
		t.Errorf("reasons = %+v", res.Skipped)
	}
}

// agx TestExplain_Rounding: percents render as whole numbers.
func TestExplainRounding(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("a", nil, 4.6), cand("b", nil, 99.4)})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Explain(); got != "auto → a (max 5%) over b (max 99%)" {
		t.Errorf("Explain = %q", got)
	}
}

// reserve is a custom policy: refuse a home whose busiest window is at or
// over the threshold, else score like MaxPercent.
type reserve struct{ threshold float64 }

// Name is "reserve". Pure.
func (reserve) Name() string { return "reserve" }

// Score refuses a home at or over the threshold. Pure.
func (r reserve) Score(c Candidate) (float64, bool, string) {
	if m := c.Usage.MaxPercent(); m >= r.threshold {
		return 0, false, "over the reserve"
	}
	return c.Usage.MaxPercent(), true, ""
}

// TestCustomPolicy: a custom policy scores, refuses with a typed reason
// matching ErrPolicySkip, and is named in Explain.
func TestCustomPolicy(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("personal", nil, 95), cand("work", nil, 40), cand("spare", nil, 60)}, WithPolicy(reserve{threshold: 90}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Chosen.Home.Name() != "work" || res.Policy != "reserve" || len(res.Skipped) != 1 || res.Skipped[0].Reason != SkipPolicy || !errors.Is(res.Skipped[0].Err, ErrPolicySkip) {
		t.Fatalf("result %+v", res)
	}
	if got := res.Explain(); got != "auto [reserve] → work (reserve 40) over spare (reserve 60)" {
		t.Errorf("Explain = %q", got)
	}
	_, err = Best([]Candidate{cand("a", nil, 99)}, WithPolicy(reserve{threshold: 90}))
	if !errors.Is(err, ErrNoCandidate) || !strings.Contains(err.Error(), "a: pick: refused by policy: over the reserve") {
		t.Errorf("all refused: %v", err)
	}
}

// TestAllSkipped: every kind of skip together, reasons in input order.
func TestAllSkipped(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("e", errors.New("expired")), apiCand("k", nil), cand("full", nil, 100)}, WithPolicy(reserve{threshold: 90}))
	if !errors.Is(err, ErrNoCandidate) {
		t.Fatal(err)
	}
	var reasons []SkipReason
	for _, s := range res.Skipped {
		reasons = append(reasons, s.Reason)
	}
	if skipNames(res.Skipped) != "e,k,full" || reasons[0] != SkipUsageError || reasons[1] != SkipPerToken || reasons[2] != SkipPolicy {
		t.Errorf("skipped %+v", res.Skipped)
	}
}

// TestTiesAreStable: equal scores keep input order for the choice and for
// Others, whatever the order of the inputs.
func TestTiesAreStable(t *testing.T) {
	t.Parallel()
	res, err := Best([]Candidate{cand("z", nil, 30), cand("m", nil, 10), cand("a", nil, 10), cand("b", nil, 30)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Chosen.Home.Name() != "m" || names(res.Others) != "a,z,b" {
		t.Errorf("chose %s others %s", res.Chosen.Home.Name(), names(res.Others))
	}
}

// TestCachedAge: a cached or transcript reading shows its age with
// WithNow; a live one, or no clock, shows none.
func TestCachedAge(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	cached := cand("personal", nil, 30)
	cached.Usage.Source, cached.Usage.ObservedAt = agentio.UsageCache, now.Add(-4*time.Minute)
	transcript := cand("work", nil, 10)
	transcript.Usage.Source, transcript.Usage.ObservedAt = agentio.UsageTranscript, now.Add(-26*time.Hour)
	live := cand("spare", nil, 50)
	live.Usage.ObservedAt = now.Add(-time.Hour)
	res, err := Best([]Candidate{cached, transcript, live}, WithNow(now))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Explain(); got != "auto → work (max 10%, cached 1d ago) over personal (max 30%, cached 4m ago), spare (max 50%)" {
		t.Errorf("Explain = %q", got)
	}
	noClock, _ := Best([]Candidate{cached})
	if got := noClock.Explain(); got != "auto → personal (max 30%)" {
		t.Errorf("no clock: %q", got)
	}
	for d, want := range map[time.Duration]string{30 * time.Second: "<1m", 2 * time.Hour: "2h", 50 * time.Hour: "2d"} {
		u := agentio.Usage{Source: agentio.UsageCache, ObservedAt: now.Add(-d)}
		if got := cachedAge(u, now); got != want {
			t.Errorf("age %s = %q, want %q", d, got, want)
		}
	}
}

// TestMixedAgents: Claude and Codex homes compete on one scale, and
// Explain qualifies the names because both have a "personal" home.
func TestMixedAgents(t *testing.T) {
	t.Parallel()
	cands := []Candidate{
		candFor("claude", "personal", nil, 80, 20),
		candFor("codex", "personal", nil, 15, 35),
		candFor("claude", "work", agentio.ErrNotLoggedIn),
	}
	res, err := Best(cands)
	if err != nil {
		t.Fatal(err)
	}
	if res.Chosen.Home.Agent() != "codex" || res.Chosen.Score != 35 {
		t.Fatalf("chose %+v", res.Chosen)
	}
	if got := res.Explain(); got != "auto → codex:personal (max 35%) over claude:personal (max 80%)" {
		t.Errorf("Explain = %q", got)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Home.Agent() != "claude" || !errors.Is(res.Skipped[0].Err, agentio.ErrNotLoggedIn) {
		t.Errorf("skipped %+v", res.Skipped)
	}
}

// TestSkipReasonString names every reason.
func TestSkipReasonString(t *testing.T) {
	t.Parallel()
	for r, want := range map[SkipReason]string{SkipUsageError: "usage-error", SkipPerToken: "per-token", SkipPolicy: "policy", 9: "SkipReason(9)"} {
		if r.String() != want {
			t.Errorf("%d = %q", r, r.String())
		}
	}
}
