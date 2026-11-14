# Codex config.toml

Typed access to Codex configuration: one `config.toml` file as a presence-tracked `codex.Config`, the effective config Codex would apply in a folder (system, user, profile, trusted project layers, `-c` overrides) with the layer each key came from, the two profile systems, and type-safe builders for `-c path=value` overrides.

## Before you start

- Module `github.com/ubgo/agentio/codex` (it carries its own TOML parser; Claude-only callers never download it).
- A `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- For project layers: a folder with a project-root marker (`.git` by default) and a `.codex/config.toml`, trusted in the user config's `[projects."<dir>"]` table.

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/). It trusts its temp project folder in the copied `config.toml`, so the project layer applies.

## Step 1 — Read config.toml

`ConfigFile` reads `<home>/config.toml` only (an absent file is an empty `Config`); `ConfigFileWarnings` adds the drift warnings. Every scalar is an `Opt[T]` (`codex.Opt` is the root `agentio.Opt`): `.Set` tells an absent key from one set to its zero value. Keys the library does not model stay in `Unknown` (an `UnknownTOML`, dotted key → TOML source text); sections it keeps but does not type yet are `TOMLRaw`. `KeyCount` reports how every leaf key was placed.

The config enums (`ApprovalPolicy`, `SandboxMode`, `ReasoningEffort`, `ReasoningSummary`, `Verbosity`, `WebSearchMode`, `TrustLevel`, `AuthMode`, `CredentialStore`, `HistoryPersistence`, …) are open string types: a value newer than the library decodes as-is, and `IsKnown()` tells you whether the library was written against it.

<!-- snippet: examples/codexconfig/config.go#config-file -->
```go
// readConfigFile decodes <home>/config.toml alone (no layers). Every
// scalar is an Opt: Set tells an absent key from its zero value.
func readConfigFile(w io.Writer, m machine) error {
	a := codex.New(m.host)
	cfg, warns, err := a.ConfigFileWarnings(a.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "model=%s effort=%s (known=%v) warnings=%d\n",
		cfg.Model.V, cfg.ModelReasoningEffort.V, cfg.ModelReasoningEffort.V.IsKnown(), len(warns))
	fmt.Fprintf(w, "sandbox_mode set=%v approval_policy set=%v\n", cfg.SandboxMode.Set, cfg.ApprovalPolicy.Set)

	kc := cfg.KeyCount()
	fmt.Fprintf(w, "keys total=%d typed=%d raw=%d unknown=%d\n", kc.Total, kc.Typed, kc.Raw, kc.Unknown)

	// Unknown keeps every unmodelled key with its TOML source text.
	keys := make([]string, 0, len(cfg.Unknown))
	for k := range cfg.Unknown {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "unknown %s = %s\n", k, cfg.Unknown[k])
	}
	return nil
}
```

`ParseConfig` is the same decoder for bytes you already hold. A syntax error fails the file; a key whose type changed only degrades that key into `Unknown` with a `type-changed` warning.

<!-- snippet: examples/codexconfig/config.go#parse-config -->
```go
// parseConfigBytes decodes config.toml text you already hold. A key whose
// type changed degrades to Unknown with a warning; the rest still reads.
func parseConfigBytes(w io.Writer) error {
	src := []byte("approval_policy = \"on-request\"\nsandbox_mode = \"brand-new-mode\"\nmodel_context_window = \"wide\"\n")
	cfg, warns, err := codex.ParseConfig(src, "inline.toml")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "approval=%s known=%v\n", cfg.ApprovalPolicy.V, cfg.ApprovalPolicy.V.IsKnown())
	fmt.Fprintf(w, "sandbox=%s known=%v\n", cfg.SandboxMode.V, cfg.SandboxMode.V.IsKnown())
	for _, wn := range warns {
		fmt.Fprintf(w, "warning: %s\n", wn)
	}
	return nil
}
```

## Step 2 — Read the effective config

`ConfigLayers(h, dir)` lists the files that apply, highest precedence first (the order Claude and the neutral `ConfigReader` use), including disabled ones with the reason. `Effective(h, EffectiveQuery)` merges them the way Codex does, from lowest to highest: system (only with `IncludeSystem`), user `config.toml`, the legacy `[profiles.X]` the root `profile` selects (only without a v2 profile), the v2 profile file, trusted project layers from the project root down to `Dir` (keys Codex denies in project layers dropped), the `Overrides` you will pass with `-c`, and the legacy `managed_config.toml` (only with `IncludeSystem`). A higher layer wins per key; tables merge. `EffectiveConfig.Layers` lists them highest first, like `ConfigLayers`. `Origins` maps each dotted key to the winning `LayerKind`. `Config.TrustOf(dir)` is the trust level of the nearest configured ancestor; `Config.FeatureOverrides()` flattens `[features]` to name → enabled.

<!-- snippet: examples/codexconfig/config.go#effective -->
```go
// effectiveConfig merges the layers Codex applies for a folder and says
// which layer won each key. The project's .codex/config.toml applies only
// once the folder is trusted in [projects].
func effectiveConfig(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()

	layers, err := a.ConfigLayers(h, m.project)
	if err != nil {
		return err
	}
	for _, l := range layers {
		state := "applied"
		if l.Disabled != "" {
			state = "disabled: " + l.Disabled
		}
		fmt.Fprintf(w, "layer %-12s exists=%-5v %s\n", l.Kind, l.Exists, state)
	}

	eff, err := a.Effective(h, codex.EffectiveQuery{
		Dir:       m.project,
		Profile:   "work",
		Overrides: []codex.Override{codex.OverrideString("model_verbosity", "low")},
	})
	if err != nil {
		return err
	}
	for _, key := range []string{"model", "model_reasoning_effort", "model_verbosity"} {
		fmt.Fprintf(w, "effective %s from %s\n", key, eff.Origins[key])
	}
	fmt.Fprintf(w, "model=%s not evaluated=%v\n", eff.Config.Model.V, eff.NotEvaluated)

	cfg := eff.Config
	if lvl, ok := cfg.TrustOf("/work/proj/sub"); ok {
		fmt.Fprintf(w, "trust of /work/proj/sub: %s\n", lvl)
	}
	fmt.Fprintf(w, "feature overrides: %v\n", cfg.FeatureOverrides())
	return nil
}
```

## Step 3 — Read profiles

Codex has two profile systems. Legacy profiles are `[profiles.<name>]` tables in `config.toml` (plus the root `profile = "…"` selector); v2 profiles (Codex 0.131.0 and later) are `<home>/<name>.config.toml` files selected with `codex -p NAME`. `Profiles` lists both (legacy first), `ProfileFiles` only the v2 names, `Config.LegacyProfileNames` the legacy ones. `Config.ProfileConflict(name)` returns `ErrProfileConflict` for a name in both, which Codex refuses.

<!-- snippet: examples/codexconfig/config.go#profiles -->
```go
// readProfiles lists legacy [profiles.*] tables and v2 <name>.config.toml
// files, and checks a name before passing it to `codex -p`.
func readProfiles(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	ps, err := a.Profiles(h)
	if err != nil {
		return err
	}
	for _, p := range ps {
		fmt.Fprintf(w, "profile %s kind=%s table=%v file=%s\n", p.Name, p.Kind, p.HasTable, m.rel(p.Path))
	}
	files, err := a.ProfileFiles(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "v2 profile files: %v\n", files)

	cfg, err := a.ConfigFile(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "legacy names: %v\n", cfg.LegacyProfileNames())
	if err := cfg.ProfileConflict("ollama-launch"); errors.Is(err, codex.ErrProfileConflict) {
		fmt.Fprintf(w, "conflict: %v\n", err)
	}
	return nil
}
```

## Step 4 — Build config overrides

An `Override` is one `-c path=value`; its `Value` is TOML source text. Build it with `OverrideString`, `OverrideBool`, `OverrideInt` or `OverrideStrings` so quoting is the library's job. `ParseOverride` reads user text exactly like Codex: the value is TOML when it parses as TOML, else a literal string (then quoted). Pass overrides in `LaunchOptions.Overrides` and in `EffectiveQuery.Overrides` to preview their effect.

<!-- snippet: examples/codexconfig/config.go#overrides -->
```go
// buildOverrides makes `-c path=value` arguments. The typed constructors
// quote TOML for you; ParseOverride reads text the way Codex does.
func buildOverrides(w io.Writer) error {
	ovs := []codex.Override{
		codex.OverrideString("model", "gpt-5.5"),
		codex.OverrideBool("features.hooks", true),
		codex.OverrideInt("model_context_window", 200000),
		codex.OverrideStrings("notify", []string{"/bin/notify", "--quiet"}),
	}
	for _, text := range []string{"sandbox_mode=read-only", "model_auto_compact_token_limit=1000"} {
		o, err := codex.ParseOverride(text)
		if err != nil {
			return err
		}
		ovs = append(ovs, o)
	}
	for _, o := range ovs {
		fmt.Fprintf(w, "-c %s\n", o)
	}
	if _, err := codex.ParseOverride("no-equals-sign"); errors.Is(err, codex.ErrBadOverride) {
		fmt.Fprintln(w, "rejected: no '='")
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== config` section:

```text
== config
model=gpt-5.6-terra effort=medium (known=true) warnings=0
sandbox_mode set=false approval_policy set=false
keys total=56 typed=50 raw=3 unknown=3
unknown some_future_key.n = 3
unknown some_future_key.nested = true
approval=on-request known=true
sandbox=brand-new-mode known=false
warning: type-changed: inline.toml: model_context_window: expected integer, found string
layer project      exists=true  applied
layer user         exists=true  applied
effective model from project
effective model_reasoning_effort from user-profile
effective model_verbosity from session
model=project-model not evaluated=[mdm cloud]
trust of /work/proj/sub: trusted
feature overrides: map[hooks:true js_repl:false]
profile ollama-launch kind=legacy table=true file=~/.codex/config.toml
profile work kind=v2 table=false file=~/.codex/work.config.toml
v2 profile files: [work]
legacy names: [ollama-launch]
conflict: codex: profile name is also a legacy [profiles] entry: "ollama-launch" (rename or remove the legacy profile, or use a different v2 profile name)
-c model="gpt-5.5"
-c features.hooks=true
-c model_context_window=200000
-c notify=["/bin/notify", "--quiet"]
-c sandbox_mode="read-only"
-c model_auto_compact_token_limit=1000
rejected: no '='
```

## Pitfalls

- **Layers are listed highest first.** `ConfigLayers` and `EffectiveConfig.Layers` put the winning layer first (legacy managed, session, deepest project, ..., user, system), the same order as Claude; earlier releases listed Codex layers lowest first.
- **Check `.Set`, not the zero value.** An absent `sandbox_mode` is `Set=false` with `V=""`; Codex then uses its own default, which this library does not invent.
- **Untrusted project layers are skipped.** A project's `.codex/config.toml` applies only when the folder (or an ancestor) is `trust_level = "trusted"`; otherwise `ConfigLayers` lists it with `Disabled = "folder is not trusted"` and `Effective` ignores it. Some keys (for example `notify`) are denied in project layers even when trusted.
- **MDM and cloud layers are never read.** `EffectiveConfig.NotEvaluated` lists them; the effective config can differ from what Codex sees on a managed machine.
- **System layers are opt-in and need a system root.** `/etc/codex/config.toml` and `managed_config.toml` are read only with `EffectiveQuery.IncludeSystem`, and only on a Host with `GOOS` and `SystemRoot` set (`HostFromOS` sets them; a fixture host does not, so they are skipped). On Windows the system file is `%ProgramData%\OpenAI\Codex\config.toml` (under `SystemRoot`) and there is no system managed file.
- **Secrets are never loaded into the struct.** MCP env values, `shell_environment_policy.set` values, `experimental_bearer_token` and inline bearer tokens are kept as names or presence flags only; read one MCP env value on purpose with [`MCPServerEnvValue`](mcp.md#step-1--list-mcp-servers).
- **`ErrProfileConflict` is real.** Codex 0.162.1 refuses `-p NAME` while a legacy `[profiles.NAME]` table or `profile = "NAME"` exists; the doctor warns (`profile-conflict`) and `LaunchOptions.ProfileCheck` refuses it before launch.
- **`ParseOverride` needs `=`**, else `ErrBadOverride`. A value that is not valid TOML is quoted as a string: `sandbox_mode=read-only` becomes `sandbox_mode="read-only"`.
- **Changing config is a vendor command.** The library never writes `config.toml`; use `FeatureCommand` or `MCPAddCommand` ([launch](launch.md#step-3--build-archive-delete-and-queue-commands), [MCP](mcp.md#step-2--build-add-and-remove-commands)).

## Related

- [Config layers (neutral)](../capabilities/config.md), [Values: Opt and Unknown](../core/values.md), [Warnings and errors](../core/errors.md)
- [Codex MCP servers](mcp.md), [Codex hooks and rules](hooks-rules.md), [Codex launch](launch.md)
- [Claude settings](../claude/settings.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
