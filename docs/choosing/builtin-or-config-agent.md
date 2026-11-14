# Built-in or config-defined agent

agentio knows two kinds of agent. A **built-in** agent (Claude Code, Codex) is a vendor module written against real files of that CLI: it reads settings, logins, usage, conversations and live sessions. A **config-defined** agent is an `agentio.Spec` value (binary name, home folder, home variable, argv templates) that you can load from your own config file. It gets the core every agent shares, and nothing that needs code.

## Decision table

| Capability | Built-in (`claude`, `codex`) | Config-defined (`agentio.Spec`) |
|---|---|---|
| Homes: default, home variable, extra-home glob, markers | ✅ | ✅ from `DefaultHome`, `HomeEnv`, `ExtraHomes`, `HomeMarkers` |
| Find the binary on `PATH` | ✅ | ✅ from `Binary` |
| Launch and resume commands | ✅ typed vendor options plus neutral `LaunchRequest` | ✅ neutral `LaunchRequest` from `NewArgs`, `ResumeArgs`, `ModelArgs`, `AddDirArgs`, `PromptArgs` |
| Autonomy levels, headless runs, token logins | ✅ | ❌ `ErrUnsupported` |
| Login command | ✅ every vendor flow | ✅ browser flow only, when `LoginArgs` is set |
| `--version` through a Runner | ✅ | ✅ when `VersionArgs` is set |
| Recognise its processes | ✅ | ✅ when `Process` is set |
| Link shared files into a new home | ✅ Claude | ✅ when `SharedHomeFiles` is set |
| Doctor checks | ✅ vendor checks | ✅ the core checks (binary, home) |
| Install method, identity, usage, conversations, prompt history, live sessions, config, MCP, skills, plugins, run events | ✅ | ❌ "config-only agent: a spec describes L1 only; this needs code" |
| Guide | [Built-in agents](../capabilities/builtin.md#step-1--build-every-agent) | [Config-defined agents](../capabilities/fromspec.md#step-1--describe-the-agent-as-a-spec) |
| Example | [accessors](../../examples/accessors/), [quickstart](../../examples/quickstart/) | [fromspec](../../examples/fromspec/) |

## If this, then that

- If the agent is Claude Code or Codex, use the built-in: `claude.New(h)`, `codex.New(h)`, or both through `builtin.New(h)`.
- If you want a launcher, session picker or home switcher to list another CLI agent next to the built-ins, describe it as a `Spec` and pass it to `builtin.New(h, specs...)`. It appears in `Agents.All()` and `Agents.Custom` after the built-ins, and every accessor loop handles it.
- If you need one config-defined agent on its own, `agentio.FromSpec(spec, h)` builds it without the `builtin` module.
- If your users write the Specs (a TOML or JSON config of your program), call `Spec.Validate()` on each before `builtin.New`: one invalid Spec fails the whole set.
- If you need usage, identity or conversations for a new vendor, a Spec cannot give them. That needs a vendor module modelled on real files of that CLI, like `claude` and `codex`.
- If you want to know what a config-defined agent supports at run time, `agentio.Supported(a)` lists it and `agentio.Unsupported(a, cap)` explains each gap (for example "spec has no version args").

## Pitfalls

- **IDs must be unique.** `builtin.New` refuses a Spec with the ID `claude` or `codex`, or a repeated custom ID, with `ErrInvalidSpec`; a Spec never overrides a built-in. `FromSpec` alone does not check against built-ins.
- **Empty `HomeMarkers` means any folder is a home.** With an `ExtraHomes` glob that can list unrelated folders; name at least one file only a real home has.
- **Use the accessors, never type assertions.** The config-defined agent's Go type implements every data-driven interface; only the accessors check whether the Spec data turns the capability on.
- **Process matching order.** `agentio.ClassifyProcess` returns the first agent that claims a process and built-ins come first, so a broad custom match cannot take Claude Code or Codex processes, but two broad custom matches shadow each other in config order.
- **Without `LogoutArgs`, `LogoutCommand` is a zero `Command`.** Check `Command.Path` before showing a logout action.

## Related

- [Config-defined agents](../capabilities/fromspec.md), [Built-in agents](../capabilities/builtin.md), [Agent, Spec and Core](../core/agent-and-spec.md#step-2--read-an-agents-spec), [Doctor checks](../capabilities/doctor.md)
- Other choices: [Concrete type or accessor](concrete-or-accessor.md), [Which module to import](modules.md)
- Examples: [fromspec](../../examples/fromspec/), [accessors](../../examples/accessors/)
- [Feature index](../reference/features.md#built-in-and-config-defined-agents), [All choosing guides](README.md)
