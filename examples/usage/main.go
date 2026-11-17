// Command usage reads plan usage and the subscription through the shared
// accessors, builds login commands, maps a token login onto a launch and
// probes a token, all against an in-process fake vendor API and copies of
// the recorded fixture homes. It never reaches the real network, keychain
// or quota.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// Sample tokens: shapes only, never real values. DetectTokenKind reads the
// prefix; the probe sends the OAuth one to the fake API.
const (
	sampleOAuthToken = "sk-ant-" + "oat01-EXAMPLE-NOT-A-TOKEN"
	sampleAPIKey     = "sk-ant-" + "api03-EXAMPLE-NOT-A-KEY"
	sampleRefresh    = "sk-ant-" + "ort01-EXAMPLE-NOT-A-TOKEN"
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "usage:", err)
		os.Exit(1)
	}
}

// runMain starts the fake API, copies the fixtures to a temp dir and runs
// every step. Disk · Write (temp dir).
func runMain(w io.Writer) error {
	srv := fakeAPI()
	defer srv.Close()
	client := srv.Client()
	tmp, err := os.MkdirTemp("", "agentio-usage-")
	if err != nil {
		return err
	}
	// The temp dir holds only copies; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	h, err := setup(tmp, client)
	if err != nil {
		return err
	}
	return run(context.Background(), w, h, srv.URL)
}

// run prints each step. baseURL is the fake API origin both agents are
// pointed at with WithBaseURL, so no request leaves the process.
func run(ctx context.Context, w io.Writer, h agentio.Host, baseURL string) error {
	agents := []agentio.Agent{claude.New(h).WithBaseURL(baseURL).Neutral(), codex.New(h).WithBaseURL(baseURL).Neutral()}
	steps := []struct {
		title string
		fn    func() error
	}{
		{"cached usage", func() error { return printCachedUsage(w, h, agents) }},
		{"live usage", func() error { return printLiveUsage(ctx, w, agents) }},
		{"subscription", func() error { return printSubscriptions(ctx, w, agents) }},
		{"login commands", func() error { printLoginCommands(w, agents); return nil }},
		{"token login", func() error { return printTokenLogin(w, agents) }},
		{"token probe", func() error { return printTokenProbe(ctx, w, agents) }},
	}
	for _, s := range steps {
		fmt.Fprintf(w, "== %s ==\n", s.title)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	return nil
}

// printWindows prints one line per usage window. Class is derived from the
// window length; ResetsAt is nil for a window with no scheduled reset.
func printWindows(w io.Writer, u agentio.Usage) {
	for _, win := range u.Windows {
		reset := "-"
		if win.ResetsAt != nil {
			reset = win.ResetsAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(w, "  %-16s %-5s %5.1f%% resets %s\n", win.Label, win.Class(), win.Percent, reset)
	}
}

// snippet:start cached
// printCachedUsage reads the usage each vendor CLI left on disk: no network,
// safe in a loop. The reading is as old as the vendor's last refresh, so the
// age (Host clock minus ObservedAt) is part of the answer.
func printCachedUsage(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		cu, ok := agentio.CachedUsageOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			u, found, err := cu.CachedUsage(home)
			if err != nil {
				return err
			}
			if !found {
				fmt.Fprintf(w, "%s %s: no cached usage\n", a.ID(), home.Name())
				continue
			}
			age := h.Now().Sub(u.ObservedAt).Round(time.Minute)
			fmt.Fprintf(w, "%s %s: max %.0f%% source=%s age=%s\n", a.ID(), home.Name(), u.MaxPercent(), u.Source, age)
			printWindows(w, u)
		}
	}
	return nil
}

// snippet:end cached

// snippet:start live
// printLiveUsage asks each vendor once for the default home's usage. This
// is a network read that may 429: call it on demand, cache the answer,
// never poll. An expired login is refused before any request.
func printLiveUsage(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		ur, ok := agentio.UsageOf(a)
		if !ok {
			continue
		}
		u, err := ur.Usage(ctx, a.DefaultHome())
		switch {
		case errors.Is(err, agentio.ErrExpired):
			fmt.Fprintf(w, "%s: login expired; run the vendor CLI once to refresh it\n", a.ID())
			continue
		case errors.Is(err, agentio.ErrRateLimited):
			fmt.Fprintf(w, "%s: rate limited; keep the last reading\n", a.ID())
			continue
		case err != nil:
			return err
		}
		fmt.Fprintf(w, "%s: max %.0f%% billing=%s source=%s\n", a.ID(), u.MaxPercent(), u.Billing, u.Source)
		printWindows(w, u)
	}
	return nil
}

// snippet:end live

// snippet:start subscription
// printSubscriptions reads the plan of each default home. Claude asks the
// profile endpoint (network); Codex answers from the login on disk first.
func printSubscriptions(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		sr, ok := agentio.SubscriptionOf(a)
		if !ok {
			continue
		}
		s, err := sr.Subscription(ctx, a.DefaultHome())
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: plan=%q status=%q billing-type=%q\n", a.ID(), s.Plan, s.Status, s.BillingType)
	}
	return nil
}

// snippet:end subscription

// snippet:start login
// printLoginCommands builds the login command of every flow, and the logout
// command, for each default home. They are data: the caller runs them in a
// terminal. A flow the vendor does not have is ErrUnsupported.
func printLoginCommands(w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		lc, ok := agentio.LoginCommanderOf(a)
		if !ok {
			continue
		}
		home := a.DefaultHome()
		for _, f := range agentio.LoginFlowValues() {
			cmd, err := lc.LoginCommand(home, f)
			if errors.Is(err, agentio.ErrUnsupported) {
				fmt.Fprintf(w, "%s %-19s unsupported\n", a.ID(), f)
				continue
			}
			fmt.Fprintf(w, "%s %-19s %s\n", a.ID(), f, cmd)
		}
		fmt.Fprintf(w, "%s %-19s %s\n", a.ID(), "logout", lc.LogoutCommand(home))
	}
}

// snippet:end login

// snippet:start token
// printTokenLogin recognises token shapes, maps a token onto the vendor's
// variables and builds a launch that uses it instead of the home's login.
// The token lives only in Command.Env; argv and printed values are redacted.
func printTokenLogin(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		tl, ok := agentio.TokenLoginOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapTokenLogin)
			fmt.Fprintf(w, "%s: no token login (%s)\n", a.ID(), reason)
			continue
		}
		fmt.Fprintf(w, "%s kinds: %v\n", a.ID(), tl.TokenKinds())
		for _, s := range []string{sampleOAuthToken, sampleAPIKey, sampleRefresh} {
			kind, ok := tl.DetectTokenKind(agentio.NewToken(s))
			fmt.Fprintf(w, "  %.13s... kind=%s usable=%t\n", s, kind, ok)
		}
		cred := agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken(sampleOAuthToken)}
		env, err := tl.TokenEnv(cred)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  token env: %v\n", env)
		cmd, err := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Prompt: "hello", Credential: &cred})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  launch: %s\n", cmd)
		fmt.Fprintf(w, "  secret env names: %v\n", cmd.SecretEnvNames())
		fmt.Fprintf(w, "  token on argv: %t\n", strings.Contains(cmd.String(), sampleOAuthToken))
		if setup, ok := tl.TokenSetupCommand(a.DefaultHome()); ok {
			fmt.Fprintf(w, "  setup: %s\n", setup)
		}
	}
	return nil
}

// snippet:end token

// snippet:start probe
// printTokenProbe reads a token's usage from one minimal request and its
// rate-limit headers. It spends a little quota: run it once on demand,
// never in a loop. An API key has no plan windows and is never probed.
func printTokenProbe(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		tp, ok := agentio.TokenProberOf(a)
		if !ok {
			continue
		}
		oauth := agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken(sampleOAuthToken)}
		u, err := tp.TokenUsage(ctx, oauth)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s oauth token: max %.0f%% source=%s\n", a.ID(), u.MaxPercent(), u.Source)
		printWindows(w, u)
		key := agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken(sampleAPIKey)}
		if _, err := tp.TokenUsage(ctx, key); errors.Is(err, agentio.ErrBilledPerToken) {
			fmt.Fprintf(w, "%s api key: billed per token, not probed\n", a.ID())
		}
	}
	return nil
}

// snippet:end probe
