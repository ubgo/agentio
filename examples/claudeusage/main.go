// Command claudeusage shows the four ways agentio reads Claude Code plan
// usage: the live usage endpoint, the copy Claude Code caches in
// .claude.json, the extra-usage object, and a token probe that reads the
// rate-limit headers of one minimal request.
//
// Every network call goes to a local httptest server through
// Agent.WithBaseURL; the live endpoint serves the fixture's own cached body.
// Nothing reaches api.anthropic.com and no quota is spent.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultFixture is the fixture user home, relative to the examples module.
const defaultFixture = "../claude/testdata/homes/2.1.325/userhome"

// Endpoint paths the fake server answers (the vendor's paths).
const (
	usagePath    = "/api/oauth/usage"
	messagesPath = "/v1/messages"
)

// Probe header names and values the fake /v1/messages returns: the
// anthropic-ratelimit-unified-* family, utilization as a 0..1 fraction and
// reset as unix seconds.
const (
	hdr5hUtil      = "anthropic-ratelimit-unified-5h-utilization"
	hdr5hReset     = "anthropic-ratelimit-unified-5h-reset"
	hdr7dUtil      = "anthropic-ratelimit-unified-7d-utilization"
	hdr7dReset     = "anthropic-ratelimit-unified-7d-reset"
	hdrClaim       = "anthropic-ratelimit-unified-representative-claim"
	probe5hUtil    = "0.31"
	probe7dUtil    = "0.07"
	probeClaim     = "five_hour"
	probe5hResetIn = 3 * time.Hour
	probe7dResetIn = 4 * 24 * time.Hour
)

// fixtureNow is the fixed clock: before the fixture login expires.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "Claude fixture user home to read")
	flag.Parse()
	if err := run(context.Background(), os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "claudeusage:", err)
		os.Exit(1)
	}
}

// fakeAPI serves the usage endpoint with body and answers a probe with
// rate-limit headers. It only checks that a bearer token was sent; it never
// logs or echoes it.
func fakeAPI(body []byte) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+usagePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// A failed write means the client hung up; a handler has no caller to
		// report it to, so the result is dropped.
		_, _ = w.Write(body)
	})
	mux.HandleFunc("POST "+messagesPath, func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set(hdr5hUtil, probe5hUtil)
		h.Set(hdr5hReset, strconv.FormatInt(fixtureNow().Add(probe5hResetIn).Unix(), 10))
		h.Set(hdr7dUtil, probe7dUtil)
		h.Set(hdr7dReset, strconv.FormatInt(fixtureNow().Add(probe7dResetIn).Unix(), 10))
		h.Set(hdrClaim, probeClaim)
		_, _ = io.WriteString(w, `{"type":"message"}`)
	})
	return httptest.NewServer(mux)
}

// run reads usage for the fixture home: the live endpoint and the probe go
// to a local httptest server (WithBaseURL), the cached copy comes from the
// fixture's .claude.json. No request leaves the machine.
func run(ctx context.Context, w io.Writer, fixture string) error {
	abs, err := filepath.Abs(fixture)
	if err != nil {
		return err
	}
	host := agentio.Host{UserHome: abs, Env: []string{"HOME=" + abs, "PATH="}, Now: fixtureNow}
	base := claude.New(host)
	h := base.DefaultHome()
	// The fake live endpoint returns the body Claude Code cached on disk.
	cached, err := base.CachedUsageBody(h)
	if err != nil || cached == nil {
		return fmt.Errorf("fixture has no cached usage body: %w", err)
	}
	srv := fakeAPI(cached.Body)
	defer srv.Close()
	host.HTTP = srv.Client()
	a := claude.New(host).WithBaseURL(srv.URL)

	live, err := readLive(ctx, w, a, h)
	if err != nil {
		return err
	}
	if err := readCached(w, a, h); err != nil {
		return err
	}
	if err := readExtra(w, live); err != nil {
		return err
	}
	return probe(ctx, w, a)
}

// snippet:start live
// readLive fetches /api/oauth/usage once for h's login. Each window has a
// label, a percent, a reset time and a length class; Limits keeps the wire
// rows. Never call this in a loop: the endpoint rate-limits (429).
func readLive(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) (claude.Usage, error) {
	u, err := a.Usage(ctx, h)
	switch {
	case errors.Is(err, agentio.ErrNotLoggedIn), errors.Is(err, agentio.ErrExpired):
		fmt.Fprintln(w, "no usable login:", err)
		return u, nil
	case errors.Is(err, agentio.ErrRateLimited):
		fmt.Fprintln(w, "rate limited; use the cached copy instead")
		return u, nil
	case err != nil:
		return u, err
	}
	fmt.Fprintf(w, "live usage (%s, billing %s), highest window %.0f%%\n", u.Source, u.Billing, u.MaxPercent())
	for _, win := range u.Windows {
		fmt.Fprintf(w, "  %-16s %3.0f%%  class %-5s resets %s\n", win.Label, win.Percent, win.Class(), win.ResetsAt.UTC().Format(time.RFC3339))
	}
	for _, l := range u.Limits {
		fmt.Fprintf(w, "  wire row: kind %s, group %s, severity %s, active %t, label %q\n", l.Kind, l.Group, l.Severity, l.IsActive, l.Label())
	}
	if _, err := claude.ParseUsage([]byte(`{"limits":[]}`)); errors.Is(err, claude.ErrNoLimits) {
		fmt.Fprintln(w, "  empty body:", err)
	}
	return u, nil
}

// snippet:end live

// snippet:start cached
// readCached parses the usage body Claude Code itself cached in
// .claude.json: no network, safe in a loop, as old as its ObservedAt.
func readCached(w io.Writer, a *claude.Agent, h claude.Home) error {
	u, found, err := a.CachedUsage(h)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(w, "cached usage: none in this home")
		return nil
	}
	age := a.Host().Now().Sub(u.ObservedAt).Round(time.Hour)
	fmt.Fprintf(w, "cached usage (%s), observed %s (%s old), %d windows\n", u.Source, u.ObservedAt.UTC().Format(time.RFC3339), age, len(u.Windows))
	return nil
}

// snippet:end cached

// snippet:start extra
// readExtra decodes the pay-as-you-go overage object from a neutral
// usage's Raw body: code that only holds an agentio.Usage can still get it.
func readExtra(w io.Writer, u claude.Usage) error {
	extra, err := claude.ExtraUsageOf(u.Neutral())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "extra usage: enabled %t, utilization set %t, unknown keys %d\n", extra.IsEnabled, extra.Utilization != nil, len(extra.Unknown))
	return nil
}

// snippet:end extra

// snippet:start probe
// probe reads a token login's usage from the rate-limit headers of one
// minimal /v1/messages call. It is the Quota door: it spends a few tokens
// on the real API, so run it once, never in a loop. An API key has no plan
// windows and is answered with ErrBilledPerToken without a request.
func probe(ctx context.Context, w io.Writer, a *claude.Agent) error {
	p := a.Prober()
	u, err := p.TokenUsage(ctx, agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("sk-ant-oat01-EXAMPLE")})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "probe usage (%s):\n", u.Source)
	for _, win := range u.Windows {
		fmt.Fprintf(w, "  %-16s %3.0f%%  resets %s\n", win.Label, win.Percent, win.ResetsAt.UTC().Format(time.RFC3339))
	}
	if _, err := p.TokenUsage(ctx, agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("sk-ant-api03-EXAMPLE")}); errors.Is(err, agentio.ErrBilledPerToken) {
		fmt.Fprintln(w, "api key:", err)
	}
	half := http.Header{}
	half.Set(hdr5hUtil, probe5hUtil)
	if _, err := claude.ParseProbeHeaders(half); errors.Is(err, claude.ErrProbeHeaders) {
		fmt.Fprintln(w, "half a header pair:", err)
	}
	return nil
}

// snippet:end probe
