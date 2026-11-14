# Shared capabilities and accessors

A capability is one thing an agent can do beyond the core `Agent` interface: read its install, read cached usage, list conversations, list MCP servers, and so on. agentio defines each capability once as a small interface in the root module (`agentio.CachedUsageReader`, `agentio.MCPLister`, ...) and gives each one a typed accessor function (`agentio.CachedUsageOf`, `agentio.MCPServersOf`, ...). Use the accessors whenever your code loops over agents it does not know by type: you get a compile-checked interface or `false`, never a type assertion and never `any`.

## Before you start

- Modules: `github.com/ubgo/agentio` for the accessors; `github.com/ubgo/agentio/builtin` (or `claude` / `codex` directly) for the agents.
- An `agentio.Agent` value: `claude.New(h).Neutral()`, `codex.New(h).Neutral()`, an entry of `builtin.Agents.All()`, or a config-defined agent from `agentio.FromSpec`.
- Nothing on disk is needed to ask which capabilities exist; the capability calls themselves read the homes.

The example for this page, [`examples/accessors`](../../examples/accessors/), copies the recorded Claude Code 2.1.325 and Codex 0.162.1 fixture homes into a temporary user home and runs every step below on them.

## Step 1 — Loop over agents with an accessor

Code that knows the agent uses the vendor type (`*claude.Agent`) and its rich methods. Code that loops over agents uses the accessor: it returns the capability interface and `true`, or the zero value and `false` when the agent does not have it. Skip on `false`; there is nothing to recover from.

<!-- snippet: examples/accessors/main.go#loop -->
```go
// listHomes loops over every agent and every home with no type assertion:
// Homes is on the Agent interface, cached usage is reached through its
// typed accessor and skipped for an agent that does not have it.
func listHomes(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			line := fmt.Sprintf("%s %s %s", a.ID(), home.Name(), tidy(h, home.Dir()))
			if cu, ok := agentio.CachedUsageOf(a); ok {
				u, found, err := cu.CachedUsage(home)
				if err != nil {
					return err
				}
				if found {
					line += fmt.Sprintf(" (cached usage %.0f%%)", u.MaxPercent())
				}
			}
			fmt.Fprintln(w, line)
		}
	}
	return nil
}
```

`Homes` is on the `Agent` interface itself, so it needs no accessor. Every accessor is Pure: asking costs nothing, only the capability's own methods read disk, run a process or use the network.

## Step 2 — List supported capabilities

`agentio.Supported(a)` returns every `Capability` the agent has, in the canonical order of `agentio.CapabilityValues()`. It calls each accessor from one internal table, so the list can never disagree with what the accessors return.

<!-- snippet: examples/accessors/main.go#supported -->
```go
// listSupported prints every capability an agent has, from the one table
// Supported reads; the count makes the difference between agents visible.
func listSupported(w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		caps := agentio.Supported(a)
		fmt.Fprintf(w, "%s: %d capabilities\n", a.ID(), len(caps))
		fmt.Fprintf(w, "  %v\n", caps)
	}
}
```

`Capability` is a closed enum: `String`, `MarshalText` and `ParseCapability` give you the wire spelling (`cached-usage`, `mcp-servers`) for config files and JSON.

## Step 3 — Ask why a capability is missing

`agentio.Unsupported(a, c)` returns the reason an agent declares for a capability it lacks. Agents that can explain implement `agentio.UnsupportedReasoner`; `declared` is `false` when the agent says nothing. Show the reason to users instead of a bare "not supported".

<!-- snippet: examples/accessors/main.go#unsupported -->
```go
// listUnsupported prints, for every capability an agent lacks, the reason
// it declares; "(no reason declared)" when the agent says nothing.
func listUnsupported(w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		have := map[agentio.Capability]bool{}
		for _, c := range agentio.Supported(a) {
			have[c] = true
		}
		for _, c := range agentio.CapabilityValues() {
			if have[c] {
				continue
			}
			reason, declared := agentio.Unsupported(a, c)
			if !declared {
				reason = "(no reason declared)"
			}
			fmt.Fprintf(w, "%s lacks %s: %s\n", a.ID(), c, reason)
		}
	}
}
```

Some agents decide their capabilities from data rather than from their Go type. Those implement `agentio.CapabilityGate`: the accessor first checks the interface, then asks `HasCapability(c)`. A config-defined agent built by `agentio.FromSpec` is the main case: it has `LoginCommander` only when its Spec has `LoginArgs` (see [Config-defined agents](fromspec.md)). You never call `HasCapability` yourself; the accessors and `Supported` already respect it.

## Step 4 — Check what a capability costs

Every capability has a worst-case `Cost`: a `CostClass` (`pure`, `disk`, `process`, `network`, `quota`), an `Access` (`read` or `write`), and `MayTouchLogin` for calls that run the vendor binary. Use it to decide what a loop may do without asking the user.

<!-- snippet: examples/accessors/main.go#cost -->
```go
// listCosts prints the worst-case cost of every capability: a caller that
// loops over homes skips anything that is not Pure or Disk unless the user
// asked for it.
func listCosts(w io.Writer) {
	for _, c := range agentio.CapabilityValues() {
		cost := c.Cost()
		line := fmt.Sprintf("%-20s %s %s", c, cost.Class, cost.Access)
		if cost.MayTouchLogin {
			line += " (may touch login)"
		}
		fmt.Fprintln(w, line)
	}
}
```

A reasonable policy: in a loop over every home, call only `pure` and `disk` reads; call `process` reads once; call `network` reads on demand and cache the answer; call `quota` only when the user asks; reach `write` capabilities only through an explicit user action. The doors in [Doors: Runner, Prober and Writer](doors.md) apply the same split to vendor-specific calls.

## Try it

```sh
cd examples && go run ./accessors
```

The first four sections of the output:

```text
== homes ==
claude personal ~/.claude (cached usage 6%)
claude work ~/.claude-work
codex personal ~/.codex (cached usage 12%)
== supported ==
claude: 26 capabilities
  [installer binary-version identity usage cached-usage subscription token-login token-prober login-commander conversations history-checker history-mover conversation-mover prompt-history live-sessions process-classifier turns home-linker instruction-files mcp-servers skills plugins config run-events doctor live-credential]
codex: 22 capabilities
  [installer binary-version identity usage cached-usage subscription login-commander conversations history-checker prompt-history live-sessions process-classifier turns home-linker instruction-files mcp-servers skills plugins config models run-events doctor]
gemini: 14 capabilities
  [installer binary-version identity token-login conversations history-checker process-classifier home-linker instruction-files mcp-servers skills plugins config doctor]
pi: 13 capabilities
  [installer binary-version identity token-login conversations history-checker process-classifier turns instruction-files mcp-servers skills config doctor]
== unsupported ==
claude lacks models: Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot
codex lacks token-login: Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
codex lacks token-prober: Codex has no quota probe: no token login to probe
codex lacks history-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
codex lacks conversation-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
codex lacks live-credential: Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting
gemini lacks usage: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
gemini lacks cached-usage: Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
gemini lacks subscription: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
gemini lacks token-prober: probing a Gemini key spends quota through a model call that is not modelled
gemini lacks login-commander: Gemini CLI has no login subcommand; it asks for the auth method in its first interactive run
gemini lacks history-mover: Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
gemini lacks conversation-mover: Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
gemini lacks prompt-history: the per-project prompt log (tmp/<slug>/logs.json) is recorded only from source, never from a real run
gemini lacks live-sessions: Gemini CLI writes no session lock or pid file to join a process to a chat
gemini lacks turns: Gemini chats rewrite earlier records ($rewindTo, $patch), so an offset-based turn reader would report dropped turns
gemini lacks models: Gemini CLI keeps its model catalog in its own defaults, not in a file on disk
gemini lacks run-events: the --output-format stream-json event shape is not recorded from a real run yet
gemini lacks live-credential: Gemini CLI reads its login (oauth_creds.json, the keychain or GEMINI_API_KEY) at start and refreshes OAuth itself; it has no setting that re-reads a key from a caller's command
pi lacks usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
pi lacks cached-usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
pi lacks subscription: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
pi lacks token-prober: pi has no plan or quota to probe
pi lacks login-commander: pi logs in with /login inside its interactive UI; there is no login subcommand to build
pi lacks history-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
pi lacks conversation-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
pi lacks prompt-history: pi keeps no prompt history file; prompts live only in session files
pi lacks live-sessions: pi writes no live-session registry; only the process table shows a running pi
pi lacks home-linker: pi has no documented set of files to share between agent dirs
pi lacks plugins: pi packages and extensions are code loaded into the process, not plugins with a manifest this library models
pi lacks models: pi's model catalog (models-store.json) has no recorded shape yet
pi lacks run-events: pi's --mode json event stream is recorded once but not yet modelled as a decoder
pi lacks live-credential: pi can read a key from a !command in auth.json, but when it re-runs that command is not documented or recorded
== cost ==
installer            disk read
binary-version       process read (may touch login)
identity             process read
usage                network read
cached-usage         disk read
subscription         network read
token-login          pure read
token-prober         quota read
login-commander      pure read
conversations        disk read
history-checker      disk read
history-mover        disk write
conversation-mover   disk write
prompt-history       disk read
live-sessions        process read
process-classifier   pure read
turns                disk read
home-linker          disk write
instruction-files    disk read
mcp-servers          disk read
skills               disk read
plugins              disk read
config               disk read
models               disk read
run-events           pure read
doctor               disk read
live-credential      pure read
```

## Pitfalls

- **Vendor agents are not `agentio.Agent`s.** `*claude.Agent` and `*codex.Agent` hand out their neutral view with `Neutral()`. Passing the vendor pointer to an accessor does not compile; passing `a.Neutral()` does. This keeps vendor method names (`Usage` returning `claude.Usage`) free of the neutral signatures.
- **Do not type-assert yourself.** `a.(agentio.LoginCommander)` succeeds on a config-defined agent even when its Spec has no login args; only the accessor consults `CapabilityGate`. Always go through `XxxOf`.
- **`Cost` is the worst case of the capability, not of one call.** `identity` is listed as `process` because Claude may read the macOS keychain; Codex's identity is a plain disk read. Treat the listed class as the upper bound.
- **`Unsupported` on a capability the agent has** returns `("", false)`. Check `Supported` (or the accessor) first, as the example does.
- **A capability can be present and still fail per home.** `UsageOf` is true for Claude, but a logged-out home returns `ErrNotLoggedIn` and an expired login `ErrExpired`. Present means "this agent can", not "this home will".
- **`Homes` lists only directories with marker files.** A configured but empty home folder is not listed; see [Homes](../core/homes.md).

## Related

- [Install, version and identity](install-identity.md), [Usage and subscription](usage.md), [Config, MCP, instructions, skills, plugins and models](config.md), [Doctor checks](doctor.md): the capabilities this example walks.
- [Doors: Runner, Prober and Writer](doors.md): the explicit opt-in for process, quota and write calls.
- [Built-in agents](builtin.md) and [Config-defined agents](fromspec.md): where the agents in the loop come from.
- [Agent, Spec and Core](../core/agent-and-spec.md): the core interface every agent has.
- [Choosing guides](../choosing/README.md).
- Example: [`examples/accessors`](../../examples/accessors/). Feature index: [features](../reference/features.md).
