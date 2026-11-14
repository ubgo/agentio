# Targets and commands

agentio never starts a process for you when you launch an agent. It builds an `agentio.Command`: the binary, the argv, the full child environment and the working dir, as data. You decide how to run it (`syscall.Exec`, `exec.Cmd`, or print it for a dry run). A `Target` is the home plus the inputs from the caller's profile; a `LaunchRequest` is one invocation (prompt, resume id, model, autonomy). You want this page to start or resume an agent in the right home with the right environment.

## Before you start

- Modules: `github.com/ubgo/agentio` and the vendor module of the agent you launch (`claude` in the example).
- Nothing on disk: building a command is Pure. Only running it needs the binary.

## Step 1 — Build a Target from a home

`Home.Target()` gives a plain target. Fill the rest from the caller's profile: `ExtraArgs` go before the request's own args, `ExtraEnv` is applied onto the base env before the credential and home rules, `Version` is the installed vendor version (zero = unknown), `AllowDangerous` unlocks full autonomy, `Billing` is a hint.

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

`Agent.Launch(target, request)` is the same call for every agent. Each `LaunchRequest` field the agent cannot express returns `ErrUnsupported`; nothing is silently dropped.

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
| `Path` | the binary name (`claude`); resolve it with `Agent.LookPath` before exec |
| `Args` | argv without argv[0]; never holds a token |
| `Env` | the full child environment; the one place a token may live |
| `Dir` | the working dir (`LaunchRequest.Dir`) |
| `Warnings` | drift notes from building (unknown or untested version) |
| `String()` | the shell-quoted argv, env never included |
| `Redacted()` | a copy whose secret env values read `[redacted]` |
| `SecretEnvNames()` | env names whose values came from a `Token` |
| `MarshalJSON` | always the redacted form |

The default home is the important special case. Its home variable is removed from the child env, even when the caller's shell set it, and an unknown `Target.Version` still builds, with a warning:

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

`Autonomy` is the neutral permission level: `default` (no flag), `read-only`, `ask`, `accept-edits`, `full`. A level with no exact vendor mode is refused, never mapped to a more permissive one. `full` (no prompts, no sandbox) needs `Target.AllowDangerous`; without it every builder returns `ErrDangerousNotAllowed`, and `agentio.CheckAutonomy` applies the same rule in your own code.

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

An `EnvChange` is one edit to a child environment: `EnvSet(name, value)`, `EnvSetSecret(name, token)` or `EnvUnset(name)`. `ApplyEnv(base, changes...)` returns a new env (the input is never modified): each change first removes every entry of its name, duplicates included, then a set appends one. `HomeEnvChange(homeEnv, home)` is the home rule (unset for the default home, set to the folder for any other) and `ScrubEnv(names)` unsets a list. `NewCommand` builds a `Command` from those pieces and remembers which names hold secrets.

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

Launch builders apply changes in a fixed order: `Target.ExtraEnv` first, then (token login only) a scrub of the vendor's credential variables followed by the token itself, then the home rule last. So a profile can never override the home variable or bring back a scrubbed credential.

## Try it

```sh
cd examples && go run ./launch
```

The lines for this page (the rest belong to [Tokens and redacted values](secrets.md)):

```text
new: claude --verbose --model sonnet --add-dir /work/shared-lib -- "fix the failing tests"
new: dir=/work/project
new: env ANTHROPIC_API_KEY=[redacted]
new: env TEAM=platform
new: env CLAUDE_CONFIG_DIR=~/.claude-work
new: env -CLAUDE_CODE_OAUTH_TOKEN
new: warnings=0
resume: claude --verbose --resume f85efbdb-fc2f-1e8e-7748-9f915edc8e2d
default: claude
default: env ANTHROPIC_API_KEY=[redacted]
default: env -CLAUDE_CONFIG_DIR
default: env -CLAUDE_CODE_OAUTH_TOKEN
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

- **A home-login launch keeps the caller's API-key variables.** Only a token login scrubs `ANTHROPIC_API_KEY` and friends. If the base env carries one (as above), Claude Code bills per token instead of using the home's subscription. Remove it with `agentio.EnvUnset` in `Target.ExtraEnv`, and watch for the `api-billing-env` doctor check ([Doctor](../capabilities/doctor.md)).
- **`ExtraEnv` cannot set the home variable.** The home rule is applied last; to launch another home, use that home's Target.
- **A prompt goes after `--`.** Claude's builder separates the prompt from flags, so a prompt that starts with `-` is never read as a flag.
- **`Command.Env` holds real secrets.** Print `Redacted()`, `String()` or the JSON form; never `fmt.Println(cmd.Env)`.
- **`Redacted` also masks names that look secret.** Values from `ExtraEnv` whose names contain `TOKEN`, `KEY`, `SECRET`, `PASSWORD`, `PASSWD`, `CREDENTIAL` or `AUTH` are masked even though they did not come from a Token; `SecretEnvNames` lists only the Token ones.
- **Unknown version builds and warns.** A zero `Target.Version` is never a refusal; read `Command.Warnings`. A known version that provably lacks a flag returns an error matching `ErrFeatureUnavailable` ([Per-feature version table](../compat/features.md)).
- **`EnvChange` hides its secret.** `Value()` returns `""` for a secret change and its `String()` and JSON are redacted; only `ApplyEnv` reads the token.

## Related

- [Tokens and redacted values](secrets.md)
- [Homes](homes.md)
- [Claude launch and resume](../claude/launch.md)
- [Codex launch](../codex/launch.md)
- [Config-defined agents](../capabilities/fromspec.md)
- Example: [`examples/launch`](../../examples/launch/)
- [Feature index](../reference/features.md)
