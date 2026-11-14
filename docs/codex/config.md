# Codex config.toml

Typed access to Codex configuration: one `config.toml` file as a presence-tracked [`codex.Config`](ds:block?id=codex-config-4zvr4t68), the effective config Codex would apply in a folder (system, user, profile, trusted project layers, `-c` overrides) with the layer each key came from, the two profile systems, and type-safe builders for `-c path=value` overrides.

## Before you start

- Module `github.com/ubgo/agentio/codex` (it carries its own TOML parser; Claude-only callers never download it).
- A `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- For project layers: a folder with a project-root marker ([`.git`](ds:block?id=codex-projectroot-645w589u) by default) and a `.codex/config.toml`, trusted in the user config's `[projects."<dir>"]` table.

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/). It trusts its temp project folder in the copied `config.toml`, so the project layer applies.

## Step 1 — Read config.toml

[`ConfigFile`](ds:block?id=codex-readconfigpath-dfhqgu47) reads `<home>/config.toml` only (an absent file is an empty `Config`); [`ConfigFileWarnings`](ds:block?id=codex-agent-configfilewarnings-z5usj78f) adds the drift warnings. Every scalar is an `Opt[T]` ([`codex.Opt`](ds:block?id=codex-opt-d24avffs) is the root `agentio.Opt`): `.Set` tells an absent key from one set to its zero value. Keys the library does not model stay in `Unknown` (an [`UnknownTOML`](ds:block?id=codex-tomldecoder-keep-cd6urynz), dotted key → TOML source text); sections it keeps but does not type yet are [`TOMLRaw`](ds:block?id=codex-tomlraw-j7dab432). [`KeyCount`](ds:block?id=codex-decodeentriesinto-6fcyfvdp) reports how every leaf key was placed.

The config enums (`ApprovalPolicy`, `SandboxMode`, `ReasoningEffort`, `ReasoningSummary`, `Verbosity`, `WebSearchMode`, `TrustLevel`, `AuthMode`, `CredentialStore`, `HistoryPersistence`, …) are open string types: [a value newer than the library decodes as-is](ds:block?id=codex-enumopt-sp5njn2z), and `IsKnown()` tells you whether the library was written against it.

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

[`ParseConfig`](ds:block?id=codex-parseconfig-bbrucwhw) is the same decoder for bytes you already hold. A syntax error fails the file; a key whose type changed only degrades that key into `Unknown` with a [`type-changed`](ds:block?id=codex-tomldecoder-mismatch-z7r3y8m5) warning.

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

[`ConfigLayers(h, dir)`](ds:block?id=codex-agent-configlayers-9expgqan) lists the files that apply, highest precedence first (the order Claude and the neutral `ConfigReader` use), including disabled ones with the reason. [`Effective(h, EffectiveQuery)`](ds:block?id=codex-agent-layers-yeww4cy8) merges them the way Codex does, from lowest to highest: system (only with `IncludeSystem`), user `config.toml`, the legacy `[profiles.X]` the root `profile` selects (only without a v2 profile), the v2 profile file, trusted project layers from the project root down to `Dir` (keys Codex denies in project layers dropped), the `Overrides` you will pass with `-c`, and the legacy `managed_config.toml` (only with `IncludeSystem`). [A higher layer wins per key; tables merge.](ds:block?id=codex-agent-effective-zsyuwnnc) `EffectiveConfig.Layers` lists them highest first, like `ConfigLayers`. `Origins` maps each dotted key to the winning `LayerKind`. [`Config.TrustOf(dir)`](ds:block?id=codex-config-trustof-bhxj6vhr) is the trust level of the nearest configured ancestor; [`Config.FeatureOverrides()`](ds:block?id=codex-config-featureoverrides-wqauyv6a) flattens `[features]` to name → enabled.

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

Codex has two profile systems. Legacy profiles are `[profiles.<name>]` tables in `config.toml` (plus the root `profile = "…"` selector); v2 profiles (Codex 0.131.0 and later) are `<home>/<name>.config.toml` files selected with `codex -p NAME`. [`Profiles`](ds:block?id=codex-agent-profiles-vhcadz8n) lists both (legacy first), [`ProfileFiles`](ds:block?id=codex-agent-profilefiles-uczzc9je) only the v2 names, [`Config.LegacyProfileNames`](ds:block?id=codex-config-legacyprofilenames-hqw2wzbk) the legacy ones. [`Config.ProfileConflict(name)`](ds:block?id=codex-config-profileconflict-yrme7f58) returns `ErrProfileConflict` when the name is a legacy profile (a `[profiles.<name>]` table or the root `profile`), because Codex 0.162.1 refuses `-p` for such a name.

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

An `Override` is one `-c path=value`; its `Value` is TOML source text. Build it with `OverrideString`, `OverrideBool`, `OverrideInt` or [`OverrideStrings`](ds:block?id=codex-overridestrings-qyfnckfz) so quoting is the library's job. [`ParseOverride`](ds:block?id=codex-parseoverride-csjwphzj) reads user text exactly like Codex: the value is TOML when it parses as TOML, else a literal string (then quoted). Pass overrides in `LaunchOptions.Overrides` and in `EffectiveQuery.Overrides` to preview their effect.

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

- **Layers are listed highest first.** [`ConfigLayers`](ds:block?id=codex-agent-configlayers-9expgqan) and `EffectiveConfig.Layers` put the winning layer first (legacy managed, session, deepest project, ..., user, system), the same order as Claude; earlier releases listed Codex layers lowest first.
- **Check [`.Set`](ds:block?id=opt-nj39byj3), not the zero value.** An absent `sandbox_mode` is `Set=false` with `V=""`; Codex then uses its own default, which this library does not invent.
- **Untrusted project layers are skipped.** A project's `.codex/config.toml` applies only when the folder (or an ancestor) is [`trust_level = "trusted"`](ds:block?id=codex-agent-layers-yeww4cy8); otherwise `ConfigLayers` lists it with `Disabled = "folder is not trusted"` and `Effective` ignores it. Some keys (for example [`notify`](ds:block?id=codex-projectdeniedroots-n2zvt35j)) are denied in project layers even when trusted.
- **MDM and cloud layers are never read.** [`EffectiveConfig.NotEvaluated`](ds:block?id=codex-agent-effective-zsyuwnnc) lists them; the effective config can differ from what Codex sees on a managed machine.
- **System layers are opt-in for `Effective` and need a system root.** `/etc/codex/config.toml` and `managed_config.toml` are merged by `Effective` only with [`EffectiveQuery.IncludeSystem`](ds:block?id=codex-agent-layers-yeww4cy8) (`ConfigLayers` always lists them), and only on a Host with `GOOS` and `SystemRoot` set (`HostFromOS` sets them; a fixture host does not, so they are skipped). On Windows the system file is [`%ProgramData%\OpenAI\Codex\config.toml`](ds:block?id=codex-agent-systemconfigpaths-bztdnt4n) (under `SystemRoot`) and there is no system managed file.
- **Secrets are never loaded into the struct.** [MCP env values](ds:block?id=codex-mcpserver-f2ettbda), [`shell_environment_policy.set`](ds:block?id=codex-shellenvironmentpolicy-armg7sww) values, [`experimental_bearer_token`](ds:block?id=codex-modelprovider-d7mxgc59) and inline bearer tokens are kept as names or presence flags only; read one MCP env value on purpose with [`MCPServerEnvValue`](mcp.md#step-1--list-mcp-servers).
- **[`ErrProfileConflict`](ds:block?id=codex-config-profileconflict-yrme7f58) is real.** Codex 0.162.1 refuses `-p NAME` while a legacy `[profiles.NAME]` table or `profile = "NAME"` exists; the doctor warns ([`profile-conflict`](ds:block?id=codex-agent-profilecheck-eqn24h4u)) and [`LaunchOptions.ProfileCheck`](ds:block?id=codex-builder-profile-wmpsnv3m) refuses it before launch.
- **`ParseOverride` needs `=`**, else [`ErrBadOverride`](ds:block?id=codex-parseoverride-csjwphzj). A value that is not valid TOML is quoted as a string: `sandbox_mode=read-only` becomes `sandbox_mode="read-only"`.
- **Changing config is a vendor command.** The library never writes [`config.toml`](ds:block?id=codex-writer-tax23vch); use [`FeatureCommand`](ds:block?id=codex-agent-featurecommand-w47qatyu) or `MCPAddCommand` ([launch](launch.md#step-3--build-archive-delete-and-queue-commands), [MCP](mcp.md#step-2--build-add-and-remove-commands)).

## Related

- [Config layers (neutral)](../capabilities/config.md), [Values: Opt and Unknown](../core/values.md), [Warnings and errors](../core/errors.md)
- [Codex MCP servers](mcp.md), [Codex hooks and rules](hooks-rules.md), [Codex launch](launch.md)
- [Claude settings](../claude/settings.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
