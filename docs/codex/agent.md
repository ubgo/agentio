# The Codex agent

`codex.New` gives you OpenAI Codex CLI on one `agentio.Host`: the typed entry point for everything the `codex` module reads (install, homes, `config.toml`, MCP servers, `auth.json`, usage, rollouts, live sessions) and every command it builds. Use it when your code knows it is talking to Codex and wants the vendor types; call `Neutral()` when the same code also loops over other agents.

## Before you start

- Add the module: `go get github.com/ubgo/agentio/codex`. It pulls the root module and a TOML parser; the SQLite reader lives in the separate [`codex/state`](state.md) module so a caller of `codex` never downloads a SQLite driver.
- Build a `Host`. Real programs call `agentio.HostFromOS()`; tests and these examples build one by hand around a temp user home (see [Host](../core/host.md)).
- A Codex home is a directory holding at least one marker: `config.toml`, `auth.json`, `sessions/`, `state_5.sqlite`, `version.json` or `installation_id`. The default home is `~/.codex`; another home is whatever directory `CODEX_HOME` names. Codex has no `~/.codex-*` convention, so other homes come from you (`HomeAt`).

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/), which runs on a temp copy of the 0.162.1 fixture home.

## Step 1 — Create the agent

`codex.New` does no I/O and never fails. `ID`, `Name` and `Spec` are constants; `HomeEnv` applies the home rule to an environment you pass (`CODEX_HOME` removed for the default home, set for any other), and `InstallationID` reads `<home>/installation_id` as a redacting `agentio.Personal`.

<!-- snippet: examples/codexconfig/agent.go#create-agent -->
```go
// createAgent builds the Codex agent and reads its static facts. New reads
// nothing; InstallationID is one small file read.
func createAgent(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	spec := a.Spec()
	fmt.Fprintf(w, "id=%s name=%s binary=%s home-env=%s default-home=%s\n",
		a.ID(), a.Name(), spec.Binary, spec.HomeEnv, spec.DefaultHome)

	h := a.DefaultHome()
	env := a.HomeEnv([]string{"PATH=/usr/bin", codex.EnvHome + "=/somewhere"}, h)
	_, set := agentio.EnvLookup(env, codex.EnvHome)
	fmt.Fprintf(w, "default home keeps CODEX_HOME in the child env: %v\n", set)

	id, err := a.InstallationID(h)
	if err != nil {
		return err
	}
	// InstallationID is Personal: printing it shows a redacted form.
	fmt.Fprintf(w, "installation id: %v\n", id)
	return nil
}
```

## Step 2 — Pick a home

Every reader takes a `codex.Home`, never a bare path, so a Claude home cannot be passed by mistake.

| Call | Returns | Cost |
|---|---|---|
| `a.DefaultHome()` | `~/.codex` (launched with `CODEX_HOME` absent) | Pure |
| `a.Homes()` | the default home and the `CODEX_HOME` one, only those holding a marker | Disk read |
| `a.HomeFromEnv()` | the home `CODEX_HOME` in `Host.Env` selects; `false` when unset | Pure |
| `a.HomeAt(dir)` | a caller-named home; `agentio.ErrNotFound` when `dir` holds no marker | Disk read |
| `a.IsHome(dir)` | whether `dir` holds any marker | Disk read |
| `a.HomeOf(n)` | converts a neutral `agentio.Home`; `ErrForeignHome` for another agent's | Pure |

`Home.Dir()`, `Home.Name()` (`personal` for the default), `Home.IsDefault()`, `Home.Neutral()` and `Home.Target()` (a plain launch target, version unknown) are all pure.

<!-- snippet: examples/codexconfig/agent.go#pick-home -->
```go
// pickHome shows every way to get a codex.Home: the default, the homes
// on disk, the one CODEX_HOME names, a caller-named directory, and the
// conversion from a neutral agentio.Home.
func pickHome(w io.Writer, m machine) error {
	a := codex.New(m.host)
	def := a.DefaultHome()
	fmt.Fprintf(w, "default: %s (default=%v)\n", m.rel(def.Dir()), def.IsDefault())

	homes, err := a.Homes()
	if err != nil {
		return err
	}
	for _, h := range homes {
		fmt.Fprintf(w, "found home %q at %s\n", h.Name(), m.rel(h.Dir()))
	}

	_, fromEnv := a.HomeFromEnv()
	fmt.Fprintf(w, "CODEX_HOME set: %v\n", fromEnv)

	ok, err := a.IsHome(def.Dir())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "is a home: %v\n", ok)

	if _, err := a.HomeAt(m.project); err != nil {
		fmt.Fprintf(w, "project dir is not a home: %v\n", errors.Is(err, agentio.ErrNotFound))
	}

	back, err := a.HomeOf(def.Neutral())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "neutral round trip: %v\n", back.Dir() == def.Dir())
	return nil
}
```

## Step 3 — Get the neutral view

`Neutral()` returns an `agentio.Agent`. Code that handles several agents reaches each capability through a typed accessor (`agentio.DoctorOf`, `agentio.UsageOf`, …) and asks `agentio.Unsupported` why a capability is missing. Codex declares the conversation mover unsupported, with a reason.

<!-- snippet: examples/codexconfig/agent.go#neutral -->
```go
// neutralView is the agentio.Agent view of Codex, for code that loops
// over agents: capabilities are reached through typed accessors.
func neutralView(w io.Writer, host agentio.Host) {
	n := codex.New(host).Neutral()
	fmt.Fprintf(w, "neutral id=%s capabilities=%d\n", n.ID(), len(agentio.Supported(n)))
	if reason, declared := agentio.Unsupported(n, agentio.CapConversationMover); declared {
		fmt.Fprintf(w, "conversation mover unsupported: %s\n", reason)
	}
	if d, ok := agentio.DoctorOf(n); ok {
		fmt.Fprintf(w, "doctor checks via the neutral view: %d\n", len(d.Checks(context.Background(), n.DefaultHome())))
	}
}
```

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== agent` section:

```text
== agent
id=codex name=Codex binary=codex home-env=CODEX_HOME default-home=.codex
default home keeps CODEX_HOME in the child env: false
installation id: [redacted]
default: ~/.codex (default=true)
found home "personal" at ~/.codex
CODEX_HOME set: false
is a home: true
project dir is not a home: true
neutral round trip: true
neutral id=codex capabilities=22
conversation mover unsupported: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
doctor checks via the neutral view: 7
```

## Pitfalls

- **The default home is "variable absent", not `CODEX_HOME=~/.codex`.** `HomeEnv` and every command builder remove `CODEX_HOME` for the default home and set it for any other. Do not add it yourself through `Target.ExtraEnv`; the home rule is applied after it and wins.
- **`Homes()` finds at most two homes.** Unlike Claude Code there is no `~/.codex-<name>` convention, so a second home on disk is invisible until you name it with `HomeAt`.
- **`HomeAt` checks for a marker.** An empty directory, or a project folder that only has `.codex/config.toml` inside it, is not a home and returns `agentio.ErrNotFound`.
- **`codex.Agent` is not an `agentio.Agent`.** Pass `a.Neutral()` where the neutral interface is expected; the neutral view refuses another agent's home with `agentio.ErrForeignHome`.
- **`Writer()` has no methods yet.** Codex changes go through vendor commands (`FeatureCommand`, `MCPAddCommand`, `Archive`), so the write door exists but is empty.

## Related

- [Codex install and updates](install.md), [Codex paths and inventory](paths.md), [Codex config.toml](config.md)
- [Agent and Spec](../core/agent-and-spec.md), [Homes](../core/homes.md), [Typed accessors](../capabilities/accessors.md)
- [The Claude agent](../claude/agent.md) for the same shape on Claude Code
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
