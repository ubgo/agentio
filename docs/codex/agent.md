# The Codex agent

[`codex.New`](ds:block?id=codex-new-cv22qtg5) gives you OpenAI Codex CLI on one `agentio.Host`: the typed entry point for everything the `codex` module reads (install, homes, `config.toml`, MCP servers, `auth.json`, usage, rollouts, live sessions) and every command it builds. Use it when your code knows it is talking to Codex and wants the vendor types; call `Neutral()` when the same code also loops over other agents.

## Before you start

- Add the module: `go get github.com/ubgo/agentio/codex`. It pulls the root module and a TOML parser; the SQLite reader lives in the separate [`codex/state`](state.md) module so a caller of `codex` never downloads a SQLite driver.
- Build a `Host`. Real programs call `agentio.HostFromOS()`; tests and these examples build one by hand around a temp user home (see [Host](../core/host.md)).
- A Codex home is a directory holding at least one marker: `config.toml`, `auth.json`, `sessions/`, `state_5.sqlite`, `version.json` or [`installation_id`](ds:block?id=codex-spec-ut8me6cu). The first four are names pi and other tools write too: `IsHome` on a folder other than `~/.codex` or the `CODEX_HOME` one needs `state_5.sqlite` or `installation_id` (see [Homes](../core/homes.md#step-2--list-every-home)); `HomeAt` takes any marker. The default home is [`~/.codex`](ds:block?id=codex-agent-defaulthome-54qemjw6); another home is whatever directory [`CODEX_HOME`](ds:block?id=codex-agent-homefromenv-hgwbx2cn) names. Codex has no `~/.codex-*` convention, so other homes come from you ([`HomeAt`](ds:block?id=codex-agent-homes-nffucnw6)).

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/), which runs on a temp copy of the 0.162.1 fixture home.

## Step 1 — Create the agent

[`codex.New`](ds:block?id=codex-new-cv22qtg5) does no I/O and never fails. `ID`, `Name` and `Spec` are constants; [`HomeEnv`](ds:block?id=codex-agent-homeenv-rtwbj8nt) applies the home rule to an environment you pass (`CODEX_HOME` removed for the default home, set for any other), and [`InstallationID`](ds:block?id=codex-agent-installationid-dp47g2yq) reads `<home>/installation_id` as a redacting `agentio.Personal`.

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

Every reader takes a [`codex.Home`](ds:block?id=codex-home-hsdrs8z7), never a bare path, so a Claude home cannot be passed by mistake.

| Call | Returns | Cost |
|---|---|---|
| [`a.DefaultHome()`](ds:block?id=codex-agent-defaulthome-54qemjw6) | `~/.codex` (launched with `CODEX_HOME` absent) | Pure |
| [`a.Homes()`](ds:block?id=codex-agent-homes-nffucnw6) | the default home and the `CODEX_HOME` one, only those holding a marker | Disk read |
| [`a.HomeFromEnv()`](ds:block?id=codex-agent-homefromenv-hgwbx2cn) | the home `CODEX_HOME` in `Host.Env` selects; `false` when unset | Pure |
| [`a.HomeAt(dir)`](ds:block?id=codex-agent-homeat-5kpz3t54) | a caller-named home; `agentio.ErrNotFound` when `dir` holds no marker | Disk read |
| [`a.IsHome(dir)`](ds:block?id=codex-agent-ishome-28msd42y) | whether `dir` holds a marker (a distinctive one outside `~/.codex` and `CODEX_HOME`) | Disk read |
| [`a.HomeOf(n)`](ds:block?id=codex-agent-homeof-xjk5xqdc) | converts a neutral `agentio.Home`; `ErrForeignHome` for another agent's | Pure |

`Home.Dir()`, `Home.Name()` ([`personal`](ds:block?id=defaulthomename-2x8teypk) for the default), `Home.IsDefault()`, `Home.Neutral()` and [`Home.Target()`](ds:block?id=codex-home-target-4s8v9g29) (a plain launch target, version unknown) are all pure.

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

[`Neutral()`](ds:block?id=codex-agent-neutral-hbg9hqd3) returns an `agentio.Agent`. Code that handles several agents reaches each capability through a typed accessor (`agentio.DoctorOf`, `agentio.UsageOf`, …) and asks `agentio.Unsupported` why a capability is missing. [Codex declares the conversation mover unsupported, with a reason.](ds:block?id=codex-neutral-unsupportedreason-evgu3az7)

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
neutral id=codex capabilities=27
conversation mover unsupported: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
doctor checks via the neutral view: 7
```

## Pitfalls

- **The default home is "variable absent", not `CODEX_HOME=~/.codex`.** [`HomeEnv`](ds:block?id=codex-agent-homeenv-rtwbj8nt) and every command builder remove `CODEX_HOME` for the default home and set it for any other. Do not add it yourself through [`Target.ExtraEnv`](ds:block?id=core-envchanges-f8hrn3we); the home rule is applied after it and wins.
- **[`Homes()`](ds:block?id=codex-agent-homes-nffucnw6) finds at most two homes.** Unlike Claude Code there is no `~/.codex-<name>` convention, so a second home on disk is invisible until you name it with `HomeAt`.
- **[`HomeAt`](ds:block?id=codex-agent-homeat-5kpz3t54) checks for a marker.** An empty directory, or a project folder that only has `.codex/config.toml` inside it, is not a home and returns `agentio.ErrNotFound`.
- **`codex.Agent` is not an `agentio.Agent`.** Pass [`a.Neutral()`](ds:block?id=codex-agent-neutral-hbg9hqd3) where the neutral interface is expected; the neutral view refuses another agent's home with [`agentio.ErrForeignHome`](ds:block?id=codex-agent-homeof-xjk5xqdc).
- **[`Writer()`](ds:block?id=codex-writer-tax23vch) has no methods yet.** Codex changes go through vendor commands (`FeatureCommand`, `MCPAddCommand`, `Archive`), so the write door exists but is empty.

## Related

- [Codex install and updates](install.md), [Codex paths and inventory](paths.md), [Codex config.toml](config.md)
- [Agent and Spec](../core/agent-and-spec.md), [Homes](../core/homes.md), [Typed accessors](../capabilities/accessors.md)
- [The Claude agent](../claude/agent.md) for the same shape on Claude Code
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
