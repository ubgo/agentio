# Claude launch and resume

[`a.Launch(target, opts)`](ds:block?id=claude-agent-launch-b9s8bctp) turns typed Claude Code flags into an `agentio.Command` (binary, argv, full child environment, warnings) that you run yourself: with `exec.Cmd`, `syscall.Exec`, a terminal multiplexer, or by printing it for a dry run. It applies the home rule, checks every flag against the installed version, and rejects flag combinations Claude Code refuses. Use it to start Claude Code in a chosen home, resume or fork a conversation, or start a headless run whose output you decode with [Claude stream decoding](stream.md).

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A target: `h.Target()` for a home, with `Version` filled by [`a.TargetFor(h)`](ds:block?id=claude-agent-targetfor-c8cb3m3m) (Step 3) so flags can be checked.
- Nothing is run by building. Resolve the binary with [`a.LookPath()`](ds:block?id=core-lookpath-ke3ntet7) (a `PATH` walk, never a shell) when you execute the command.

The snippets come from [`examples/claudelaunch`](../../examples/claudelaunch/). It lays out a temp dir like a native install (`~/.local/share/claude/versions/2.1.325`, an empty file, and a `claude` symlink on `PATH`) so the version is read from disk; homes come from the checked-in 2.1.325 fixture.

## Step 1 — Build a launch command

[`claude.LaunchOptions`](ds:block?id=claude-launchoptions-5jyjfngt) models Claude Code's flags; zero values emit nothing. Print mode (`Print`, `OutputFormat`, `InputFormat`, `IncludePartialMessages`, `Verbose`, `JSONSchema`, `MaxBudgetUSD`, `NoSessionPersistence`), model and agents (`Model`, `FallbackModels`, `Effort`, `Agent`, `Agents`), permissions (`PermissionMode`, `AllowedTools`, `DisallowedTools`, `Tools`, `DangerouslySkipPermissions`), context (`AddDirs`, `MCPConfigs`, `Settings`, `SettingSources`, system prompt flags, plugin dirs), session (`Resume`, `Continue`, `ForkSession`, `SessionID`, `Name`, `FromPR`, `Worktree`, `Background`) and more; `ExtraArgs` are appended verbatim for flags not modelled. [The prompt goes last after `--`](ds:block?id=claude-launchoptions-build-78wzjpre), so a prompt starting with `-` is never read as a flag.

[`opts.Args()`](ds:block?id=claude-launchoptions-args-t9vscnsz) renders just the flags (no version check). A combination Claude Code rejects returns [`claude.ErrInvalidLaunch`](ds:block?id=claude-launchoptions-validate-tgdhh8yc) before anything is built: `--output-format` or `--input-format` without `-p`, `--include-partial-messages` without stream-json, `--bg` with `-p`, `--resume` with `--continue`, `--chrome` with `--no-chrome`. [`a.PurgeCommand(dir, dryRun)`](ds:block?id=claude-agent-purgecommand-379a3tjz) builds `claude purge <dir> [--dry-run]`.

For the default home the command's env has [`CLAUDE_CONFIG_DIR` removed](ds:block?id=homeenvchange-59nx5j7k) (the variable must be absent, not pointing at `~/.claude`); for any other home it is set to the home dir [exactly once](ds:block?id=applyenv-9fc572ee).

<!-- snippet: examples/claudelaunch/main.go#launch -->
```go
// buildLaunch turns LaunchOptions into a Command (data, never run). The
// prompt goes last after "--"; a combination Claude Code rejects is
// ErrInvalidLaunch before anything is built.
func buildLaunch(w io.Writer, a *claude.Agent, t agentio.Target) error {
	cmd, err := a.Launch(t, claude.LaunchOptions{
		Model:          "opus",
		PermissionMode: claude.ModePlan,
		AddDirs:        []string{"/work/shared"},
		Prompt:         "-explain the build",
	})
	if err != nil {
		return err
	}
	show(w, "launch", cmd)
	args, err := claude.LaunchOptions{Print: true, OutputFormat: claude.OutputStreamJSON, Verbose: true, IncludePartialMessages: true}.Args()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "headless args: %s\n", strings.Join(args, " "))
	if _, err := a.Launch(t, claude.LaunchOptions{OutputFormat: claude.OutputJSON}); errors.Is(err, claude.ErrInvalidLaunch) {
		fmt.Fprintln(w, "invalid:", err)
	}
	show(w, "purge", a.PurgeCommand("/work/old-project", true))
	return nil
}
```

## Step 2 — Resume a conversation

Three options pick up earlier work. `Resume` takes a conversation id, a session name or a transcript path (`claude --resume <id>`); `Continue` resumes the most recent conversation in the working folder (`--continue`); `ForkSession` with `Resume` starts a new conversation from the old one's history, optionally under a fixed `SessionID`. Resume in the home that holds the conversation: a transcript lives under one home's `projects/` dir, so the target's home must match (see [Claude conversations](conversations.md#step-4--plan-a-move) to move it). [The neutral `LaunchRequest.ResumeID` maps onto `Resume`](ds:block?id=claude-launchfromrequest-tr947xnr).

<!-- snippet: examples/claudelaunch/main.go#resume -->
```go
// resume builds the three ways back into a conversation: by id, the most
// recent one in the folder, and a fork under a new id. Another home gets
// CLAUDE_CONFIG_DIR in the child env; the default home never does.
func resume(w io.Writer, a *claude.Agent, t agentio.Target, work claude.Home) error {
	byID, err := a.Launch(t, claude.LaunchOptions{Resume: conversationID})
	if err != nil {
		return err
	}
	show(w, "resume", byID)
	last, err := a.Launch(t, claude.LaunchOptions{Continue: true})
	if err != nil {
		return err
	}
	show(w, "continue", last)
	wt := work.Target()
	wt.Version = t.Version
	fork, err := a.Launch(wt, claude.LaunchOptions{Resume: conversationID, ForkSession: true, SessionID: "22222222-0000-4000-8000-000000000002"})
	if err != nil {
		return err
	}
	show(w, "fork in work home", fork)
	if _, err := a.Launch(t, claude.LaunchOptions{Resume: conversationID, Continue: true}); errors.Is(err, claude.ErrInvalidLaunch) {
		fmt.Fprintln(w, "invalid:", err)
	}
	return nil
}
```

## Step 3 — Fill the Target version

[`a.TargetFor(h)`](ds:block?id=claude-agent-targetfor-c8cb3m3m) returns `h.Target()` with `Version` set from the install on disk: the versioned path of a native install or [the `package.json` of an npm install](ds:block?id=claude-methodandversion-8h4tytjj). It never runs `claude --version` (for that, see [Claude Runner](runner.md#step-1--read-the-binary-version)). When `claude` is not on `PATH` the version stays zero and the error is nil; other read errors are returned with the target as far as it was filled.

<!-- snippet: examples/claudelaunch/main.go#target -->
```go
// targetFor fills Target.Version from the install on disk (the versioned
// path of a native install), never by running claude --version.
func targetFor(w io.Writer, a *claude.Agent, h claude.Home) (agentio.Target, error) {
	t, err := a.TargetFor(h)
	if err != nil {
		return t, err
	}
	fmt.Fprintf(w, "target: home %s, version %s\n", t.Home.Name(), t.Version)
	return t, nil
}
```

## Step 4 — Handle unavailable features

Every flag a launch emits is checked against `Target.Version` using the per-feature table ([`claude.Features`](ds:block?id=claude-checkfeature-am2yhjzp), from the vendor changelog):

- the version provably lacks the flag: `Launch` returns a [`*claude.FeatureUnavailableError`](ds:block?id=claude-featureunavailableerror-is-anzr93zz) (`Feature`, `Installed`, `Since`, `Until`) that matches `agentio.ErrFeatureUnavailable` with `errors.Is`;
- the version is zero (unknown): the command is built with an [`agentio.WarnVersionUnknown`](ds:block?id=claude-versionwarnings-82bmgcbc) warning, flags unchecked;
- the version is outside the tested range ([`claude.TestedMin`](ds:block?id=claude-compat-ngsh97c9)..`claude.TestedMax`): the command is built with an [`agentio.WarnUntestedVersion`](ds:block?id=compat-warning-yeqfn6xw) warning.

Show warnings to the user (or log them) rather than dropping them: they are how drift between the library and a newer Claude Code becomes visible. See [Per-feature version table](../compat/features.md#step-3--handle-featureunavailableerror) for the table itself.

<!-- snippet: examples/claudelaunch/main.go#unavailable -->
```go
// unavailable shows the version check every flag goes through: a version
// that provably lacks a flag is a *FeatureUnavailableError; an unknown
// version or one outside the tested range builds with a warning.
func unavailable(w io.Writer, a *claude.Agent, h claude.Home) error {
	worktree := ""
	opts := claude.LaunchOptions{Worktree: &worktree}
	old := h.Target()
	old.Version = agentio.MustVersion(oldVersion)
	_, err := a.Launch(old, opts)
	var fe *claude.FeatureUnavailableError
	if errors.As(err, &fe) {
		fmt.Fprintf(w, "unavailable: %v (feature %s, since %s, matches ErrFeatureUnavailable: %t)\n",
			err, fe.Feature, fe.Since, errors.Is(err, agentio.ErrFeatureUnavailable))
	}
	unknown, err := a.Launch(h.Target(), opts)
	if err != nil {
		return err
	}
	show(w, "unknown version", unknown)
	future := h.Target()
	future.Version = agentio.MustVersion(futureVersion)
	untested, err := a.Launch(future, opts)
	if err != nil {
		return err
	}
	show(w, "untested version", untested)
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudelaunch
```

The lines for this page (the rest is on [Claude Runner](runner.md) and [Claude doctor](doctor.md)):

```text
target: home personal, version 2.1.325
launch: claude --model opus --permission-mode plan --add-dir /work/shared -- "-explain the build"
  CLAUDE_CONFIG_DIR set: false, warnings: 0
headless args: -p --output-format stream-json --include-partial-messages --verbose
invalid: claude: invalid launch options: --output-format needs -p
purge: claude purge /work/old-project --dry-run
  CLAUDE_CONFIG_DIR set: false, warnings: 0
resume: claude --resume 3f474440-903e-5410-8bfb-d537b89f9547
  CLAUDE_CONFIG_DIR set: false, warnings: 0
continue: claude --continue
  CLAUDE_CONFIG_DIR set: false, warnings: 0
fork in work home: claude --resume 3f474440-903e-5410-8bfb-d537b89f9547 --fork-session --session-id 22222222-0000-4000-8000-000000000002
  CLAUDE_CONFIG_DIR set: true, warnings: 0
invalid: claude: invalid launch options: --resume and --continue are exclusive
unavailable: claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40) (feature --worktree, since 2.1.49, matches ErrFeatureUnavailable: true)
unknown version: claude --worktree
  CLAUDE_CONFIG_DIR set: false, warnings: 1
  warning version-unknown: installed Claude Code version unknown; flags not checked
untested version: claude --worktree
  CLAUDE_CONFIG_DIR set: false, warnings: 1
  warning untested-version: installed 2.1.400 is outside tested 2.1.323..2.1.330
```

## Pitfalls

- **`Command.Path` is the bare name `claude`.** [Building is Pure and never walks `PATH`](ds:block?id=claude-spec-yq7hsrb6); resolve it with `a.LookPath()` before `exec`. Never resolve it through a shell: `claude` is often also a shell function or alias.
- **Print `Command.Env` only through [`Redacted()`](ds:block?id=command-redacted-qbsw6442).** The env is the full child environment and is where a token login lives; `String()` never includes it and [JSON shows it only redacted](ds:block?id=command-marshaljson-gy7a7qs2).
- **Default home means the variable is absent.** Setting `CLAUDE_CONFIG_DIR=~/.claude` is not the same: Claude Code then uses a hashed keychain name and looks logged out. [`Launch` handles this](ds:block?id=homeenvchange-59nx5j7k); hand-built commands must too.
- **Inherited credential variables are removed.** `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` and `CLAUDE_CODE_OAUTH_TOKEN` in the base env would silently override the home's login, so [every launch (and `LoginCommand`) unsets them](ds:block?id=core-envchanges-f8hrn3we). Use a token login or `Target.ExtraEnv` to launch with a key on purpose.
- **[A zero version skips checks, it does not fail.](ds:block?id=featurespan-in-vh7rmz94)** If you never call `TargetFor`, every launch builds with a `version-unknown` warning and no flag is checked.
- **Since dates come only from explicit changelog lines.** Many flags have no known `since`; [they are treated as always available](ds:block?id=featurespan-in-vh7rmz94), so an old Claude Code may still reject a flag the library allowed.
- **[`ExtraArgs` are not checked.](ds:block?id=claude-launchoptions-build-78wzjpre)** They go in verbatim after the modelled flags and before the prompt; validation and version checks do not see them. Through `LaunchConfigured`, an `extra_args` option and the request's raw `Args` are both kept, `extra_args` first.
- **[`Tools` and `Worktree` are pointers on purpose.](ds:block?id=claude-launchoptions-build-78wzjpre)** `Tools: &[]string{}` emits `--tools ""` (no tools) while nil emits nothing; `Worktree: &""` emits `--worktree` with no name.

## Related

- [Claude stream decoding](stream.md), [Claude conversations](conversations.md), [Claude credentials and tokens](credential.md) (token logins), [Claude install and updates](install.md), [Claude Runner](runner.md)
- Neutral view: [The Agent interface](../core/agent-and-spec.md), [Logins and tokens](../capabilities/logins.md#step-2--launch-with-a-token), [Per-feature version table](../compat/features.md)
- Example: [`examples/claudelaunch`](../../examples/claudelaunch/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
