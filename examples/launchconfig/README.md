# launchconfig

Typed launch configuration for a launcher such as agx: the launch option registry (`claude.LaunchOptionKeys`, `DecodeLaunchOptions`, `ParseLaunchOption`), Claude settings that keep unknown keys (`ValidateSettings`) and go to a 0600 per-launch file (`LaunchSettings`), a live credential plan (`agentio.LiveCredentialOf`), Codex `-c` overrides checked against the config key table (`codex.CheckOverride`), and run-file cleanup (`Host.ProcessAlive`).

Everything is pure or writes into a temp dir (`<run dir>` in the output); no agent runs.

## Run

```sh
cd examples && go run ./launchconfig
```

## Output

```text
== claude options ==
61 option keys
  effort                     enum     flag=--effort since=- values=[low medium high xhigh max]
  mcp_configs                list     flag=--mcp-config since=0.2.75 values=[]
  append_system_prompt_file  string   flag=--append-system-prompt-file since=- values=[]
effort=max (the --option beat the profile)
refused: agentio: invalid launch option: unknown key "codex_profile"
== claude settings ==
ok=true unknown keys (kept, warned): [permissions.futureRule spinnerTipsEnabled]
type error: cleanupPeriodDays expected int
settings file: <run dir>/settings-20261114-120000-ab12.json mode=-rw-------
command: claude --effort max --mcp-config "~/mcp/github.json" --settings <run dir>/settings-20261114-120000-ab12.json --append-system-prompt-file "~/prompts/work.md"
== live credential ==
settings {"apiKeyHelper":"/usr/local/bin/agx credential"}
env [CLAUDE_CODE_API_KEY_HELPER_TTL_MS=10000]
scrub [CLAUDE_CODE_OAUTH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN]
refresh 10s
== codex options and overrides ==
codex profile=deep search=true
  -c model_reasoning_effort="high"                  typed
  -c model_reasoning_effort="turbo"                 unlisted-value one of none, minimal, low, medium, high, xhigh, max, ultra, persistent
  -c sandbox_workspace_write.network_access=true    typed
  -c model=true                                     type-error     expected string, found bool
  -c tui.theme="dark"                               raw
  -c brand_new_key="x"                              unknown
== run files ==
pid 4242: alive (err=<nil>)
pid 5151: ended (err=<nil>)
pid 6060: ended (err=<nil>)
```

## Test

```sh
cd examples && go test ./launchconfig/
```
