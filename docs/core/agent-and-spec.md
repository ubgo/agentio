# Agent, Spec and Core

[`agentio.Agent`](ds:block?id=agent-476xnza7) is the interface every agent shares: its ID and name, its binary on PATH, its homes, and building a launch command. A `Spec` is the plain data that describes an agent (binary, home variable and folders, argv templates, credential variables, how to recognise its process), and `Core` is the shared implementation built from a Spec. You want this page when you write code that loops over agents, or when you describe a CLI agentio does not ship by itself.

## Before you start

- Module: `github.com/ubgo/agentio`; the examples also use `github.com/ubgo/agentio/claude`.
- A Host ([Host: the only door to the machine](host.md)).

## Step 1 — Use the Agent interface

Vendor agents (`*claude.Agent`, `*codex.Agent`) are not `agentio.Agent` themselves; they hand out their neutral view with [`Neutral()`](ds:block?id=claude-agent-neutral-eh63bcpc). That keeps vendor method names free for rich vendor types (`claude.Usage` on the vendor agent, `agentio.Usage` on the neutral one). Code that works on any agent takes `agentio.Agent`:

<!-- snippet: examples/quickstart/spec.go#agent-interface -->
```go
// describe works on any agent through the neutral interface: no type
// switch, no vendor import. Only Homes touches the disk.
func describe(p printer, a agentio.Agent) error {
	p.line(fmt.Sprintf("agent: id=%s name=%q", a.ID(), a.Name()))
	p.line(fmt.Sprintf("agent: default home=%s", a.DefaultHome().Dir()))
	if h, ok := a.HomeFromEnv(); ok {
		p.line("agent: env selects " + h.Dir())
	}
	homes, err := a.Homes()
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("agent: %d homes on disk", len(homes)))
	// IDs from config go through ParseID; built-in IDs are typed constants.
	if _, err := agentio.ParseID("Claude"); errors.Is(err, agentio.ErrInvalidID) {
		p.line("agent: ParseID(\"Claude\") refused: " + err.Error())
	}
	return nil
}
```

[The interface has nine methods.](ds:block?id=agent-476xnza7) All are Pure except `LookPath`, `Homes` and `IsHome`, which read the disk:

| Method | Returns | Cost |
|---|---|---|
| `ID()` | `agentio.ID` (`claude`, `codex`, or a config-defined one) | Pure |
| `Name()` | display name (`Claude Code`) | Pure |
| `Spec()` | the Spec the core is built from | Pure |
| `LookPath()` | binary path from `Host.Env`'s PATH, or `ErrNotInstalled` | Disk |
| `DefaultHome()` | the default `Home` | Pure |
| `HomeFromEnv()` | the home the home variable selects, if set | Pure |
| `Homes()` | every home on disk, default first | Disk |
| `IsHome(dir)` | whether `dir` is a home of this agent | Disk |
| `Launch(t, r)` | a `Command` for one invocation | Pure |

Shared capabilities beyond this core (usage, conversations, live sessions …) are reached with typed accessors such as `agentio.UsageOf(a)`, never with type assertions in your code: see [Shared capabilities and accessors](../capabilities/accessors.md).

An `ID` is `[a-z0-9-]`, starts with a letter and is at most 64 bytes, because it ends up in file names and flags. Built-in IDs are typed constants (`claude.ID`, `codex.ID`); IDs that come from config go through [`agentio.ParseID`](ds:block?id=parseid-rfund9ym), which returns an error matching `ErrInvalidID`.

## Step 2 — Read an agent's Spec

Every agent, built-in or not, has a Spec. [Built-ins build their core from their own Spec too](ds:block?id=claude-new-2b7v6ezn), so home discovery, the home variable rule and launch templates are data for every agent:

<!-- snippet: examples/quickstart/spec.go#read-spec -->
```go
// readSpec prints the data Claude's core is built from. A Spec is plain
// data: binary, home variable and folders, home markers, argv templates,
// credential variables and how to recognise the process. Pure.
func readSpec(p printer, a agentio.Agent) error {
	s := a.Spec()
	if err := s.Validate(); err != nil {
		return err
	}
	p.line(fmt.Sprintf("spec: binary=%s home env=%s default home=%s extra homes=%s", s.Binary, s.HomeEnv, s.DefaultHome, s.ExtraHomes))
	p.line(fmt.Sprintf("spec: markers=%v", s.HomeMarkers))
	p.line(fmt.Sprintf("spec: resume template=%v", s.ResumeArgs))
	p.line(fmt.Sprintf("spec: credential env=%v", s.CredentialEnv))
	for _, proc := range []agentio.Process{
		{PID: 1, Argv: []string{"/usr/local/bin/claude", "--resume", "x"}},
		{PID: 2, Exe: "/Applications/Claude.app/Contents/MacOS/claude"},
	} {
		p.line(fmt.Sprintf("spec: process exe=%q argv=%v matches=%t", proc.Exe, proc.Argv, s.Process.Match(proc)))
	}
	return nil
}
```

The fields, grouped:

- **Identity:** `ID`, `Name`, `Binary` (the executable looked up on PATH).
- **Homes:** `HomeEnv` (the variable that selects a non-default home, `""` = one home only), `DefaultHome` (relative to `Host.UserHome`), `ExtraHomes` (a glob such as `.claude-*`), [`HomeMarkers`](ds:block?id=spec-9rwc3qhn) (entries that make a folder a real home), and `HomeEnvNamesParent` when the variable names the folder that contains the home (Gemini CLI's `GEMINI_CLI_HOME`).
- **Launch templates:** `NewArgs`, `ResumeArgs`, `ModelArgs`, `AddDirArgs`, `PromptArgs`, with placeholders `{args}`, `{id}`, `{model}`, `{dir}`, `{prompt}`.
- **Data-driven capabilities:** `VersionArgs` (binary version through a Runner), `LoginArgs` / `LogoutArgs` (login commands), `SharedHomeFiles` (home links).
- **Credentials:** `CredentialEnv` (variables that would override the home's stored login: [scrubbed from a launch on the home's login, and from a token login when they outrank the token](ds:block?id=core-envchanges-f8hrn3we)), `APIBillingEnv` (variables that switch to per-token billing).
- **Process:** `ProcessMatch` with `Basenames`, `ArgvContains` and `Exclude` ([an exclude substring vetoes a match](ds:block?id=processmatch-match-6phmdfg3), which is how the Claude desktop app is told apart from the CLI).

[`Spec.Validate`](ds:block?id=spec-validate-chxkxnh2) checks the ID, that `Binary` is set, that `DefaultHome` is relative and set whenever `HomeEnv` is, that `ExtraHomes` is a valid local glob, that `HomeEnvNamesParent` has a home variable and a one-element `DefaultHome`, that every `TokenEnv` entry has a kind and a variable and no (kind, provider) repeats, and that every non-empty template holds its placeholder. A failure matches `ErrInvalidSpec`.

## Step 3 — Build a Core from a Spec

[`agentio.NewCore(spec, host)`](ds:block?id=newcore-dqu45h82) validates the Spec and binds it to a Host. The result implements the whole L1 core from data: homes, the home variable rule, PATH lookup, launch from the templates, process matching, home links, login commands and the L1 doctor checks.

<!-- snippet: examples/quickstart/spec.go#build-core -->
```go
// toySpec describes a made-up CLI entirely as data.
func toySpec() agentio.Spec {
	return agentio.Spec{
		ID: "toy", Name: "Toy Agent", Binary: "toy",
		HomeEnv: "TOY_HOME", DefaultHome: ".toy", ExtraHomes: ".toy-*",
		HomeMarkers: []string{"config.json"},
		NewArgs:     []string{"{args}"},
		ResumeArgs:  []string{"--resume", "{id}", "{args}"},
		ModelArgs:   []string{"--model", "{model}"},
		PromptArgs:  []string{"{prompt}"},
		LoginArgs:   []string{"login"},
	}
}

// buildCore binds a Spec to a Host and uses the L1 core every agent shares:
// launch from the templates, login command, doctor checks. NewCore
// validates the Spec first.
func buildCore(p printer, h agentio.Host) error {
	if _, err := agentio.NewCore(agentio.Spec{ID: "Toy"}, h); errors.Is(err, agentio.ErrInvalidSpec) {
		p.line("core: invalid spec refused: " + err.Error())
	}
	core, err := agentio.NewCore(toySpec(), h)
	if err != nil {
		return err
	}
	work := core.HomeAt(".toy-work")
	cmd, err := core.Launch(work.Target(), agentio.LaunchRequest{ResumeID: "abc", Model: "fast", Prompt: "hello"})
	if err != nil {
		return err
	}
	p.line("core: launch " + cmd.String())
	p.line("core: env " + strings.Join(onlyVar(cmd.Env, "TOY_HOME"), " "))
	_, err = core.Launch(work.Target(), agentio.LaunchRequest{Headless: true})
	p.line(fmt.Sprintf("core: headless unsupported=%t", errors.Is(err, agentio.ErrUnsupported)))
	login, err := core.LoginCommand(core.DefaultHome(), agentio.LoginBrowser)
	if err != nil {
		return err
	}
	p.line("core: login " + login.String())
	for _, c := range core.Checks(context.Background(), work) {
		p.line(fmt.Sprintf("core: check %s %s: %s", c.ID, c.Status, c.Message))
	}
	return nil
}
```

[`Core.Launch` expands the templates in a fixed order](ds:block?id=core-launch-v8h5ys7c): `Target.ExtraArgs`, then the autonomy and headless args, then the model and add-dir templates, then the request's own `Args`, then the prompt; the result is spliced where `{args}` stands in `NewArgs` (or `ResumeArgs` with `{id}` filled). [`Core.Checks`](ds:block?id=core-checks-8dj5h3dt) reports `binary-found`, `home`, `home-env` (the home variable is set in the base env) and `api-billing-env`; `api-billing-env` names the variables only, never their values, while `home-env` also names the folder the variable selects.

To turn a Spec into a full `agentio.Agent` (with process classification, login commands, token logins and doctor wired as capabilities), use [`agentio.FromSpec`](ds:block?id=fromspec-4tcd24bd): see [Config-defined agents](../capabilities/fromspec.md).

## Try it

```sh
cd examples && go run ./quickstart
```

The lines for this page:

```text
agent: id=claude name="Claude Code"
agent: default home=~/.claude
agent: 2 homes on disk
agent: ParseID("Claude") refused: agentio: invalid agent id: "Claude": only [a-z0-9-] allowed
spec: binary=claude home env=CLAUDE_CONFIG_DIR default home=.claude extra homes=.claude-*
spec: markers=[settings.json projects .credentials.json .claude.json]
spec: resume template=[--resume {id} {args}]
spec: credential env=[CLAUDE_CODE_OAUTH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN]
spec: process exe="" argv=[/usr/local/bin/claude --resume x] matches=true
spec: process exe="/Applications/Claude.app/Contents/MacOS/claude" argv=[] matches=false
core: invalid spec refused: agentio: invalid spec: agentio: invalid agent id: "Toy": only [a-z0-9-] allowed
core: launch toy --resume abc --model fast hello
core: env TOY_HOME=~/.toy-work
core: headless unsupported=true
core: login toy login
core: check binary-found fail: toy not found on PATH
core: check home warn: ~/.toy-work is not a home (missing or no marker)
```

## Pitfalls

- **A vendor agent is not an `agentio.Agent`.** Pass `claude.New(h).Neutral()`, not `claude.New(h)`, to code that takes the interface.
- **A Spec describes L1 only.** Usage, identity and conversations need code; a config-only agent never has them, and [the accessors report them as unsupported](ds:block?id=specagent-unsupportedreason-jp9ga9ft).
- **[A field without a template is refused, not dropped.](ds:block?id=core-launch-v8h5ys7c)** `Core.Launch` returns `ErrUnsupported` for a model, add-dir, prompt or resume when the Spec has no template for it, for a non-default `Autonomy` without an `AutonomyArgs` entry, for `Headless` without `HeadlessArgs`, and for a token `Credential` without a matching `TokenEnv` entry.
- **[`HomeMarkers` empty means any folder is a home.](ds:block?id=core-ishome-nxgj8ve9)** A config agent without markers counts every folder its `ExtraHomes` glob matches.
- **[`HomeEnv` set but empty counts as absent.](ds:block?id=core-homefromenv-jk7mncfj)** `HomeFromEnv` returns false for `CLAUDE_CONFIG_DIR=`, as the vendor CLIs fall back to the default home.
- **[`Command.Path` is the binary name, not a path.](ds:block?id=core-command-m93k4eug)** Building is Pure, so it never walks PATH; resolve it with `LookPath` before you exec.
- **[`LogoutCommand`](ds:block?id=core-logoutcommand-ztcp3jsm) returns a zero `Command`** (not an error) when the Spec has no `LogoutArgs` or the home is foreign; check `Path == ""`.

## Related

- [Homes](homes.md)
- [Targets and commands](launch.md)
- [Shared capabilities and accessors](../capabilities/accessors.md)
- [Config-defined agents](../capabilities/fromspec.md)
- [Built-in agents](../capabilities/builtin.md)
- Example: [`examples/quickstart`](../../examples/quickstart/)
- [Feature index](../reference/features.md)
