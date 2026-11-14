# Concrete vendor type or neutral accessor

agentio gives you two ways to reach the same data. The concrete vendor types (`*claude.Agent`, `*codex.Agent`) expose everything the vendor module models, with vendor types in and out. The neutral `agentio.Agent` plus the typed accessors (`agentio.UsageOf(a)`, `agentio.ConversationsOf(a)` and the rest) expose only what every agent shares, so one loop serves Claude Code, Codex and config-defined agents. Most real programs use both: the neutral loop for the overview, the concrete type for the one vendor detail the overview cannot show.

## Decision table

| Your code | Use | You get | Guide | Example |
|---|---|---|---|---|
| Only ever talks to Claude Code | `claude.New(h)` → `*claude.Agent` | the full L3 API: settings layers, hooks, transcripts, stream-json, Writer | [The Claude agent](../claude/agent.md#step-1--create-the-agent) | [claudesettings](../../examples/claudesettings/) |
| Only ever talks to Codex | `codex.New(h)` → `*codex.Agent` | `config.toml`, profiles, rollouts, exec events, thread locks | [The Codex agent](../codex/agent.md#step-1--create-the-agent) | [codexconfig](../../examples/codexconfig/) |
| Loops over every agent on the machine | `builtin.New(h)`, `Agents.All()`, then `agentio.XOf(a)` | one code path; [an agent without the capability is skipped (`ok == false`)](ds:block?id=capabilityof-t96bhbpq) | [Shared capabilities and accessors](../capabilities/accessors.md#step-1--loop-over-agents-with-an-accessor) | [accessors](../../examples/accessors/) |
| Loops, but needs one vendor detail | the neutral loop plus `agents.Claude` / `agents.Codex` | typed vendor fields next to the neutral list | [Built-in agents](../capabilities/builtin.md#step-1--build-every-agent) | [accessors](../../examples/accessors/) |
| Takes agents chosen at run time (by ID, from config) | [`Agents.ByID(id)`](ds:block?id=builtin-agents-byid-3a82nzqs) → `agentio.Agent` | the neutral view only | [Built-in agents](../capabilities/builtin.md#step-2--find-an-agent-by-id) | [accessors](../../examples/accessors/) |
| Handles a config-defined agent | `agentio.FromSpec` or `builtin.New(h, specs...)` | accessors only: no vendor type exists for it | [Config-defined agents](../capabilities/fromspec.md#step-2--build-it-with-fromspec) | [fromspec](../../examples/fromspec/) |
| Has a vendor agent and wants to hand it to neutral code | [`c.Neutral()`](ds:block?id=claude-agent-neutral-eh63bcpc) | the `agentio.Agent` view of the same agent | [The Claude agent](../claude/agent.md#step-4--get-the-neutral-view) | [claudesettings](../../examples/claudesettings/) |

## If this, then that

- If you need a field that only one vendor has ([Claude `Limits` and `Extra` on usage](ds:block?id=claude-usage-sbsaf2c4), Codex profiles, Claude hook matchers), use the concrete type. The neutral types never grow vendor fields.
- If the same code must work for an agent added next year, write it against `agentio.Agent` and accessors. A new vendor module or a `Spec` plugs in without changes to your loop.
- If you hold a neutral `agentio.Home` from the loop and need the vendor home, convert it with [`c.HomeOf(n)`](ds:block?id=claude-agent-homeof-qtt8m9qe) (`claude` and `codex` both have it); going the other way, every vendor `Home` has `Neutral()`.
- If you hold a rich vendor result and need the neutral one, call its `Neutral()`: `claude.Usage`, `codex.Usage`, [`claude.Conversation`](ds:block?id=claude-conversation-neutral-3uhu3gfe), `codex.Conversation`, `claude.Identity`, `codex.Identity`, `claude.StreamMessage` and others all have it.
- If you hold only the neutral `agentio.Usage` and want Claude's extra-usage object, [`claude.ExtraUsageOf(u)`](ds:block?id=claude-extrausageof-g9ejxj4p) decodes it from `u.Raw` without a second request.
- If you need to know why an accessor said no, ask [`agentio.Unsupported(a, cap)`](ds:block?id=unsupported-zhw69bda); to list everything an agent can do, `agentio.Supported(a)`.

## Both in one function

The loop uses only accessors and never names a vendor; the last block uses the concrete `*claude.Agent` that `builtin.Agents` keeps, so the rich result is compile-checked.

```go
// report prints cached usage for every home of every agent through the
// neutral accessor, then the Claude-only extra usage through the concrete
// type. The loop needs no type assertion; the vendor detail needs no loop.
func report(w io.Writer, h agentio.Host) error {
	agents, err := builtin.New(h)
	if err != nil {
		return err
	}
	for _, a := range agents.All() {
		cu, ok := agentio.CachedUsageOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapCachedUsage)
			fmt.Fprintf(w, "%s: no cached usage (%s)\n", a.ID(), reason)
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
			if found {
				fmt.Fprintf(w, "%s %s: %.0f%%\n", a.ID(), home.Name(), u.MaxPercent())
			}
		}
	}

	// Vendor detail: the concrete field is a *claude.Agent, so the rich
	// result (claude.Usage with Limits and Extra) is compile-checked.
	c := agents.Claude
	u, found, err := c.CachedUsage(c.DefaultHome())
	if err != nil {
		return err
	}
	if found {
		fmt.Fprintf(w, "claude limits on disk: %d\n", len(u.Limits))
	}
	return nil
}
```

On the Claude 2.1.325 fixture home this prints `claude personal: 6%` and `claude limits on disk: 3`.

## Pitfalls

- **Do not type-assert the neutral view back.** [`c.Neutral()`](ds:block?id=claude-agent-neutral-eh63bcpc) returns a wrapper, not the `*claude.Agent`, so `a.(*claude.Agent)` fails. Keep the concrete value (or `builtin.Agents`) next to the neutral list instead.
- **Neutral methods take `agentio.Home`, vendor methods take the vendor `Home`.** Passing a Codex home to `claude.Agent.HomeOf` returns [`agentio.ErrForeignHome`](ds:block?id=claude-agent-homeof-qtt8m9qe); this is the check that stops you reading one vendor's home with another's reader.
- **`ok == false` has two causes.** The agent does not implement the interface ([Claude has no model catalog](ds:block?id=claude-neutral-unsupportedreason-t6rggqu7), so `ModelsOf` is false), or it implements it but gates it off from data (a config-defined agent without [`VersionArgs`](ds:block?id=specagent-hascapability-tzc86rcg) has no `BinaryVersion`). `agentio.Unsupported` explains both.
- **An accessor is free; its methods are not.** `UsageOf(a)` costs nothing, `Usage(ctx, h)` is a network read that may return [`ErrRateLimited`](ds:block?id=claude-agent-get-8w45njgb). [`Capability.Cost()`](ds:block?id=capability-cost-u64h92wf) tells you the worst case before you call; see [which usage to read](live-or-cached-usage.md).
- **Layers are highest precedence first for every vendor.** [`ConfigReader.ConfigLayers`](ds:block?id=configreader-y3cxtpm2) lists the winning layer first (Claude: managed, CLI, local, project, user; Codex: legacy managed, session, project, profile, user, system), so the first layer that sets a value is the one in effect. Use `Scope` to label a layer.
- **Neutral results are summaries.** [`agentio.Conversation`](ds:block?id=conversation-7mywvcsh) has title, folder and times; the full transcript or rollout reader is vendor API ([Claude transcripts](../claude/transcript.md), [Codex rollouts](../codex/rollout.md)).

## Related

- [Shared capabilities and accessors](../capabilities/accessors.md), [Built-in agents](../capabilities/builtin.md), [Agent, Spec and Core](../core/agent-and-spec.md)
- Other choices: [Which module to import](modules.md), [Built-in or config-defined agent](builtin-or-config-agent.md), [Which door](doors.md)
- Examples: [accessors](../../examples/accessors/), [claudesettings](../../examples/claudesettings/), [codexconfig](../../examples/codexconfig/)
- [Feature index](../reference/features.md), [All choosing guides](README.md)
