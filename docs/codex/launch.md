# Codex launch, resume and exec

Build the exact `codex` command line for an interactive start, a resume, a fork, a headless `codex exec` run or review, and the vendor's own management commands (archive, delete, queue, doctor, servers, feature switches). Builders are pure: they return an `agentio.Command` (path, argv, env with the home rule applied, warnings) and never run it. Flags the installed Codex provably lacks are refused before launch.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- An `agentio.Target`: `h.Target()` (version unknown) or `a.TargetFor(h)` (installed version from `package.json`). Set `Target.Version` when you know it; set `Target.AllowDangerous` before asking for `BypassAll`.
- Running the command is your job (`os/exec` with `Command.Path`, `Args`, `Env`, `Dir`). Running `exec` or `review` spends quota.

The snippets come from [`examples/codexlaunch`](../../examples/codexlaunch/), which puts a placeholder `codex` on `PATH` (never run) and prints each command as `codex <args>`.

## Step 1 — Start and resume

`New(t, LaunchOptions)` builds `codex [flags] [PROMPT]`; `Resume(t, id, o)` builds `codex resume ID`; `ResumeLast(t, o, all)` builds `codex resume --last [--all]`; `Fork(t, id, o)` builds `codex fork ID [PROMPT]`. `LaunchOptions` carries the global flags as typed fields: `Model`, `Profile`, `Sandbox` (`codex.SandboxMode`), `Approval` (`codex.ApprovalPolicy`), `AddDirs`, `Images`, `Overrides` (typed `-c`, see [config](config.md#step-4--build-config-overrides)), `Enable`/`Disable`, `Search`, `OSS`, `Worktree`, `NoDaemon`, `Remote`, `RemoteAuthTokenEnv` (a variable name, never a token), and more. `ProfileCheck` takes the home's `Config` and refuses a profile name that collides with a legacy profile.

<!-- snippet: examples/codexlaunch/main.go#start -->
```go
// startAndResume builds interactive commands. TargetFor reads the
// installed version from disk (here unknown: the placeholder binary has
// no package.json), so the builder warns instead of gating flags.
func startAndResume(w io.Writer, a *codex.Agent, other codex.Home) error {
	t, err := a.TargetFor(a.DefaultHome())
	if err != nil {
		return err
	}
	opts := codex.LaunchOptions{
		Prompt:   "fix the build",
		Model:    "gpt-5.5",
		Sandbox:  codex.ModeWorkspaceWrite,
		Approval: codex.ApprovalOnRequest,
		AddDirs:  []string{"/work/shared"},
		Overrides: []codex.Override{
			codex.OverrideString("model_reasoning_effort", "high"),
		},
	}
	c, err := a.New(t, opts)
	if err != nil {
		return err
	}
	show(w, "new", c)

	// Pin the version when you know it: no warning, and flags are gated.
	t.Version = agentio.MustVersion(codex.TestedMax)
	r, err := a.Resume(t, threadID, codex.LaunchOptions{})
	if err != nil {
		return err
	}
	show(w, "resume", r)
	last, err := a.ResumeLast(t, codex.LaunchOptions{}, true)
	if err != nil {
		return err
	}
	show(w, "resume last", last)
	f, err := a.Fork(t, threadID, codex.LaunchOptions{Prompt: "try another way"})
	if err != nil {
		return err
	}
	show(w, "fork", f)

	// A non-default home sets CODEX_HOME; the default home removes it.
	o, err := a.New(other.Target(), codex.LaunchOptions{})
	if err != nil {
		return err
	}
	show(w, "other home", o)

	if _, err := a.Resume(t, "", codex.LaunchOptions{}); errors.Is(err, codex.ErrNoSessionID) {
		fmt.Fprintln(w, "resume without an id: ErrNoSessionID")
	}
	if _, err := a.New(t, codex.LaunchOptions{BypassAll: true}); errors.Is(err, agentio.ErrDangerousNotAllowed) {
		fmt.Fprintln(w, "bypass without Target.AllowDangerous: ErrDangerousNotAllowed")
	}

	// ProfileCheck refuses a v2 profile name that a legacy [profiles]
	// table also uses, before Codex does.
	legacy, _, err := codex.ParseConfig([]byte("[profiles.work]\nmodel = \"o3\"\n"), "config.toml")
	if err != nil {
		return err
	}
	if _, err := a.New(t, codex.LaunchOptions{Profile: "work", ProfileCheck: &legacy}); errors.Is(err, codex.ErrProfileConflict) {
		fmt.Fprintln(w, "profile work: ErrProfileConflict")
	}
	return nil
}
```

## Step 2 — Run exec headless

`Exec(t, ExecOptions)` builds `codex exec [flags] [PROMPT|-]`, `ExecResume(t, id, o)` builds `codex exec resume ID`, `Review(t, ReviewTarget, o)` builds `codex exec review [--uncommitted|--base B|--commit SHA] [--title T]`. `ExecOptions` embeds `LaunchOptions` and adds `JSON`, `OutputLastMessage`, `OutputSchema`, `Color`, `SkipGitRepoCheck`, `Ephemeral`, `IgnoreUserConfig`, `IgnoreRules` and more. `PromptFromStdin` passes `-` so the prompt is read from stdin.

<!-- snippet: examples/codexlaunch/main.go#exec -->
```go
// execHeadless builds `codex exec` runs. Building is pure; RUNNING the
// command spends quota. In exec, Approval renders as -c approval_policy.
func execHeadless(w io.Writer, a *codex.Agent, _ codex.Home) error {
	t := a.DefaultHome().Target()
	t.Version = agentio.MustVersion(codex.TestedMax)
	e, err := a.Exec(t, codex.ExecOptions{
		LaunchOptions:     codex.LaunchOptions{PromptFromStdin: true, Approval: codex.ApprovalNever, Sandbox: codex.ModeReadOnly},
		JSON:              true,
		OutputLastMessage: "/tmp/last.txt",
		SkipGitRepoCheck:  true,
	})
	if err != nil {
		return err
	}
	show(w, "exec", e)
	er, err := a.ExecResume(t, threadID, codex.ExecOptions{LaunchOptions: codex.LaunchOptions{Prompt: "continue"}, JSON: true})
	if err != nil {
		return err
	}
	show(w, "exec resume", er)
	rv, err := a.Review(t, codex.ReviewTarget{Base: "main", Title: "Release review"}, codex.ExecOptions{JSON: true})
	if err != nil {
		return err
	}
	show(w, "review", rv)
	return nil
}
```

## Step 3 — Build archive, delete and queue commands

Codex keeps rollouts, its SQLite index and thread history consistent itself, so this library changes conversations and config only through the vendor's commands: `Archive`, `Unarchive`, `Delete(h, id, force)`, `Queue(h, thread, message)`, `DoctorCommand(h, json)`, `MCPServer(h)` (Codex as a stdio MCP server), `AppServer(h, listen)` (`""` = stdio) and `FeatureCommand(h, name, on)` (`codex features enable|disable`).

<!-- snippet: examples/codexlaunch/main.go#vendor -->
```go
// vendorCommands builds the vendor's own management commands: Codex keeps
// rollouts, its SQLite index and config consistent itself, so changes go
// through these instead of file writes.
func vendorCommands(w io.Writer, a *codex.Agent, _ codex.Home) error {
	h := a.DefaultHome()
	show(w, "archive", a.Archive(h, threadID))
	show(w, "unarchive", a.Unarchive(h, threadID))
	show(w, "delete", a.Delete(h, threadID, true))
	show(w, "queue", a.Queue(h, threadID, "run the tests next"))
	show(w, "doctor", a.DoctorCommand(h, true))
	show(w, "mcp-server", a.MCPServer(h))
	show(w, "app-server", a.AppServer(h, "ws://127.0.0.1:8390"))
	show(w, "feature", a.FeatureCommand(h, "hooks", true))
	return nil
}
```

## Step 4 — Handle unavailable features

Every version-dependent flag and subcommand is a `codex.Feature` with a verified `Since`/`Until` span (`codex.FeatureInfo`). When `Target.Version` is known and outside the span, the builder returns a `*codex.FeatureUnavailableError` naming the feature, the installed version and the bound; it matches `errors.Is(err, agentio.ErrFeatureUnavailable)`. When the version is unknown the builder emits the flag and adds a `version-unknown` warning instead.

<!-- snippet: examples/codexlaunch/main.go#unavailable -->
```go
// unavailableFeature asks for a flag the installed Codex provably lacks:
// the builder refuses with *FeatureUnavailableError, which also matches
// agentio.ErrFeatureUnavailable.
func unavailableFeature(w io.Writer, a *codex.Agent, _ codex.Home) error {
	t := a.DefaultHome().Target()
	t.Version = agentio.MustVersion(oldVersion)
	_, err := a.New(t, codex.LaunchOptions{NoDaemon: true})
	var fe *codex.FeatureUnavailableError
	if !errors.As(err, &fe) {
		return fmt.Errorf("expected a feature error, got %v", err)
	}
	fmt.Fprintf(w, "feature=%s installed=%s since=%s neutral match=%v\n",
		fe.Feature, fe.Installed, fe.Since, errors.Is(err, agentio.ErrFeatureUnavailable))
	span := codex.FeatureInfo(codex.FeatureFlagNoDaemon)
	fmt.Fprintf(w, "span known=%v since=%s\n", span.Known(), span.Since)
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexlaunch
```

The launch part of the output (the rest belongs to [exec decoding](exec.md)):

```text
new          codex -c model_reasoning_effort="high" -m gpt-5.5 -s workspace-write -a on-request --add-dir /work/shared "fix the build"  [warning version-unknown]
resume       codex resume 0199e1a0-0000-7000-8000-000000000001
resume last  codex resume --last --all
fork         codex fork 0199e1a0-0000-7000-8000-000000000001 "try another way"
other home   codex   [CODEX_HOME=work-codex]  [warning version-unknown]
resume without an id: ErrNoSessionID
bypass without Target.AllowDangerous: ErrDangerousNotAllowed
profile work: ErrProfileConflict
exec         codex exec -s read-only -c approval_policy="never" --json -o /tmp/last.txt --skip-git-repo-check -
exec resume  codex exec resume --json 0199e1a0-0000-7000-8000-000000000001 continue
review       codex exec review --base main --title "Release review" --json
archive      codex archive 0199e1a0-0000-7000-8000-000000000001
unarchive    codex unarchive 0199e1a0-0000-7000-8000-000000000001
delete       codex delete --force 0199e1a0-0000-7000-8000-000000000001
queue        codex queue --thread 0199e1a0-0000-7000-8000-000000000001 --message "run the tests next"
doctor       codex doctor --json
mcp-server   codex mcp-server
app-server   codex app-server --listen ws://127.0.0.1:8390
feature      codex features enable hooks
feature=--no-daemon installed=0.150.0 since=0.156.0 neutral match=true
span known=true since=0.156.0
```

## Pitfalls

- **Approval is a TUI flag.** `codex exec` has no `-a`; in `Exec` and `Review` an `Approval` renders as `-c approval_policy="…"`.
- **`BypassAll` needs `Target.AllowDangerous`**, else `agentio.ErrDangerousNotAllowed`. It maps to `--dangerously-bypass-approvals-and-sandbox`.
- **Resume and fork need an id**: an empty id is `codex.ErrNoSessionID`. A value Codex would reject is `codex.ErrInvalidOption`.
- **Unknown version means no gate.** `h.Target()` and a `TargetFor` without an npm-family install leave `Version` zero: flags are emitted and the command carries a `version-unknown` warning. Pin the version to get the gate.
- **`ProfileCheck` nil skips the conflict check** and adds a `version-unknown` warning when a `Profile` is set. Pass the home's `ConfigFile` result to refuse a v2 profile name that Codex 0.162.1 would refuse. Below Codex 0.131.0, `-p` selects a legacy profile, so no conflict check applies.
- **The default home removes `CODEX_HOME`**, a non-default home sets it; `Target.ExtraEnv` cannot override that. Secrets never go on argv: `RemoteAuthTokenEnv` and `BearerTokenEnvVar` take variable names.
- **Building is free, running is not.** `Exec`, `ExecResume` and `Review` spend quota when run; `DoctorCommand` and `PluginListCommand` may touch the network and auth.

## Related

- [Launch (neutral)](../core/launch.md), [Compat features](../compat/features.md), [Versions](../core/versions.md)
- [Codex exec event decoding](exec.md), [Codex config.toml](config.md), [Codex conversations](conversations.md)
- [Claude launch](../claude/launch.md#step-4--handle-unavailable-features)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
