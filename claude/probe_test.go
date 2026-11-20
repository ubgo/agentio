package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// probeHeaders builds a probe answer's headers: name → utilization,
// reset, status.
func probeHeaders(windows map[string][3]string, claim string) http.Header {
	h := http.Header{}
	for id, v := range windows {
		if v[0] != "" {
			h.Set(hdrPrefix+id+hdrUtilization, v[0])
		}
		if v[1] != "" {
			h.Set(hdrPrefix+id+hdrReset, v[1])
		}
		if v[2] != "" {
			h.Set(hdrPrefix+id+hdrStatus, v[2])
		}
	}
	if claim != "" {
		h.Set(hdrRepresentative, claim)
	}
	h.Set(hdrPrefix+"status", "allowed")
	h.Set(hdrPrefix+"fallback-percentage", "0.5")
	h.Set(hdrPrefix+"overage-status", "rejected")
	return h
}

// TestParseProbeHeadersGeneric: every <window> becomes a window with its
// id, length, scope, percent, reset, status and binding, in a stable
// order (5h, 7d, then by name); the overall status, fallback and overage
// headers are not windows; an unknown future window is kept.
func TestParseProbeHeadersGeneric(t *testing.T) {
	h := probeHeaders(map[string][3]string{
		"5h":     {"0.12", "1794700000", "allowed"},
		"7d":     {"0.4", "1795000000", "allowed_warning"},
		"7d_oi":  {"0.8", "1795100000", "rejected"},
		"30d_zz": {"0.05", "1796000000", ""},
	}, "seven_day_oi")
	u, err := ParseProbeHeaders(h)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id, scope, kind, status string
		length                  time.Duration
		percent                 float64
		binding                 bool
	}{
		{"5h", "", kindSession, "allowed", sessionLength, 12, false},
		{"7d", "", kindWeeklyAll, "allowed_warning", weeklyLength, 40, false},
		{"30d_zz", "zz", "30d_zz", "", 30 * 24 * time.Hour, 5, false},
		{"7d_oi", "oi", kindWeeklyScoped, agentio.WindowRejected, weeklyLength, 80, true},
	}
	if len(u.Windows) != len(want) {
		t.Fatalf("windows %+v", u.Windows)
	}
	for i, w := range want {
		g := u.Windows[i]
		if g.ID != w.id || g.Scope != w.scope || g.VendorKind != w.kind || g.Status != w.status || g.Length != w.length ||
			g.Percent != w.percent || g.Binding != w.binding || g.ResetsAt == nil {
			t.Errorf("window %d: %+v, want %+v", i, g, w)
		}
	}
	if u.Windows[3].Label != "Scoped (oi)" || u.Windows[0].Label != labelSession {
		t.Errorf("labels %q %q", u.Windows[3].Label, u.Windows[0].Label)
	}
}

// TestParseProbeHeadersBroken: a broken 5h or 7d still fails the probe;
// a broken unknown window is skipped; no window is ErrProbeHeaders.
func TestParseProbeHeadersBroken(t *testing.T) {
	for name, c := range map[string]struct {
		windows map[string][3]string
		ok      bool
		n       int
	}{
		"7d without reset":           {map[string][3]string{"7d": {"0.4", "", ""}}, false, 0},
		"5h not a number":            {map[string][3]string{"5h": {"x", "1794700000", ""}, "7d": {"0.4", "1795000000", ""}}, false, 0},
		"unknown without reset":      {map[string][3]string{"7d": {"0.4", "1795000000", ""}, "7d_oi": {"0.8", "", ""}}, true, 1},
		"unknown bad number":         {map[string][3]string{"7d": {"0.4", "1795000000", ""}, "7d_oi": {"NaN", "1795000000", ""}}, true, 1},
		"unknown bad reset":          {map[string][3]string{"7d": {"0.4", "1795000000", ""}, "7d_oi": {"0.1", "-4", ""}}, true, 1},
		"only an unknown window":     {map[string][3]string{"7d_oi": {"0.8", "1795000000", ""}}, true, 1},
		"nothing but overall status": {map[string][3]string{}, false, 0},
	} {
		u, err := ParseProbeHeaders(probeHeaders(c.windows, ""))
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrProbeHeaders)) || len(u.Windows) != c.n {
			t.Errorf("%s: %+v %v", name, u.Windows, err)
		}
	}
}

// TestClaimOf: the claim wording of a header window name.
func TestClaimOf(t *testing.T) {
	for id, want := range map[string]string{"5h": "five_hour", "7d": "seven_day", "7d_oi": "seven_day_oi", "30d": "thirty_day", "9x": "", "d": "", "99h": ""} {
		if got := claimOf(id); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
	if w := probeWindow("5h", 1, nil, "", "5h"); !w.Binding {
		t.Error("a claim naming the window by its header name does not bind")
	}
}

// TestProbeModel: aliases resolve to model ids, a context suffix is
// dropped, an id passes, "" and default are the default probe model.
func TestProbeModel(t *testing.T) {
	for in, want := range map[string]string{"": probeModel, "default": probeModel, "fable": "claude-fable-5-2", "Opus[1m]": "claude-opus-5-5",
		"sonnet": "claude-sonnet-5-5", "claude-fable-5-2": "claude-fable-5-2", " haiku ": "claude-haiku-5-5"} {
		if got := ProbeModel(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// TestTokenUsageModel: the probe sends the resolved model and still
// identifies as Claude Code; the scoped window of that model comes back;
// a 429 with headers is usage (the scoped window rejected and binding);
// a refused model is ErrModelRefused, never a login failure.
func TestTokenUsageModel(t *testing.T) {
	var sent probeBody
	status, refuse := http.StatusOK, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &sent)
		if refuse != "" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"not_found_error","message":"model: ` + refuse + `"}}`))
			return
		}
		for k, v := range probeHeaders(map[string][3]string{"5h": {"0.1", "1794700000", "allowed"}, "7d": {"0.3", "1795000000", "allowed"},
			"7d_oi": {"1.0", "1795100000", "rejected"}}, "seven_day_oi") {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithHTTP(srv.Client()))
	cred := agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("sk-ant-oat01-x")}
	a = a.WithBaseURL(srv.URL)
	var mp agentio.ModelTokenProber = a.Neutral().(neutral)
	u, err := mp.TokenUsageModel(context.Background(), cred, "fable")
	if err != nil || sent.Model != "claude-fable-5-2" || sent.System != probeSystem || len(u.Windows) != 3 || u.Windows[2].ID != "7d_oi" {
		t.Fatalf("fable probe: model %q, %+v %v", sent.Model, u.Windows, err)
	}
	status = http.StatusTooManyRequests
	u, err = mp.TokenUsageModel(context.Background(), cred, "fable")
	if err != nil || u.Windows[2].Status != agentio.WindowRejected || !u.Windows[2].Binding || u.Windows[2].Percent != 100 {
		t.Fatalf("429 with headers: %+v %v", u.Windows, err)
	}
	if _, err := a.Prober().TokenUsage(context.Background(), cred); err != nil || sent.Model != probeModel {
		t.Errorf("TokenUsage model %q %v", sent.Model, err)
	}
	refuse = "claude-nope-1"
	_, err = mp.TokenUsageModel(context.Background(), cred, "claude-nope-1")
	if !errors.Is(err, agentio.ErrModelRefused) || errors.Is(err, agentio.ErrUnauthorized) {
		t.Errorf("refused model: %v", err)
	}
}

// TestModelRefused: only a 404, or a 400 that names the model.
func TestModelRefused(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
		want   bool
	}{
		{404, `{"error":{"type":"not_found_error","message":"x"}}`, true},
		{400, `{"error":{"type":"invalid_request_error","message":"Invalid model name"}}`, true},
		{400, `{"error":{"type":"invalid_request_error","message":"max_tokens too small"}}`, false},
		{500, `{"error":{"type":"not_found_error"}}`, false},
		{404, `not json`, false},
	} {
		if got := modelRefused(c.status, []byte(c.body)); got != c.want {
			t.Errorf("%d %s: %v", c.status, c.body, got)
		}
	}
}
