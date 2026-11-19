# Typed launch configuration

A launcher that reads its own config file (agx's profiles, `--option key=value`, `--setting key=value`) has to turn loose keys into vendor flags, settings and overrides without guessing spellings or types. agentio describes each of those as data: a registry of launch options, Claude settings that keep every key, a table of Codex config paths, a plan for a credential the agent re-reads while it runs, and a liveness check for per-run files. Use this page when you build that layer; for launching from a neutral request see [Targets and commands](../core/launch.md).

## Before you start

- Modules `github.com/ubgo/agentio`, `.../claude` and `.../codex`.
- Nothing here runs an agent or reads the network. The only write is the per-launch settings file, into a dir you choose.

The snippets come from [`examples/launchconfig`](../../examples/launchconfig/).

## Step 1 — Validate typed options with the registry

[`claude.LaunchOptionKeys`](ds:block?id=claude-launchoptionkeys-r6h2j9xa) lists every `claude.LaunchOptions` field as an [`agentio.OptionKey`](ds:block?id=launch-optionkey-r6k4m2qa): a stable snake_case key (`effort`, `mcp_configs`, `append_system_prompt_file`), its kind and allowed values, the flag it emits, the flag's feature gate from `features.tsv`, and [`Path`](ds:block?id=launch-optionkey-r6k4m2qa) when the value names a file or folder on this machine. [The registry is derived by running the real argv builder](ds:block?id=launch-deriveoptionkeys-r6t7v3wd) once per field with a sample value, so it cannot drift from the flags that are built; a field the builder does not handle fails the derivation and its test.

[`DecodeLaunchOptions`](ds:block?id=claude-decodelaunchoptions-r6m4n8qe) strictly decodes a profile's typed block: an unknown key fails with the valid keys listed, and an enum value outside its set fails with the choices. [`ParseLaunchOption`](ds:block?id=claude-parselaunchoption-r6w3z7tb) applies one `key=value` on top (text is quoted, a list takes a JSON array or comma-separated text). [`LaunchOptionsSchema`](ds:block?id=claude-launchoptionsschema-r6c9f2ud) is the closed JSON Schema of the block, [built from the same keys](ds:block?id=launch-optionsschema-r6p8c5ke). Codex has the same four functions ([`codex.LaunchOptionKeys`](ds:block?id=codex-launchoptionkeys-r6x5k3pa); its profile key is `codex_profile`).

<!-- snippet: examples/launchconfig/main.go#options -->
```go
// claudeOptions validates a profile's typed block and one --option on top
// of it through the registry, then shows what three keys map to.
func claudeOptions(w io.Writer) (claude.LaunchOptions, error) {
	keys, err := claude.LaunchOptionKeys()
	if err != nil {
		return claude.LaunchOptions{}, err
	}
	fmt.Fprintf(w, "%d option keys\n", len(keys))
	for _, name := range []string{"effort", "mcp_configs", "append_system_prompt_file"} {
		k, err := agentio.FindOptionKey(keys, name)
		if err != nil {
			return claude.LaunchOptions{}, err
		}
		fmt.Fprintf(w, "  %-26s %-8s flag=%s since=%s values=%v\n", k.Key, k.Kind, k.Flag, orDash(k.Gate.Since), k.Values)
	}
	profile := []byte(`{"effort": "high", "append_system_prompt_file": "~/prompts/work.md", "mcp_configs": ["~/mcp/github.json"]}`)
	o, err := claude.DecodeLaunchOptions(profile)
	if err != nil {
		return o, err
	}
	if err := claude.ParseLaunchOption(&o, "effort", "max"); err != nil {
		return o, err
	}
	fmt.Fprintf(w, "effort=%s (the --option beat the profile)\n", o.Effort)
	_, err = claude.DecodeLaunchOptions([]byte(`{"effort": "high", "codex_profile": "deep"}`))
	fmt.Fprintln(w, "refused:", firstClause(err))
	return o, nil
}
```

## Step 2 — Type-check settings and hand them to Claude

A merged settings map (a profile's `claude.settings` plus `--setting` keys) goes to Claude as one JSON object. [`ValidateSettings`](ds:block?id=claude-validatesettings-r6e7r4ty) decodes it into `claude.Settings` to type-check the keys this library models: a wrong type is a `TypeErrors` entry (key and expected type, never the value), and every unmodelled key is listed in `UnknownKeys`, dotted when nested, so you can warn and still pass it on. [Every settings type writes its unknown keys back](ds:block?id=claude-settings-marshaljson-r6q2w8ne), so `ParseSettings` followed by `json.Marshal` loses nothing. [`SettingsSchema`](ds:block?id=claude-settingsschema-r6u3i9oz) describes the modelled keys and stays open to the rest.

[`LaunchSettings`](ds:block?id=claude-launchsettings-r6a5s2dg) prepares the value for `LaunchOptions.Settings`: with `SettingsViaFile` it writes `settings-<run id>.json` with mode 0600 into your run dir (created 0700) and refuses an existing file; with `SettingsViaInline` it returns the JSON itself, which then shows in the process list.

<!-- snippet: examples/launchconfig/main.go#settings -->
```go
// claudeSettings type-checks merged settings (unknown keys kept, listed
// for a warning) and writes them to this run's 0600 file for --settings.
func claudeSettings(w io.Writer, runDir string) (string, error) {
	merged := []byte(`{"model": "opus", "env": {"CLAUDE_CODE_API_KEY_HELPER_TTL_MS": "10000"}, "spinnerTipsEnabled": false, "permissions": {"allow": ["Bash(ls:*)"], "futureRule": true}}`)
	check, err := claude.ValidateSettings(merged)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "ok=%t unknown keys (kept, warned): %v\n", check.OK(), check.UnknownKeys)
	bad, err := claude.ValidateSettings([]byte(`{"cleanupPeriodDays": "thirty"}`))
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "type error: %s %s\n", bad.TypeErrors[0].Key, bad.TypeErrors[0].Detail)
	path, err := claude.LaunchSettings(merged, claude.SettingsViaFile, runDir, runID)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "settings file: <run dir>/%s mode=%v\n", strings.TrimPrefix(path, runDir+string(os.PathSeparator)), fi.Mode().Perm())
	return path, nil
}
```

## Step 3 — Plan a switchable login

[`agentio.LiveCredentialOf`](ds:block?id=livecredentialof-r6k2l7mz) returns the agent's live-credential planner when it can re-read its credential from a command you own. The [plan](ds:block?id=livecredentialplan-r6f8g3hj) holds the settings to merge, process env, the variables that would outrank the command (scrub them) and the refresh interval. [Claude's plan](ds:block?id=claude-livecredential-r6x9c4vb) sets `apiKeyHelper` and `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` (10 s by default); Claude runs the command itself, so the key never passes through your program. Codex has no such setting and says so through `agentio.Unsupported`.

A one-off helper login uses the `api-key-helper` token kind: [`claude.HelperSettings`](ds:block?id=claude-helpersettings-r6n3m8qw) turns it into `{"apiKeyHelper": "<command>"}`, and the neutral launch passes it inline.

<!-- snippet: examples/launchconfig/main.go#live -->
```go
// liveCredential plans a switchable session: Claude re-runs the helper
// command every Refresh, after the variables that outrank it are removed.
func liveCredential(w io.Writer, a agentio.Agent) error {
	lc, ok := agentio.LiveCredentialOf(a)
	if !ok {
		reason, _ := agentio.Unsupported(a, agentio.CapLiveCredential)
		fmt.Fprintln(w, "not switchable:", reason)
		return nil
	}
	plan, err := lc.LiveCredential("/usr/local/bin/agx credential", 0)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "settings %s\nenv %v\nscrub %v\nrefresh %s\n", plan.Settings, plan.Env, plan.Scrub, plan.Refresh)
	return nil
}
```

## Step 4 — Check Codex overrides

Codex settings travel as `-c path=value`. [`codex.ConfigKeys`](ds:block?id=codex-configkeys-r6e2r7ty) lists every typed `config.toml` path (`*` for a table key such as a profile or MCP server name) with its kind and enum values. [`codex.CheckOverride`](ds:block?id=codex-checkoverride-r6u5i3op) decodes one override through the same decoder `config.toml` goes through and reports `typed`, `unlisted-value` (Codex keeps unknown enum values, so warn), `raw` (a section kept raw, passed through), `unknown` (not modelled, passed through) or `type-error`.

<!-- snippet: examples/launchconfig/main.go#codex -->
```go
// codexConfig validates a Codex typed block and checks -c overrides
// against the config key table through the real config decoder.
func codexConfig(w io.Writer) error {
	o, err := codex.DecodeLaunchOptions([]byte(`{"codex_profile": "deep", "search": true}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "codex profile=%s search=%t\n", o.Profile, o.Search)
	for _, ov := range []codex.Override{
		codex.OverrideString("model_reasoning_effort", "high"),
		codex.OverrideString("model_reasoning_effort", "turbo"),
		codex.OverrideBool("sandbox_workspace_write.network_access", true),
		codex.OverrideBool("model", true),
		codex.OverrideString("tui.theme", "dark"),
		codex.OverrideString("brand_new_key", "x"),
	} {
		c, err := codex.CheckOverride(ov)
		if err != nil {
			return err
		}
		fmt.Fprintln(w, strings.TrimRight(fmt.Sprintf("  -c %-46s %-14s %s", ov, c.Status, c.Detail), " "))
	}
	return nil
}
```

## Step 5 — Clean up run files only after the agent ended

A launcher that writes per-run files (the settings file above, a run record) deletes them once the agent has ended. [`Host.ProcessAlive`](ds:block?id=host-processalive-r6a8s4df) takes the recorded pid, start time and an env entry you set at launch (`AGX_RUN_ID=<id>`): a gone pid, another start time, or a start env without the entry is `ended` (the pid was reused); a matching start time or env is `alive`; anything it cannot verify is `unknown` with the reason, so keep those files. `HostFromOS` reads a process's start env [on Linux and macOS](ds:block?id=processenvreader-r6g2h9jk); on Windows it reports unsupported and the start time decides. To re-check many recorded processes at once, `Host.ProcessesAlive(ctx, []ProcessRef{{PID, Start}})` reads the table once and answers per pid by start time alone (`ProcessAlive` uses it).

<!-- snippet: examples/launchconfig/main.go#alive -->
```go
// runFiles decides which run folders agx clean may delete: the recorded
// pid, start time and AGX_RUN_ID must still match a live process.
func runFiles(w io.Writer) error {
	started := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	h := agentio.Host{Processes: fixtureProcesses{
		{agentio.Process{PID: 4242, Start: started}, []string{"AGX_RUN_ID=" + runID}},
		{agentio.Process{PID: 5151, Start: started}, []string{"AGX_RUN_ID=someone-else"}},
	}}
	for _, pid := range []int{4242, 5151, 6060} {
		state, err := h.ProcessAlive(context.Background(), pid, started, "AGX_RUN_ID", runID)
		fmt.Fprintf(w, "pid %d: %s (err=%v)\n", pid, state, err)
	}
	return nil
}
```

## Step 6 — Launch with options and settings through the neutral Agent

Steps 1 to 4 use the vendor packages. A launcher that must stay agent-neutral asks [`agentio.LaunchConfigOf`](ds:block?id=launchconfigof-r7a2k4mq) instead and hands a [`LaunchConfig`](ds:block?id=launchconfig-r7b3m5nq) to [`LaunchConfigured`](ds:block?id=launchconfigurer-r7c4p6rs) next to the usual `LaunchRequest`: `Options` is the typed block as JSON, `Settings` the merged settings object, `Delivery` file (the default, into `RunDir` as `settings-<RunID>.json`) or inline. An empty `LaunchConfig` builds exactly what `Launch(t, r)` builds.

- **Options overlay the request.** [Only the keys given are set](ds:block?id=overlayoptions-r7d5q8tv) on top of what the request maps to; a key whose field the request already sets is refused naming both in plain words (`option "model" conflicts with the request's model`). Raw args are the exception: [an option list that lands where the request's raw args go](ds:block?id=overlayoptions-r7d5q8tv) (Claude's `extra_args`) is kept together with them, the option's list first and then the request's `Args`, so the raw args come later and win wherever the CLI lets a later flag override an earlier one. Every [`OptionKey`](ds:block?id=launch-optionkey-r6k4m2qa) carries the neutral `Concern` it overlaps (model, prompt, add-dir, autonomy, headless, resume, settings), so a launcher can apply one winner per concern before it builds. Every emitted flag is version-gated against `Target.Version`, as in `Launch`.
- **Settings are checked, then delivered.** `CheckSettings` returns a [`SettingsCheck`](ds:block?id=settingscheck-r7e6s9wx): type errors (key and expected type, never the value), unknown keys with a "did you mean", and the given keys that overlap a concern (Claude `model`, `permissions.defaultMode`, `apiKeyHelper`, `env.<credential variable>`; Codex `model`, `sandbox_mode`, `approval_policy`). A launch with a type error is refused; unknown keys pass and come back as `unknown-key` warnings on the `Command`.
- **Claude** [delivers settings through `LaunchSettings`](ds:block?id=claude-launchconfigured-r7f7t2yz) as `--settings <file|json>` after every other check, so a refused launch writes no file. An `api-key-helper` login is merged into the same object (a settings `apiKeyHelper` competing with it is refused) and follows `Delivery`, unlike the neutral `Launch`, which passes it inline. `RawSettingsArg` reports a raw `--settings` in the request's args: Claude would then see two, so skip your own. `LaunchConfig.Settings` is the one settings channel here: [the rich builder's `settings` option is not one of the neutral option keys](ds:block?id=claude-configoptionkeys-w3m8ht5q), so a launcher's own settings key never shows next to a second one (an options object naming `settings` is an unknown key).
- **Codex** [turns each settings leaf into `-c path=value`](ds:block?id=codex-settingsoverrides-r7g8u3za): objects recurse, strings, bools, numbers and string arrays are quoted by the `Override` constructors, anything else is refused, and [each override is checked](ds:block?id=codex-launchconfigured-r7h9v4bc) with `CheckOverride`. Overrides always travel on argv, so `Delivery` does not apply. A `codex_profile` option is [checked against the home's `config.toml`](ds:block?id=codex-agent-launchprofilecheck-h6t2nb4x) (the one disk read of this path): a legacy profile of that name is refused with `ErrProfileConflict` before Codex refuses it, any other name builds with no warning.
- **Gemini CLI, pi and config-defined agents** have no launch configurer; `agentio.Unsupported(a, agentio.CapLaunchConfig)` gives the reason. Pass their flags as raw args.

```go
lc, ok := agentio.LaunchConfigOf(a)
if !ok {
	reason, _ := agentio.Unsupported(a, agentio.CapLaunchConfig)
	return fmt.Errorf("no launch config: %s", reason)
}
cmd, err := lc.LaunchConfigured(target, agentio.LaunchRequest{Model: "opus"}, agentio.LaunchConfig{
	Options:  agentio.Raw(`{"effort": "high"}`),
	Settings: agentio.Raw(`{"cleanupPeriodDays": 30}`),
	RunDir:   runDir,
	RunID:    runID,
})
// claude --model opus --effort high --settings <run dir>/settings-<run id>.json
```

## Try it

```sh
cd examples && go run ./launchconfig
```

The output is in the example's [README](../../examples/launchconfig/README.md).

## Pitfalls

- **Expand `~` only in path options.** [`OptionKey.Path`](ds:block?id=launch-optionkey-r6k4m2qa) comes from the struct tag `agentio:"path"` on the vendor field (a list marks every element); [the derivation refuses the tag on anything but text or a text list](ds:block?id=launch-deriveoptionkeys-r6t7v3wd). Claude: `add_dirs`, `mcp_configs` (files or JSON), `settings` (file or JSON), `system_prompt_file`, `append_system_prompt_file`, `plugin_dirs`, `debug_file`. Codex: `cd`, `add_dirs`, `images`. Claude's `files` takes `file_id:relative_path` (a remote resource) and `resume` an id or a name, so neither is a path. The same keys' `Flag` tells you which raw args take a path. Gemini CLI and pi have no option registry.

- **Unknown is not wrong.** Claude and Codex know more keys than this library models. Unknown settings keys and unknown Codex overrides are passed through; show them as warnings, never drop them.
- **Inline settings are visible.** `SettingsViaInline` puts every key on argv, including an `apiKeyHelper` command and `env` values; use the file unless you have a reason not to.
- **A run id names a file.** `LaunchSettings` accepts `[A-Za-z0-9][A-Za-z0-9._-]*` only and never reuses a file, so two launches cannot share settings.
- **Record the start time from `Host.Processes`.** `ProcessAlive` allows two seconds of difference (macOS reports seconds); your own clock read before the exec is close enough, the process table's is exact.
- **A launch config file is the caller's to clean up.** `LaunchConfigured` writes the settings file only when settings are delivered as a file; remove it with the run folder once the agent has ended (Step 5).
- **The helper refresh is a floor, not a promise.** Claude also re-runs the helper on a 401; very short intervals run your command often.
- **The command must never print an empty value.** Claude Code treats an empty output or a non-zero exit as fatal: it tries three times, then exits 1, and never falls back to the home's own login. When your command cannot resolve the credential, write one line to stderr and exit non-zero so the reason shows up. A timed refresh is stale-while-revalidate, so the first request after a switch may still use the old key; a 401 makes Claude re-run the command before each retry.

## Related

- [Claude launch and resume](../claude/launch.md), [Claude settings](../claude/settings.md)
- [Codex launch](../codex/launch.md), [Codex config.toml](../codex/config.md)
- [Logins and tokens](logins.md), [Host](../core/host.md)
- Example: [`examples/launchconfig`](../../examples/launchconfig/)
