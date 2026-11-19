# Targets and commands

agentio never starts a process for you when you launch an agent. It builds an [`agentio.Command`](ds:block?id=command-c4wdgewr): the binary, the argv, the full child environment and the working dir, as data. You decide how to run it (`syscall.Exec`, `exec.Cmd`, or print it for a dry run). A `Target` is the home plus the inputs from the caller's profile; a `LaunchRequest` is one invocation (prompt, resume id, model, autonomy). You want this page to start or resume an agent in the right home with the right environment.

## Before you start

- Modules: `github.com/ubgo/agentio` and the vendor module of the agent you launch (`claude` in the example).
- Nothing on disk: building a command is Pure. Only running it needs the binary.

## Step 1 — Build a Target from a home

`Home.Target()` gives a plain target. Fill the rest from the caller's profile: `ExtraArgs` go before the request's own args, [`ExtraEnv`](ds:block?id=core-envchanges-f8hrn3we) is applied after the scrub of inherited credential variables and before a token login and the home rule, `Version` is the installed vendor version (zero = unknown), [`AllowDangerous`](ds:block?id=checkautonomy-udeuy75m) unlocks full autonomy, `Billing` is a hint.

<!-- snippet: examples/launch/main.go#target -->
```go
// workTarget builds the Target for the work home: the home plus the inputs
// that come from the caller's profile rather than from one invocation.
// Pure.
func workTarget(a *claude.Agent) agentio.Target {
	t := a.HomeAt(workHomeRel).Target()
	t.ExtraArgs = []string{"--verbose"}
	t.ExtraEnv = []agentio.EnvChange{agentio.EnvSet(profileEnvName, "platform")}
	// The installed version, so the builder can check flags against it; a
	// zero Version means unknown: build and warn.
	t.Version = agentio.MustVersion(installedVer)
	return t
}
```

## Step 2 — Launch through the neutral Agent

[`Agent.Launch(target, request)`](ds:block?id=core-launch-v8h5ys7c) is the same call for every agent. Each `LaunchRequest` field the agent cannot express returns `ErrUnsupported`; nothing is silently dropped.

<!-- snippet: examples/launch/main.go#launch-neutral -->
```go
// launchNeutral builds a new launch and a resume through the neutral Agent,
// the same call for every agent. Pure: nothing is looked up or run.
func launchNeutral(p printer, a agentio.Agent, t agentio.Target) error {
	cmd, err := a.Launch(t, agentio.LaunchRequest{
		Dir: projectDir, Prompt: promptText, Model: modelName, AddDirs: []string{extraDir},
	})
	if err != nil {
		return fmt.Errorf("launch: %w", err)
	}
	p.line("new: " + cmd.String())
	p.line("new: dir=" + cmd.Dir)
	printEnv(p, "new", cmd)
	p.line(fmt.Sprintf("new: warnings=%d", len(cmd.Warnings)))

	resume, err := a.Launch(t, agentio.LaunchRequest{Dir: projectDir, ResumeID: resumeID})
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	p.line("resume: " + resume.String())
	return nil
}
```

The `Command` you get back:

| Field / method | What it holds |
|---|---|
| `Path` | [the binary name (`claude`)](ds:block?id=core-command-m93k4eug); resolve it with `Agent.LookPath` before exec |
| `Args` | argv without argv[0]; never holds a token |
| `Env` | the full child environment; the one place a token may live |
| `Dir` | the working dir (`LaunchRequest.Dir`) |
| `Warnings` | drift notes from building (unknown or untested version) |
| `String()` | [the POSIX shell-quoted argv for display, env never included; not PowerShell or cmd.exe quoting](ds:block?id=command-string-uxssqggr) |
| `Redacted()` | a copy whose secret env values read [`[redacted]`](ds:block?id=command-redacted-qbsw6442) |
| `ShellLine()` | the line a person types to run it: `cd <Dir> && ` when `Dir` is set, the home variable of a non-default home (`CLAUDE_CONFIG_DIR=<home>`; nothing for the default home), then `String()`. No other env entry is ever printed, so a token cannot leak through it |
| `SecretEnvNames()` | [env names whose values came from a `Token`](ds:block?id=command-secretenvnames-72t7wfqg) |
| `MarshalJSON` | [always the redacted form](ds:block?id=command-marshaljson-gy7a7qs2) |

The default home is the important special case. Its home variable is [removed from the child env](ds:block?id=homeenvchange-59nx5j7k), even when the caller's shell set it, and an unknown `Target.Version` [still builds, with a warning](ds:block?id=claude-versionwarnings-82bmgcbc):

<!-- snippet: examples/launch/main.go#launch-default -->
```go
// launchDefault launches the default home. The home variable is REMOVED
// from the child env, never set to ~/.claude: Claude Code keys its keychain
// entry on the variable, so setting it would make the home look logged
// out. The unknown version (zero) builds anyway and adds a warning. Pure.
func launchDefault(p printer, a *claude.Agent) error {
	cmd, err := a.Neutral().Launch(a.DefaultHome().Target(), agentio.LaunchRequest{})
	if err != nil {
		return err
	}
	p.line("default: " + cmd.String())
	printEnv(p, "default", cmd)
	for _, w := range cmd.Warnings {
		p.line("default: warning " + w.String())
	}
	return nil
}
```

## Step 3 — Check the autonomy level

`Autonomy` is the neutral permission level: `default` (no flag), `read-only`, `ask`, `accept-edits`, `full`. A level with no exact vendor mode is refused, [never mapped to a more permissive one](ds:block?id=claude-launchfromrequest-tr947xnr). `full` (no prompts, no sandbox) needs `Target.AllowDangerous`; without it every builder returns `ErrDangerousNotAllowed`, and [`agentio.CheckAutonomy`](ds:block?id=checkautonomy-udeuy75m) applies the same rule in your own code.

<!-- snippet: examples/launch/main.go#autonomy -->
```go
// autonomy shows the one policy every builder shares: AutonomyFull (skip
// all permission prompts) needs Target.AllowDangerous. Pure.
func autonomy(p printer, a agentio.Agent, t agentio.Target) error {
	full := agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}
	_, err := a.Launch(t, full)
	p.line(fmt.Sprintf("autonomy full: refused=%t", errors.Is(err, agentio.ErrDangerousNotAllowed)))

	t.AllowDangerous = true
	if err := agentio.CheckAutonomy(t, agentio.AutonomyFull); err != nil {
		return err
	}
	cmd, err := a.Launch(t, full)
	if err != nil {
		return err
	}
	p.line("autonomy full allowed: " + cmd.String())

	cmd, err = a.Launch(t, agentio.LaunchRequest{Autonomy: agentio.AutonomyReadOnly})
	if err != nil {
		return err
	}
	p.line("autonomy read-only: " + cmd.String())
	return nil
}
```

## Step 4 — Change the child environment

An `EnvChange` is one edit to a child environment: `EnvSet(name, value)`, `EnvSetSecret(name, token)` or `EnvUnset(name)`. [`ApplyEnv(base, changes...)`](ds:block?id=applyenv-9fc572ee) returns a new env (the input is never modified): each change first removes every entry of its name, duplicates included, then a set appends one. [`HomeEnvChange(homeEnv, home)`](ds:block?id=homeenvchange-59nx5j7k) is the home rule (unset for the default home, set to the folder for any other) and [`ScrubEnv(names)`](ds:block?id=scrubenv-4rdtd62j) unsets a list. [`NewCommand`](ds:block?id=newcommand-78p36jg6) builds a `Command` from those pieces and remembers which names hold secrets.

<!-- snippet: examples/launch/main.go#child-env -->
```go
// childEnv builds an environment by hand with the same pieces the launch
// builders use. ApplyEnv never edits its input; each change first removes
// every entry of its name, then a set appends one. Pure.
func childEnv(p printer, a *claude.Agent) error {
	base := []string{"PATH=/usr/bin", "DEBUG=1", "DEBUG=2", claude.APIKeyEnv + "=sk-old"}
	changes := []agentio.EnvChange{agentio.EnvSet("DEBUG", "0"), agentio.EnvUnset("PATH")}
	// The home rule: unset for the default home, set for any other.
	if hc, ok := agentio.HomeEnvChange(claude.ConfigDirEnv, a.HomeAt(workHomeRel).Neutral()); ok {
		changes = append(changes, hc)
	}
	if hc, ok := agentio.HomeEnvChange(claude.ConfigDirEnv, a.DefaultHome().Neutral()); ok {
		p.line("home rule for default: " + hc.String())
	}
	// A token login scrubs the vendor's other credential variables first.
	changes = append(changes, agentio.ScrubEnv([]string{claude.APIKeyEnv, claude.AuthTokenEnv})...)
	changes = append(changes, agentio.EnvSetSecret(claude.OAuthTokenEnv, agentio.NewToken("sk-ant-oat01-example")))
	for _, c := range changes {
		p.line(fmt.Sprintf("change: op=%s %s", c.Op(), c))
	}
	env := agentio.ApplyEnv(base, changes...)
	p.line(fmt.Sprintf("base unchanged: %t", len(base) == 4))

	cmd := agentio.NewCommand(claude.Binary, []string{"--print"}, base, "", changes...)
	p.line(fmt.Sprintf("secret names: %v", cmd.SecretEnvNames()))
	p.line(fmt.Sprintf("redacted env: %v", cmd.Redacted().Env))
	js, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	p.line("json: " + string(js))
	p.line(fmt.Sprintf("raw env holds the token: %t", strings.Contains(strings.Join(env, " "), "sk-ant-oat01-example")))
	return nil
}
```

[Launch builders apply changes in a fixed order](ds:block?id=core-envchanges-f8hrn3we): a scrub of the vendor's credential variables first, so an inherited key never overrides the chosen login (for the home's own login every `Spec.CredentialEnv` variable; for a token login only the ones that outrank that token, so a variable that is itself an agent's login mechanism survives); then `Target.ExtraEnv`, so a credential you set on purpose is kept for a home login; then (token login only) the scrub again followed by the token itself; then the home rule last. So a profile can never override the home variable, and a token login always wins over a profile credential.

## Try it

```sh
cd examples && go run ./launch
```

The lines for this page (the rest belong to [Tokens and redacted values](secrets.md)):

```text
new: claude --verbose --model sonnet --add-dir /work/shared-lib -- "fix the failing tests"
new: dir=/work/project
new: env TEAM=platform
new: env CLAUDE_CONFIG_DIR=~/.claude-work
new: env -CLAUDE_CODE_OAUTH_TOKEN
new: env -ANTHROPIC_API_KEY
new: warnings=0
resume: claude --verbose --resume f85efbdb-fc2f-1e8e-7748-9f915edc8e2d
default: claude
default: env -CLAUDE_CONFIG_DIR
default: env -CLAUDE_CODE_OAUTH_TOKEN
default: env -ANTHROPIC_API_KEY
default: env -TEAM
default: warning version-unknown: installed Claude Code version unknown; flags not checked
autonomy full: refused=true
autonomy full allowed: claude --verbose --dangerously-skip-permissions
autonomy read-only: claude --verbose --permission-mode plan
home rule for default: -CLAUDE_CONFIG_DIR
change: op=set DEBUG=0
change: op=unset -PATH
change: op=set CLAUDE_CONFIG_DIR=~/.claude-work
change: op=unset -ANTHROPIC_API_KEY
change: op=unset -ANTHROPIC_AUTH_TOKEN
change: op=set-secret CLAUDE_CODE_OAUTH_TOKEN=[redacted]
base unchanged: true
secret names: [CLAUDE_CODE_OAUTH_TOKEN]
redacted env: [DEBUG=0 CLAUDE_CONFIG_DIR=~/.claude-work CLAUDE_CODE_OAUTH_TOKEN=[redacted]]
json: {"path":"claude","args":["--print"],"env":["DEBUG=0","CLAUDE_CONFIG_DIR=~/.claude-work","CLAUDE_CODE_OAUTH_TOKEN=[redacted]"]}
raw env holds the token: true
```

`~` is the fixture user home; `-NAME` means the variable is absent from the child env.

## Pitfalls

- **`String()` is POSIX sh only.** It is for display and logs; never paste it into PowerShell or `cmd.exe`. To run a command on any OS, start `Path` with `Args`, `Env` and `Dir` through `exec.Cmd`: no shell, no quoting. The OS runner resolves a bare `Path` on the command's own `PATH` (with `PATHEXT` on Windows).
- **Env edits follow the Host's OS.** Agent builders use `Host.NewCommand`, so on a windows Host a change to `PATH` or `CODEX_HOME` also removes `Path=` or `codex_home=`. `ApplyEnv` and `NewCommand` are the case-sensitive forms; `ApplyEnvFor` and `NewCommandFor` take the OS.
- **Every launch removes inherited credential variables.** A base env carrying `ANTHROPIC_API_KEY` (as above) would make Claude Code bill per token instead of using the home's login, so `Launch` unsets [`Spec.CredentialEnv`](ds:block?id=claude-credentialenv-kqm45qxy) (Claude: `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`; Codex: `CODEX_API_KEY`) for a home login too. To launch with a key on purpose, pass it as a token login (`LaunchRequest.Credential`) or set it in `Target.ExtraEnv`. [Codex keeps `OPENAI_API_KEY`](ds:block?id=codex-spec-ut8me6cu): Codex auth never reads it, and a model provider may name it as its key.
- **`ExtraEnv` cannot set the home variable.** [The home rule is applied last](ds:block?id=core-envchanges-f8hrn3we); to launch another home, use that home's Target.
- **[A prompt goes after `--`.](ds:block?id=claude-launchoptions-build-78wzjpre)** Claude's builder separates the prompt from flags, so a prompt that starts with `-` is never read as a flag.
- **`Command.Env` holds real secrets.** Print `Redacted()`, `String()` or the JSON form; never `fmt.Println(cmd.Env)`.
- **`Redacted` judges a value by its source.** [A value from a Token or `EnvValue`, and a credential variable the build scrubbed and the caller set again (an `ExtraEnv` API key), are masked; a plain value the build set with `EnvSet` is shown whatever its name](ds:block?id=secretnamesof-7pd7kykk) (`CLAUDE_CODE_API_KEY_HELPER_TTL_MS=300000` reads as written), so put a secret in `ExtraEnv` with `EnvSetSecret`. Only a value inherited unchanged from the base env, whose source is unknown, is masked by its name: `TOKEN`, `KEY`, `SECRET`, [`PASSWORD`](ds:block?id=sensitivenameparts-55jafnj3), `PASSWD`, `CREDENTIAL` or `AUTH` (a user's `GITHUB_TOKEN`). `SecretEnvNames` lists only the Token ones.
- **[Unknown version builds and warns.](ds:block?id=featurespan-in-vh7rmz94)** A zero `Target.Version` is never a refusal; read `Command.Warnings`. A known version that provably lacks a flag returns an error matching [`ErrFeatureUnavailable`](ds:block?id=claude-checkfeature-am2yhjzp) ([Per-feature version table](../compat/features.md)).
- **`EnvChange` hides its secret.** [`Value()`](ds:block?id=envchange-value-43pazd3a) returns `""` for a secret change and its `String()` and JSON are redacted; [only `ApplyEnv` and `ApplyEnvFor` read the token](ds:block?id=applyenv-9fc572ee).

## Related

- [Tokens and redacted values](secrets.md)
- [Homes](homes.md)
- [Claude launch and resume](../claude/launch.md)
- [Codex launch](../codex/launch.md)
- [Config-defined agents](../capabilities/fromspec.md)
- Example: [`examples/launch`](../../examples/launch/)
- [Feature index](../reference/features.md)
