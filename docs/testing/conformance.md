# The conformance suite

`agenttest.Conformance` runs the checks every agent must pass, Claude Code, Codex or one you define from a `Spec`, in one call. Run it when you write a new agent (see [Config-defined agents](../capabilities/fromspec.md)), when you wrap a vendor agent, or as a guard in your own test suite that the agents you depend on still keep their promises on your fixture homes.

## Before you start

- A `*testing.T` (the suite runs subtests, so `testing.TB` is not enough).
- An agent whose Host comes from `agenttest.NewHost`, or at least carries `FailingHTTP(t)` and `FailExec{T: t}`: the read checks rely on them to catch a process run or a network call. See [Fakes and fixture hosts](agenttest.md).
- A fixture home of that agent with real files in it. The snippet uses the `newFixtureHost` helper from [Fakes and fixture hosts](agenttest.md#step-1--build-a-fixture-host), which copies the [fixture homes](fixtures.md) into a temp dir.

## Step 1 — Run Conformance on an agent

Pass the neutral view of the agent (`Neutral()` on a vendor agent) and a neutral home. The suite runs five subtests:

| Subtest | What it checks |
|---|---|
| `home-env` | The default home launches with the home variable (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`) absent, even when the target's extra env tries to set it; a non-default home launches with it set exactly once to the home dir; a home of another agent is refused. |
| `resume` | A `LaunchRequest.ResumeID` lands in the argv. Skipped when the agent cannot resume. |
| `supported` | `agentio.Supported` agrees with `agentio.Unsupported` (a capability is never both), and every supported capability has a cost row. |
| `read-accessors` | Every read accessor the agent supports (install, identity, cached usage, conversations, history, prompts, live sessions, instruction files, MCP servers, skills, plugins, config, models, doctor, homes) runs on the fixture home without reaching `Host.Exec` or `Host.HTTP`, and no output contains the probe secret. |
| `token-redaction` | A token login keeps the token out of argv, errors, `String()`, every `fmt` verb and JSON; only `Command.Env` holds it. Skipped when the agent has no token login. |

<!-- snippet: examples/testing/main_test.go#conformance -->
```go
// TestConformance runs the shared suite on both vendors' neutral views with
// a fixture home each. The suite fails the test if a read accessor runs a
// process or makes a network call, if the default home launches with the
// home variable set, or if a token login leaks the token.
func TestConformance(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		a := claude.New(newFixtureHost(t))
		agenttest.Conformance(t, a.Neutral(), a.DefaultHome().Neutral())
	})
	t.Run("codex", func(t *testing.T) {
		a := codex.New(newFixtureHost(t))
		agenttest.Conformance(t, a.Neutral(), a.DefaultHome().Neutral())
	})
}
```

The vendor modules run the same call in their own tests on every fixture version, and the root module runs it on a sample `Spec` agent, so an agent you build from a `Spec` is held to the same rules as the built-in ones.

## Try it

```sh
cd examples && go test -count=1 -v -run TestConformance ./testing
```

```text
=== RUN   TestConformance
=== RUN   TestConformance/claude
=== RUN   TestConformance/claude/home-env
=== RUN   TestConformance/claude/resume
=== RUN   TestConformance/claude/supported
=== RUN   TestConformance/claude/read-accessors
=== RUN   TestConformance/claude/token-redaction
=== RUN   TestConformance/codex
=== RUN   TestConformance/codex/home-env
=== RUN   TestConformance/codex/resume
=== RUN   TestConformance/codex/supported
=== RUN   TestConformance/codex/read-accessors
=== RUN   TestConformance/codex/token-redaction
--- PASS: TestConformance (0.04s)
    --- PASS: TestConformance/claude (0.02s)
        --- PASS: TestConformance/claude/home-env (0.00s)
        --- PASS: TestConformance/claude/resume (0.00s)
        --- PASS: TestConformance/claude/supported (0.00s)
        --- PASS: TestConformance/claude/read-accessors (0.02s)
        --- PASS: TestConformance/claude/token-redaction (0.00s)
    --- PASS: TestConformance/codex (0.01s)
        --- PASS: TestConformance/codex/home-env (0.00s)
        --- PASS: TestConformance/codex/resume (0.00s)
        --- PASS: TestConformance/codex/supported (0.00s)
        --- PASS: TestConformance/codex/read-accessors (0.01s)
        --- PASS: TestConformance/codex/token-redaction (0.00s)
PASS
ok  	github.com/ubgo/agentio/examples/testing	0.045s
```

## Pitfalls

- A Host built by hand with a real `http.Client` and a real runner makes the `read-accessors` subtest meaningless: nothing fails when a read reaches the network. Build it with `agenttest.NewHost` and override only what you need.
- The suite reads the home you pass, so pass a home with files. An empty temp dir passes trivially and proves little; copy a [fixture home](fixtures.md) first.
- Read errors do not fail the suite (a reader may legitimately return `ErrNotFound` on a sparse fixture); only a process run, a network call or a leaked probe secret does. Assert values in your own tests.
- Unknown-key preservation is format-specific and is not part of the suite; each vendor's reader tests check it.
- The `home-env` subtest needs a non-default home to compare; when you pass the default home it builds one next to it (`<default>-agenttest`), which does not need to exist.

## Related

- [Fakes and fixture hosts](agenttest.md): the Host the suite expects.
- [Fixture homes](fixtures.md): homes with real files to run it on.
- [Config-defined agents](../capabilities/fromspec.md): agents you define from a `Spec`.
- [Capability accessors](../capabilities/accessors.md): what `Supported` and `Unsupported` report.
- [examples/testing](../../examples/testing/) and the [feature index](../reference/features.md#testing-agenttest-and-fixtures).
- [Choosing guides](../choosing/README.md).
