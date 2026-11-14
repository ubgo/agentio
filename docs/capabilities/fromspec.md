# Config-defined agents

Not every coding agent needs Go code. If an agent is a binary on `PATH` with a config folder in your home, a way to resume a conversation and a recognisable process, you can describe it as an `agentio.Spec` and build it with `agentio.FromSpec`. The result is an `agentio.Agent` like the built-ins: it lists its homes, builds launch, resume and login commands, classifies its processes, links shared files into a new home and runs the basic doctor checks. Usage, identity and conversations need code, so a config-defined agent does not have them.

## Before you start

- Module: `github.com/ubgo/agentio` (stdlib only).
- A Host. Nothing is read when the agent is built; `Homes`, `LookPath` and the doctor read disk later.
- To mix it with the built-ins, pass the Spec to `builtin.New` instead (see [Built-in agents](builtin.md#step-3--add-config-defined-agents)).

[`examples/fromspec`](../../examples/fromspec/) describes a made-up agent, "Acme Code" (binary `acme`, homes `~/.acme` and `~/.acme-*`), in a temporary folder with an inert fake binary. Nothing is executed.

## Step 1 — Describe the agent as a Spec

A Spec is plain data. The fields that matter most:

- `ID`, `Name`, `Binary`: identity and the executable looked up on `PATH`.
- `DefaultHome` (relative to the user home), `ExtraHomes` (a glob such as `.acme-*`), `HomeMarkers` (entries that make a folder a real home) and `HomeEnv` (the variable that selects a non-default home).
- Argv templates: `NewArgs` and `ResumeArgs` with `{args}` (where the computed args go; appended when absent) and `{id}`; optional `ModelArgs` (`{model}`), `AddDirArgs` (`{dir}`), `PromptArgs` (`{prompt}`).
- `VersionArgs`, `LoginArgs`, `LogoutArgs`, `SharedHomeFiles`: each enables one data-driven capability.
- `CredentialEnv` (inherited copies are scrubbed from every launch), `APIBillingEnv` (variables that switch to per-token billing) and `Process` (a `ProcessMatch` of `Basenames`, `ArgvContains` and `Exclude`).

<!-- snippet: examples/fromspec/main.go#spec -->
```go
// acmeSpec describes Acme Code as data: binary, homes, argv templates
// ({args}, {id}, {model}, {prompt}), version and login args, files a new
// home shares with the default one, credential variables and how to
// recognise its process.
func acmeSpec() agentio.Spec {
	return agentio.Spec{
		ID:              acmeID,
		Name:            "Acme Code",
		Binary:          acmeBinary,
		HomeEnv:         acmeHomeEnv,
		DefaultHome:     acmeDefault,
		ExtraHomes:      ".acme-*",
		HomeMarkers:     []string{acmeConfig},
		NewArgs:         []string{"{args}"},
		ResumeArgs:      []string{"--resume", "{id}", "{args}"},
		ModelArgs:       []string{"--model", "{model}"},
		PromptArgs:      []string{"--message", "{prompt}"},
		VersionArgs:     []string{"--version"},
		LoginArgs:       []string{"login"},
		LogoutArgs:      []string{"logout"},
		SharedHomeFiles: []string{acmeConfig},
		CredentialEnv:   []string{acmeKeyEnv},
		APIBillingEnv:   []string{acmeKeyEnv},
		Process:         agentio.ProcessMatch{Basenames: []string{acmeBinary}, Exclude: []string{"acme-desktop"}},
	}
}
```

`Spec.Validate()` checks the ID's characters, that `Binary` is set, that `DefaultHome` is relative and set whenever `HomeEnv` is, that `ExtraHomes` is a valid local glob, and that every non-empty template holds its placeholder:

<!-- snippet: examples/fromspec/main.go#validate -->
```go
// validate shows the checks FromSpec runs: a template without its
// placeholder and an absolute default home are refused with ErrInvalidSpec.
func validate(w io.Writer) {
	bad := acmeSpec()
	bad.ResumeArgs = []string{"--resume"}
	fmt.Fprintln(w, bad.Validate())
	bad = acmeSpec()
	bad.DefaultHome = "/etc/acme"
	err := bad.Validate()
	fmt.Fprintln(w, err, errors.Is(err, agentio.ErrInvalidSpec))
}
```

## Step 2 — Build it with FromSpec

`agentio.FromSpec(spec, h)` validates the Spec and returns the agent. Its capabilities follow the data: `BinaryVersion` needs `VersionArgs`, `LoginCommander` needs `LoginArgs`, `HomeLinker` needs `SharedHomeFiles`, `ProcessClassifier` needs a non-empty `Process`, and `Doctor` is always there. Everything else is missing, and `agentio.Unsupported` says why.

<!-- snippet: examples/fromspec/main.go#build -->
```go
// build turns the Spec into an agentio.Agent bound to h and prints what it
// can do. A config-only agent has only the capabilities its Spec data
// enables; everything else declares why it is missing.
func build(w io.Writer, h agentio.Host) (agentio.Agent, error) {
	a, err := agentio.FromSpec(acmeSpec(), h)
	if err != nil {
		return nil, err
	}
	homes, err := a.Homes()
	if err != nil {
		return nil, err
	}
	for _, home := range homes {
		fmt.Fprintf(w, "home %s default=%t %s\n", home.Name(), home.IsDefault(), tidy(h, home.Dir()))
	}
	fmt.Fprintf(w, "supported: %v\n", agentio.Supported(a))
	reason, _ := agentio.Unsupported(a, agentio.CapUsage)
	fmt.Fprintf(w, "usage: %s\n", reason)

	// Capabilities follow the data: without LoginArgs there is no LoginCommander.
	noLogin := acmeSpec()
	noLogin.LoginArgs = nil
	b, err := agentio.FromSpec(noLogin, h)
	if err != nil {
		return nil, err
	}
	_, ok := agentio.LoginCommanderOf(b)
	reason, _ = agentio.Unsupported(b, agentio.CapLoginCommander)
	fmt.Fprintf(w, "without login args: login commander %t (%s)\n", ok, reason)
	return a, nil
}
```

## Step 3 — Launch it

`Launch(target, request)` fills the templates. The home rule applies as for every agent: the default home's command removes the home variable, another home's sets it, and nothing in `Target.ExtraEnv` can override that. A request field without a template is `ErrUnsupported`, never silently dropped. The login command, process classification and doctor checks also come straight from the Spec.

<!-- snippet: examples/fromspec/main.go#launch -->
```go
// launch builds commands from the templates. The default home's command
// removes ACME_HOME; another home's sets it. A request field the Spec has
// no template for is ErrUnsupported, never silently dropped.
func launch(w io.Writer, h agentio.Host, a agentio.Agent) error {
	homes, err := a.Homes()
	if err != nil {
		return err
	}
	def, work := homes[0], homes[1]
	requests := []struct {
		home agentio.Home
		req  agentio.LaunchRequest
	}{
		{def, agentio.LaunchRequest{}},
		{def, agentio.LaunchRequest{Model: exampleModel, Prompt: examplePrompt}},
		{work, agentio.LaunchRequest{ResumeID: exampleConvID}},
	}
	for _, r := range requests {
		cmd, err := a.Launch(r.home.Target(), r.req)
		if err != nil {
			return err
		}
		env, set := agentio.EnvLookup(cmd.Env, acmeHomeEnv)
		fmt.Fprintf(w, "%s: %s (%s set=%t %s)\n", r.home.Name(), cmd, acmeHomeEnv, set, tidy(h, env))
	}
	_, err = a.Launch(def.Target(), agentio.LaunchRequest{Headless: true})
	fmt.Fprintf(w, "headless: %v (unsupported=%t)\n", err, errors.Is(err, agentio.ErrUnsupported))
	_, err = a.Launch(def.Target(), agentio.LaunchRequest{Autonomy: agentio.AutonomyFull})
	fmt.Fprintf(w, "full autonomy: %v\n", err)
	return nil
}
```

<!-- snippet: examples/fromspec/main.go#extras -->
```go
// extras builds the login command, classifies two process rows and runs
// the L1 doctor checks; all of it comes from the Spec.
func extras(ctx context.Context, w io.Writer, h agentio.Host, a agentio.Agent) error {
	if lc, ok := agentio.LoginCommanderOf(a); ok {
		cmd, err := lc.LoginCommand(a.DefaultHome(), agentio.LoginBrowser)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "login: %s, logout: %s\n", cmd, lc.LogoutCommand(a.DefaultHome()))
		_, err = lc.LoginCommand(a.DefaultHome(), agentio.LoginDevice)
		fmt.Fprintf(w, "device login: %v\n", err)
	}
	rows := []agentio.Process{
		{PID: 10, Exe: "/opt/bin/acme", Argv: []string{"acme", "--resume", exampleConvID}},
		{PID: 11, Exe: "/opt/acme-desktop/acme", Argv: []string{"acme"}},
	}
	for _, p := range rows {
		info, ok := agentio.ClassifyProcess([]agentio.Agent{a}, p)
		fmt.Fprintf(w, "pid %d: match=%t kind=%s session=%t\n", p.PID, ok, info.Kind, info.Session)
	}
	if d, ok := agentio.DoctorOf(a); ok {
		for _, c := range d.Checks(ctx, a.DefaultHome()) {
			fmt.Fprintf(w, "check %-15s %-4s %s\n", c.ID, c.Status, tidy(h, c.Message))
		}
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./fromspec
```

Output:

```text
== validate ==
agentio: invalid spec: acme: resume args [--resume] lack {id}
agentio: invalid spec: acme: default home "/etc/acme" must be relative to the user home true
== build ==
home personal default=true ~/.acme
home work default=false ~/.acme-work
supported: [binary-version login-commander process-classifier home-linker doctor]
usage: config-only agent: a spec describes L1 only; this needs code
without login args: login commander false (spec has no login args)
== launch ==
personal: acme (ACME_HOME set=false )
personal: acme --model acme-large --message "fix the build" (ACME_HOME set=false )
work: acme --resume c-42 (ACME_HOME set=true ~/.acme-work)
headless: agentio: unsupported: acme: headless: not described by the agent spec (unsupported=true)
full autonomy: agentio: full autonomy needs Target.AllowDangerous
== login, process, doctor ==
login: acme login, logout: acme logout
device login: agentio: unsupported: acme: login flow device: not described by the agent spec
pid 10: match=true kind=cli session=true
pid 11: match=false kind= session=false
check binary-found    ok   /opt/bin/acme
check home            ok   ~/.acme
check api-billing-env warn ACME_API_KEY set: launches bill per token
```

## Pitfalls

- **A Spec describes the core only.** Usage, identity, conversations, turns, prompt history, live sessions, config, MCP, skills and run events need code; a config-defined agent never has them.
- **Autonomy, headless and token logins are refused.** Any non-default `Autonomy`, `Headless: true` and a `Credential` return `ErrUnsupported` (and `AutonomyFull` without `Target.AllowDangerous` returns `ErrDangerousNotAllowed` first). A Spec cannot describe a permission flag or an event format.
- **Only the browser login flow.** `LoginArgs` is the browser flow; every other `LoginFlow` is `ErrUnsupported`. Without `LogoutArgs`, `LogoutCommand` returns a zero `Command`.
- **Do not type-assert capabilities.** The agent's Go type implements every data-driven interface; only the accessors and `Supported` check the Spec data (`CapabilityGate`).
- **Empty `HomeMarkers` means any folder is a home.** With an `ExtraHomes` glob, that can list unrelated folders; give at least one marker file.
- **`ExtraArgs` and `Args` order.** Computed args go where `{args}` stands (or at the end): `Target.ExtraArgs` first, then model, add-dir, `LaunchRequest.Args`, then the prompt template.
- **IDs are unique per set.** `FromSpec` accepts the ID `claude`; `builtin.New` refuses it. Keep custom IDs distinct from the built-ins.

## Related

- [Built-in agents](builtin.md#step-3--add-config-defined-agents): mixing Specs with Claude and Codex.
- [Shared capabilities and accessors](accessors.md#step-3--ask-why-a-capability-is-missing): `Unsupported` and `CapabilityGate`.
- [Agent, Spec and Core](../core/agent-and-spec.md), [Homes](../core/homes.md), [Launch commands](../core/launch.md).
- [Doors](doors.md#step-1--run-the-vendor-binary-through-a-runner): the root Runner behind `BinaryVersion`.
- [Choosing guides](../choosing/README.md). Example: [`examples/fromspec`](../../examples/fromspec/). Feature index: [features](../reference/features.md).
