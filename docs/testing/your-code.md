# Testing your own code

Code that uses agentio is easy to test if it takes an `agentio.Host` (or an agent built from one) as a parameter instead of calling `agentio.HostFromOS()` itself. Then the program passes the real machine and the test passes a fixture Host from `agenttest`, with no build tags, no environment juggling and no chance of reading your own homes in CI. This page shows that shape on a small report that loops over every agent.

## Before you start

- Modules: `github.com/ubgo/agentio` (root, includes `agenttest`), `github.com/ubgo/agentio/builtin` for the set of built-in agents, plus `claude` / `codex` if your code uses vendor types.
- Read [Fakes and fixture hosts](agenttest.md) for the Host defaults and [Fixture homes](fixtures.md) for the homes used below.
- The code is [examples/testing/main.go](../../examples/testing/main.go) and its tests are in [main_test.go](../../examples/testing/main_test.go).

## Step 1 — Inject a Host

Keep the only `HostFromOS()` call in `main` (or wherever your program starts) and pass the Host down. Everything below it is ordinary code that you can call from a test. Here `collect` builds the agents with `builtin.New(h, custom...)`, loops over their neutral views, and uses the typed accessor `agentio.ConversationsOf` so an agent without a conversation reader is skipped, not special-cased.

<!-- snippet: examples/testing/main.go#yourcode -->
```go
// AgentReport is one row of this program's report.
type AgentReport struct {
	ID            agentio.ID
	Homes         []string
	Conversations int
	Capabilities  int
}

// collect is the code under test: it loops over every built-in agent on h,
// plus one config-defined agent per custom Spec, and counts homes,
// conversations and supported capabilities. It takes the Host as a
// parameter, so a test hands it a fixture Host and a program hands it
// HostFromOS(). An agent without a conversation reader counts zero. Disk ·
// Read.
func collect(ctx context.Context, h agentio.Host, custom ...agentio.Spec) ([]AgentReport, error) {
	agents, err := builtin.New(h, custom...)
	if err != nil {
		return nil, err
	}
	var out []AgentReport
	for _, a := range agents.All() {
		homes, err := a.Homes()
		if err != nil {
			return nil, fmt.Errorf("%s homes: %w", a.ID(), err)
		}
		r := AgentReport{ID: a.ID(), Capabilities: len(agentio.Supported(a))}
		lister, canList := agentio.ConversationsOf(a)
		for _, home := range homes {
			r.Homes = append(r.Homes, home.Name())
			if !canList {
				continue
			}
			convs, err := lister.Conversations(ctx, home, agentio.ConversationQuery{})
			if err != nil {
				return nil, fmt.Errorf("%s conversations in %s: %w", a.ID(), home.Name(), err)
			}
			r.Conversations += len(convs)
		}
		out = append(out, r)
	}
	return out, nil
}

// run prints the report for h.
func run(ctx context.Context, w io.Writer, h agentio.Host) error {
	rows, err := collect(ctx, h)
	if err != nil {
		return err
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%s homes=%v conversations=%d capabilities=%d\n", r.ID, r.Homes, r.Conversations, r.Capabilities)
	}
	return nil
}
```

In the example, `main` hands `run` a Host over the checked-in fixtures (so `go run ./testing` gives the same output everywhere); a real tool would hand it `agentio.HostFromOS()`. The test hands it a fixture Host built with `agenttest.NewHost` over a copy of the fixtures and checks the exact output. Because that Host's HTTP client and process runner fail the test on use, the same test also proves the report is a pure Disk read.

<!-- snippet: examples/testing/main_test.go#testcollect -->
```go
// TestCollect runs the code under test on the fixture Host and checks its
// output. FailingHTTP and FailExec (agenttest defaults) prove the report
// reads disk only: a network call or a process run fails the test.
func TestCollect(t *testing.T) {
	h := newFixtureHost(t)
	var b strings.Builder
	if err := run(context.Background(), &b, h); err != nil {
		t.Fatal(err)
	}
	want := "claude homes=[personal work] conversations=3 capabilities=27\n" +
		"codex homes=[personal] conversations=2 capabilities=23\n" +
		"gemini homes=[] conversations=0 capabilities=14\n" +
		"pi homes=[] conversations=0 capabilities=13\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}
```

## Step 2 — Test a loop over agents

A loop over agents should work for agents you have not met yet, including config-defined ones (see [Config-defined agents](../capabilities/fromspec.md)). Give your function a way to accept extra `Spec`s, write a home for a toy agent into the copied user home, and check that the loop handles it: here the toy agent has one home and no conversation reader, so it reports zero conversations and no error.

<!-- snippet: examples/testing/main_test.go#loop -->
```go
// toySpec is a config-defined agent with one home marker and no
// conversation reader: the loop in collect must handle it without a
// special case.
func toySpec() agentio.Spec {
	return agentio.Spec{
		ID: "toy", Name: "Toy Agent", Binary: "toy",
		DefaultHome: ".toy", HomeMarkers: []string{"config.json"},
		NewArgs: []string{"{args}"},
	}
}

// TestLoopOverAgents runs collect with the two built-ins plus the toy
// agent. The toy home is written into the copied user home, so the loop
// sees three agents and the toy one reports no conversations.
func TestLoopOverAgents(t *testing.T) {
	h := newFixtureHost(t)
	agenttest.WriteFile(t, h.UserHome, ".toy/config.json", "{}")
	rows, err := collect(context.Background(), h, toySpec())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]agentio.ID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if !slices.Equal(ids, []agentio.ID{claude.ID, codex.ID, gemini.ID, pi.ID, "toy"}) {
		t.Fatalf("ids %v", ids)
	}
	toy := rows[len(rows)-1]
	if len(toy.Homes) != 1 || toy.Conversations != 0 {
		t.Fatalf("toy row %+v", toy)
	}
}
```

## Step 3 — Keep docs snippets in sync

The snippets on this site are cut from the example programs, so they always compile and their tests always run. If you contribute to agentio, the same machinery applies to your changes: mark a region in a Go file with `// snippet:start NAME` and `// snippet:end NAME`, put `<!-- snippet: examples/<dir>/<file>.go#NAME -->` on the line above a fenced block in the markdown, and run

```sh
go test -count=1 ./internal/docscheck/ -run TestSnippets -args -update
```

from the repository root (`task docs:snippets`) to rewrite the blocks from source. `task docs:check` (`go test -count=1 ./internal/docscheck/`) then fails on a block that drifted from its source, a broken relative link or `#anchor`, a hard-wrapped paragraph, or a local machine path in a public file. The checker lives in `internal/docscheck` and has no non-test code. The same task then runs `ds check` ([docsync](https://github.com/ubgo/docsync)): behavioural sentences in the README and guides cite the code that implements them, and the check fails when that code changed since the sentence was acked. CONTRIBUTING describes how to re-check and `ds ack` a flagged sentence.

## Try it

```sh
cd examples && go test -count=1 -v -run 'TestCollect|TestLoopOverAgents' ./testing
```

```text
=== RUN   TestCollect
--- PASS: TestCollect (0.01s)
=== RUN   TestLoopOverAgents
--- PASS: TestLoopOverAgents (0.00s)
PASS
ok  	github.com/ubgo/agentio/examples/testing	0.015s
```

## Pitfalls

- Calling `agentio.HostFromOS()` inside a helper makes every caller untestable; the same goes for building `builtin.New` from a Host you created in a package-level variable. Pass the Host (or the `builtin.Agents`) in.
- A Host built with `agenttest.NewHost` reports a stray process run or HTTP request with `Errorf` and returns `ErrForbidden`. If your code swallows errors, the test still fails, but only through that report: read the failure output.
- `builtin.New` refuses a custom `Spec` whose ID equals a built-in one (`claude`, `codex`) or repeats an earlier custom ID; it never overrides. Pick a distinct ID for test agents.
- A config-defined agent with `HomeMarkers` only sees a home that contains one of the markers; write the marker file (here `.toy/config.json`) before you build the agents.
- Home names differ by how a home was found: the Codex fixture copied into `<userhome>/.codex` is the default home `personal`, the same files selected with `CODEX_HOME` are a non-default home named after the directory. Do not hard-code a home name your production machine will not have.
- The length of `agentio.Supported(a)` grows when the library adds a capability. Assert on the capabilities your code needs (`agentio.ConversationsOf(a)` returning `ok`), not on the total, unless you want to notice every addition.

## Related

- [Fakes and fixture hosts](agenttest.md) and [Fixture homes](fixtures.md).
- [The conformance suite](conformance.md): check the agents themselves.
- [Capability accessors](../capabilities/accessors.md): `ConversationsOf`, `Supported` and the rest.
- [The built-in agent set](../capabilities/builtin.md) and [Config-defined agents](../capabilities/fromspec.md).
- [The Host](../core/host.md).
- [examples/testing](../../examples/testing/) and the [feature index](../reference/features.md#testing-agenttest-and-fixtures).
- [Choosing guides](../choosing/README.md).
