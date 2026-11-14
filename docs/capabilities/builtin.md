# Built-in agents

`github.com/ubgo/agentio/builtin` assembles every agent agentio knows on one Host: Claude Code and Codex as typed fields, plus any config-defined agents you describe as `agentio.Spec` values. Use it in a program that supports "every coding agent on this machine"; a program that only needs one vendor imports `claude` or `codex` directly and never downloads the other's dependencies.

## Before you start

- Module: `github.com/ubgo/agentio/builtin`. It imports both vendor modules, so importing it pulls in Codex's TOML dependency too.
- A Host (`agentio.HostFromOS()` in a program, a fixture Host in tests).
- Building is Pure: nothing is read from disk until you call a method.

[`examples/accessors`](../../examples/accessors/) builds its agents with `builtin.New` on copies of the fixture homes.

## Step 1 — Build every agent

`builtin.New(h, custom...)` returns `builtin.Agents` with typed fields `Claude *claude.Agent`, `Codex *codex.Agent` and `Custom []agentio.Agent`. Use the typed fields for vendor calls (they are compile-checked) and `All()` for loops: it returns the neutral view of every agent, Claude first, then Codex, then the custom agents in the order given.

<!-- snippet: examples/accessors/builtin.go#build -->
```go
// buildAgents builds Claude Code and Codex on h. Nothing is read from disk
// yet; the typed fields keep vendor calls compile-checked and All gives the
// neutral views for loops.
func buildAgents(h agentio.Host) (builtin.Agents, error) {
	return builtin.New(h)
}
```

There is no package-level registry and no default agent: the set is the value you hold, built from the Host you pass.

## Step 2 — Find an agent by ID

`ByID(id)` returns the neutral agent with that `agentio.ID`, or `false`. Use it for an ID the user typed or a config file named. The vendor IDs are constants: `claude.ID` and `codex.ID`.

<!-- snippet: examples/accessors/builtin.go#byid -->
```go
// findByID looks an agent up by the ID a user typed or a config file named;
// an unknown ID is false, never a panic or a default agent.
func findByID(w io.Writer, agents builtin.Agents) {
	for _, id := range []agentio.ID{codex.ID, "aider"} {
		a, ok := agents.ByID(id)
		if !ok {
			fmt.Fprintf(w, "%s: no such agent\n", id)
			continue
		}
		fmt.Fprintf(w, "%s: %s\n", id, a.Name())
	}
}
```

## Step 3 — Add config-defined agents

Pass Specs to `builtin.New` to add agents agentio has no code for. Each is built with `agentio.FromSpec` and appended to `Custom`, so `All()`, `ByID`, `agentio.ClassifyProcess` and `agentio.FindLiveSession` see it like a built-in. A Spec whose ID equals a built-in's or repeats an earlier custom ID is an error, never an override.

<!-- snippet: examples/accessors/builtin.go#custom -->
```go
// addCustom passes a config-defined agent to builtin.New. It joins All after
// the built-ins; reusing a built-in ID is an error, never an override.
func addCustom(w io.Writer, h agentio.Host) error {
	aider := agentio.Spec{ID: "aider", Name: "Aider", Binary: "aider", NewArgs: []string{"{args}"}}
	agents, err := builtin.New(h, aider)
	if err != nil {
		return err
	}
	for _, a := range agents.All() {
		fmt.Fprintf(w, "%s (%s)\n", a.ID(), a.Name())
	}
	clash := agentio.Spec{ID: codex.ID, Name: "My Codex", Binary: "codex"}
	if _, err := builtin.New(h, clash); err != nil {
		fmt.Fprintln(w, "refused:", err)
	}
	return nil
}
```

How to describe such an agent: [Config-defined agents](fromspec.md).

## Try it

```sh
cd examples && go run ./accessors
```

The by-id and custom sections of the output:

```text
== by id ==
codex: Codex
aider: no such agent
== custom ==
claude (Claude Code)
codex (Codex)
gemini (Gemini CLI)
pi (pi)
aider (Aider)
refused: builtin: agent id "codex" is already defined: agentio: invalid spec
```

## Pitfalls

- **`builtin` pulls every vendor's dependencies.** A Claude-only program should import `github.com/ubgo/agentio/claude` and call `claude.New(h)`; that never downloads Codex's TOML dependency.
- **ID clashes are errors.** `builtin.New` refuses a custom Spec with the ID `claude` or `codex`, or a repeated custom ID, with `ErrInvalidSpec`. You cannot replace a built-in with a Spec.
- **`All()` returns neutral views.** `agents.All()[0]` is `agents.Claude.Neutral()`, an `agentio.Agent`, not the `*claude.Agent`. Keep the typed field for vendor calls.
- **Order matters for classification.** `agentio.ClassifyProcess` returns the first agent that claims a process; `All()` puts built-ins before custom agents, so a custom Spec with a broad process match cannot steal Claude or Codex processes.
- **Invalid Specs fail the whole build.** `builtin.New` returns the first Spec error (wrapped with the Spec's ID) and no agents; validate user-supplied Specs with `Spec.Validate()` first if you want to skip bad ones.

## Related

- [Config-defined agents](fromspec.md), [Shared capabilities and accessors](accessors.md).
- [Agent, Spec and Core](../core/agent-and-spec.md), [Your first agent](../getting-started/first-agent.md).
- Vendor pages: [Claude agent](../claude/agent.md), [Codex agent](../codex/agent.md).
- [Choosing guides](../choosing/README.md). Example: [`examples/accessors`](../../examples/accessors/). Feature index: [features](../reference/features.md).
