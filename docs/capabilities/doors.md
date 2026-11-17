# Doors: Runner, Prober and Writer

Most of agentio reads files. Anything else goes through a door: a separate value you must ask for before you can make the call. The Runner runs the vendor binary (Process, may touch the login), the Prober spends a little of the account's quota, and the Writer changes files the vendor CLI owns, [always backing them up first](ds:block?id=claude-writer-t5gq26t8). Holding the door is the opt-in, so a reviewer can see every risky call by searching for `.Runner()`, `.Prober()` and `.Writer()`. [`HomeLinker`](ds:block?id=homelinker-ek5zfyg7) is the neutral write door for linking shared files into a new home.

## Before you start

- Modules: `github.com/ubgo/agentio`, `github.com/ubgo/agentio/claude`, `github.com/ubgo/agentio/codex`.
- Runner: `Host.Exec`, a `ProcessRunner`. `HostFromOS` uses [`os/exec`](ds:block?id=osrunner-a75g5r5m); tests use `agenttest.FakeExec` or a canned runner.
- Prober: `Host.HTTP`; in tests, `claude.New(h).WithBaseURL(srv.URL)` with an `httptest` server.
- Writer and HomeLinker: a home you are allowed to change. Examples and tests copy a fixture first.

[`examples/doors`](../../examples/doors/) uses a recording runner with canned `--version` answers, an in-process fake API for the probe, and a temp copy of the Claude fixture home for the writes. At the end it prints every command that reached `Host.Exec`.

## Step 1 — Run the vendor binary through a Runner

[`claude.Agent.Runner()`](ds:block?id=claude-agent-runner-8mqvmpgg) and [`codex.Agent.Runner()`](ds:block?id=codex-agent-runner-v8k7es62) return the vendor Runners: `BinaryVersion` plus vendor subcommands with typed results (Claude [`AuthStatus`](ds:block?id=claude-runner-authstatus-ktzmqmsf), `Plugins`, `Marketplaces`; Codex `LoginStatus`, `Features`, [`BundledModels`](ds:block?id=codex-runner-bundledmodels-cpzy9yw7)). The root [`agentio.Core.Runner()`](ds:block?id=core-runner-4jzfzpz9) works for any Spec: [`Run(ctx, cmd, stdin)`](ds:block?id=runner-run-5rn5nerj) runs a command through `Host.Exec`, and [`Version(ctx)`](ds:block?id=runner-version-87cgudkv) runs the Spec's `VersionArgs` and parses the first version in the output.

<!-- snippet: examples/doors/main.go#runner -->
```go
// useRunners runs `--version` through each vendor Runner and through the
// root Runner of a Spec-built core. Holding a Runner is the opt-in: no read
// method of the library ever runs a binary.
func useRunners(ctx context.Context, w io.Writer, h agentio.Host) error {
	cv, err := claude.New(h).Runner().BinaryVersion(ctx)
	if err != nil {
		return err
	}
	xv, err := codex.New(h).Runner().BinaryVersion(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "claude %s, codex %s\n", cv, xv)

	// The root Runner works for any Spec with VersionArgs.
	core, err := agentio.NewCore(agentio.Spec{ID: "aider", Name: "Aider", Binary: "aider", VersionArgs: []string{"--version"}}, h)
	if err != nil {
		return err
	}
	av, err := core.Runner().Version(ctx)
	if err != nil {
		return err
	}
	cmd, err := core.Command(core.DefaultHome().Target(), "", []string{"--help"}, nil)
	if err != nil {
		return err
	}
	res, err := core.Runner().Run(ctx, cmd, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "aider %s; `%s` exited %d\n", av, cmd, res.ExitCode)
	return nil
}
```

`Runner.Run` reports a non-zero exit in [`ProcessResult.ExitCode`](ds:block?id=osrunner-run-hm7czx3q) with a nil error; the error means the program could not run at all. [The vendor Runners turn a non-zero exit into an error](ds:block?id=claude-runner-run-af4j3992) carrying a stderr snippet, except Codex `LoginStatus`, which reads the exit code itself. Details: [Claude runner](../claude/runner.md), [Codex install](../codex/install.md#step-3--run-the-binary-through-the-runner).

## Step 2 — Spend quota through a Prober

[`claude.Agent.Prober()`](ds:block?id=claude-agent-prober-cgcpxdhn) returns the Prober. [`TokenUsage(ctx, cred)`](ds:block?id=claude-quota-door-tokenusage-j2fw66uu) sends one minimal request with a long-lived plan token and reads the plan windows from the rate-limit headers of the reply. It returns the rich `claude.Usage`; the neutral [`TokenProberOf`](ds:block?id=claude-neutral-tokenusage-m94wnsb5) accessor returns the same as `agentio.Usage`.

<!-- snippet: examples/doors/main.go#prober -->
```go
// useProber reads a token's plan usage from one minimal API request. It
// spends quota, so it lives behind its own door; call it once on demand.
func useProber(ctx context.Context, w io.Writer, h agentio.Host, baseURL string) error {
	p := claude.New(h).WithBaseURL(baseURL).Prober()
	cred := agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken(sampleOAuthToken)}
	u, err := p.TokenUsage(ctx, cred)
	if err != nil {
		return err
	}
	for _, win := range u.Windows {
		fmt.Fprintf(w, "%s %.0f%% resets %s\n", win.Label, win.Percent, win.ResetsAt.UTC().Format("15:04 MST"))
	}
	return nil
}
```

[Codex has no Prober](ds:block?id=codex-neutral-unsupportedreason-evgu3az7): it has no token login to probe. See [Claude usage](../claude/usage.md#step-4--probe-a-tokens-usage) and [Logins and tokens](logins.md#step-3--probe-a-token).

## Step 3 — Write through a Writer

[`claude.Agent.Writer()`](ds:block?id=claude-agent-writer-bwuq47c6) returns the Writer. Every method that overwrites something copies it first to [`<file>.agentio-bak-<UTC stamp>`](ds:block?id=backuppath-tfjr5hjr) (never overwriting an earlier backup) and returns an `agentio.Backup` listing each original and its copy. [Writes are atomic](ds:block?id=claude-writer-writerawsettings-6sjvkkgu) (temp file, then rename) and keep the values of keys they do not touch, though the file is re-indented with its keys sorted. Methods: `SetUserSetting`, `SetProjectSetting`, `AddHook`, `RemoveHooks`, `MoveHistory`, `ApplyMove`, `ApplyHomeLinks`.

<!-- snippet: examples/doors/main.go#writer -->
```go
// useWriter changes one top-level key of the default home's settings.json.
// The Writer backs the file up first and returns where the copy went; other
// keys keep their values (the file is rewritten indented). Codex's Writer exists but has no methods yet:
// Codex config changes are vendor commands.
func useWriter(w io.Writer, h agentio.Host) error {
	c := claude.New(h)
	b, err := c.Writer().SetUserSetting(c.DefaultHome(), claude.SettingsPatch{
		Set: map[string]agentio.Raw{themeKey: agentio.Raw(themeValue)},
	})
	if err != nil {
		return err
	}
	for _, e := range b.Entries {
		fmt.Fprintf(w, "backed up %s\n       to %s\n", tidy(h, e.Original), tidy(h, e.Copy))
	}
	s, err := c.UserSettings(c.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "theme now %q\n", s.Settings.Theme)
	_ = codex.New(h).Writer() // no methods yet
	return nil
}
```

[`codex.Agent.Writer()`](ds:block?id=codex-writer-tax23vch) exists but has no methods yet: Codex config changes are vendor commands built as data (such as `FeatureCommand` and `MCPAddCommand`), so there is no Codex file agentio writes today. Details: [Claude writer](../claude/writer.md).

## Step 4 — Link shared home files

A new home (say `~/.claude-work`) starts empty. [`HomeLinkerOf(a)`](ds:block?id=homelinkerof-fk3qdrnr) returns the neutral `HomeLinker`: [`PlanHomeLinks(def, home)`](ds:block?id=core-planhomelinks-zybhmbpb) plans one symlink per shared file of the default home (from the Spec's `SharedHomeFiles`: for Claude, `CLAUDE.md`, `skills` and [`settings.json`](ds:block?id=claude-sharedhomefiles-3vpa9mqk)), and [`ApplyHomeLinks(links)`](ds:block?id=core-applyhomelinks-q5mmu4xp) creates them. Logins, history and caches stay per home.

<!-- snippet: examples/doors/main.go#linker -->
```go
// linkHome plans and makes the symlinks that let a new home share the
// default home's instructions, skills and settings. Planning is a read;
// applying only creates symlinks and never replaces an existing file.
func linkHome(w io.Writer, h agentio.Host) error {
	c := claude.New(h)
	linker, ok := agentio.HomeLinkerOf(c.Neutral())
	if !ok {
		return fmt.Errorf("claude has no home linker")
	}
	def := c.DefaultHome().Neutral()
	fresh := c.HomeAt(filepath.Join(h.UserHome, newHomeDir)).Neutral()
	links, err := linker.PlanHomeLinks(def, fresh)
	if err != nil {
		return err
	}
	if err := linker.ApplyHomeLinks(links); err != nil {
		return err
	}
	for _, l := range links {
		fmt.Fprintf(w, "%s -> %s (existed %t)\n", tidy(h, l.To), tidy(h, l.From), l.Exists)
	}
	again, err := linker.PlanHomeLinks(def, fresh)
	if err != nil {
		return err
	}
	for _, l := range again {
		fmt.Fprintf(w, "replan %s exists=%t\n", l.Name, l.Exists)
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./doors
```

Output:

```text
== runner ==
claude 2.1.325, codex 0.162.1
aider 0.86.1; `aider --help` exited 1
== prober ==
Current session 64% resets 16:00 UTC
== writer ==
backed up ~/.claude/settings.json
       to ~/.claude/settings.json.agentio-bak-20261113T120000Z
theme now "dark"
== home linker ==
~/.claude-new/CLAUDE.md -> ~/.claude/CLAUDE.md (existed false)
~/.claude-new/skills -> ~/.claude/skills (existed false)
~/.claude-new/settings.json -> ~/.claude/settings.json (existed false)
replan CLAUDE.md exists=true
replan skills exists=true
replan settings.json exists=true
== host.exec calls ==
claude --version
codex --version
aider --version
aider --help
```

Only the four Runner calls reached `Host.Exec`; the probe, the write and the links never run a program.

## Pitfalls

- [**Every Runner call may touch the login.**](ds:block?id=claude-runner-dbm6t8tp) Vendor CLIs can refresh and rewrite credentials when they start. Never call a Runner method in a loop over homes or on a timer, and never call one implicitly; nothing in the library does.
- **A Host without `Exec` has no Runner.** [Claude Runner methods](ds:block?id=claude-runner-run-af4j3992) return an `ErrUnsupported` error ("the host has no process runner"); Codex Runner methods and the root [`Runner.Run`](ds:block?id=runner-run-5rn5nerj) return a plain error ("no process runner in host") that does not match `ErrUnsupported`.
- **Never loop the Prober.** Each call spends quota. An API key is never probed ([`ErrBilledPerToken`](ds:block?id=claude-quota-door-tokenusage-j2fw66uu)).
- **The Writer never edits [`~/.claude.json`](ds:block?id=claude-writer-t5gq26t8) or a project `.mcp.json`.** Those are changed through vendor commands built as data (for example `claude mcp add`); run them yourself.
- **Keep the `Backup`.** It is the only record of where the original went; show it to the user or store it. Backups are never cleaned up by the library.
- **`ApplyHomeLinks` returns no [`Backup`](ds:block?id=applyhomelinks-svx7cuvm).** It only creates symlinks, skips entries that already exist (`Exists` true) and never replaces a file, so there is nothing to back up. [Re-planning after applying shows every link as existing](ds:block?id=planhomelinks-yg65h8jg).
- **`ApplyHomeLinks` checks ownership.** Each `HomeLink` carries the `Agent` that planned it; the neutral `HomeLinker` and [`claude.Writer.ApplyHomeLinks`](ds:block?id=claude-writer-applyhomelinks-39hcpw7p) refuse a link of another agent with [`agentio.ErrForeignHome`](ds:block?id=core-applyhomelinks-q5mmu4xp) before linking anything, as `ApplyMove` does.
- **Codex has no write methods.** [`codex.Agent.Writer()`](ds:block?id=codex-writer-tax23vch) is an empty door today; do not expect a Codex settings writer.

## Related

- [Shared capabilities and accessors](accessors.md): the `Cost` of each capability.
- [Logins and tokens](logins.md), [Install, version and identity](install-identity.md).
- [Backup, move and link](../core/backup-move-link.md): `Backup`, `BackupFile`, `PlanHomeLinks`, `ApplyHomeLinks`.
- Vendor pages: [Claude runner](../claude/runner.md), [Claude writer](../claude/writer.md), [Claude usage](../claude/usage.md#step-4--probe-a-tokens-usage), [Codex install](../codex/install.md#step-3--run-the-binary-through-the-runner).
- [Testing with fakes](../testing/agenttest.md): `FakeExec`, `httptest`, `CopyFixture`.
- [Choosing guides](../choosing/README.md). Example: [`examples/doors`](../../examples/doors/). Feature index: [features](../reference/features.md).
