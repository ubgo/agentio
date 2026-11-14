# Gemini CLI (gemini)

The `gemini` module is agentio's typed API for Google's Gemini CLI (`@google/gemini-cli`). It reads the install and its version from package metadata, the homes, every settings layer with all keys kept, conversations per project, instruction files, MCP servers, skills and extensions, and the login state. It also builds the command that launches or resumes Gemini with an approval mode, a headless stream or an API key. It never logs in, never refreshes or prints a token, and writes nothing Gemini owns.

Every shape is recorded from Gemini CLI 0.68.0. Files the CLI writes without a login (`projects.json`, `.project_root`, `logs.json`, `state.json`) were observed on a real install; files that need a logged-in run (chats, `oauth_creds.json`, `google_accounts.json`) are modelled from the CLI's bundled source and marked so in the fixture. A recording run with a real login is still owed for those.

## Before you start

- Add the module: `go get github.com/ubgo/agentio/gemini`. It is stdlib only, like the root module.
- A Host. `agentio.HostFromOS()` reads the real machine; the example builds one on a temp copy of the fixture home.
- Know the home rule: `GEMINI_CLI_HOME` names the dir that **contains** `.gemini`, not `.gemini` itself. `~/gemini-work/.gemini` is selected with `GEMINI_CLI_HOME=~/gemini-work`, and every non-default home must be a `.gemini` dir inside its parent.

The snippets come from [`examples/gemini`](../../examples/gemini/), which runs on the checked-in fixture home `gemini/testdata/homes/0.68.0`.

## Step 1 — Homes

`gemini.New(host)` returns the agent. `DefaultHome()` is `~/.gemini` (named `personal`); `HomeAt(dir)` takes the `.gemini` dir itself, and its name comes from the parent dir (`gemini-work`). `HomeFromEnv()` reads `GEMINI_CLI_HOME` and appends `.gemini`; `Homes()` lists the default home and that one, each only when it holds a marker (`settings.json`, `projects.json`, `oauth_creds.json`, `google_accounts.json` or `tmp/`). A launch on the default home removes `GEMINI_CLI_HOME`; a launch on another home sets it to the home's parent.

<!-- snippet: examples/gemini/main.go#homes -->
```go
// homes shows the default home and a second one selected through
// GEMINI_CLI_HOME, which names the dir that CONTAINS .gemini.
func homes(p printer, a *gemini.Agent, m machine) error {
	def := a.DefaultHome()
	p(fmt.Sprintf("default: %s name=%s", def.Dir(), def.Name()))
	work := a.HomeAt(filepath.Join(m.host.UserHome, "gemini-work", ".gemini"))
	cmd, err := a.Launch(work.Target(), agentio.LaunchRequest{})
	if err != nil {
		return err
	}
	parent, _ := agentio.EnvLookup(cmd.Env, gemini.HomeEnv)
	p(fmt.Sprintf("work: %s name=%s launches with %s=%s", work.Dir(), work.Name(), gemini.HomeEnv, parent))
	return nil
}
```

## Step 2 — Install and version

`Install()` finds `gemini` on the Host's `PATH` (never a shell) and reads the version from the `package.json` of `@google/gemini-cli` above the resolved binary: npm, pnpm, Bun, Volta and the Homebrew formula (which installs through npm) all work, and no process runs. `Runner().BinaryVersion(ctx)` runs `gemini --version` when you need the binary's own answer; holding the Runner is the opt-in to running it. `TestedMin` and `TestedMax` are the versions the fixtures cover, and the doctor warns outside them.

Gemini CLI updates itself by default (`general.enableAutoUpdate`) on npm-family installs, so the installed version can change between two reads; the doctor reports it as an info check.

<!-- snippet: examples/gemini/main.go#install -->
```go
// install reads the version from the npm package's package.json next to
// the resolved binary: no process runs.
func install(p printer, a *gemini.Agent) error {
	in, err := a.Install()
	if err != nil {
		return err
	}
	p(fmt.Sprintf("install: %s method=%s version=%s", filepath.Base(in.Path), in.Method, in.Version))
	p(fmt.Sprintf("tested range: %s..%s", gemini.TestedMin, gemini.TestedMax))
	return nil
}
```

## Step 3 — Settings

`Settings(h, dir)` reads the layers highest precedence first: system (`/etc/gemini-cli/settings.json`, `/Library/Application Support/GeminiCli/settings.json` or `C:\ProgramData\gemini-cli\settings.json`, or `GEMINI_CLI_SYSTEM_SETTINGS_PATH`), the workspace (`<dir>/.gemini/settings.json`), the user (`<home>/settings.json`) and the system defaults. Each `SettingsFile` carries a `gemini.Settings`:

- the sections agentio uses are typed: `general`, `model`, `context`, `security` (with `auth.selectedType`), `privacy`, `tools`, `mcp` and `mcpServers`;
- every other key, top level or inside a typed section, is kept raw in `Unknown`, and `MarshalJSON` writes it back, so nothing is lost on a round trip;
- a typed section with an unexpected type is kept raw too, with a warning, the way Gemini CLI itself keeps such keys;
- comments are allowed, as in Gemini CLI (`StripJSONComments`).

`EffectiveSettings(h, dir)` merges the model, approval mode, auth type, context file names and MCP servers; `Defaults(h, dir)` maps them onto `agentio.Defaults`. The workspace layer is merged even for an untrusted folder, where the CLI would skip it; check `ProjectTrust` when that matters.

<!-- snippet: examples/gemini/main.go#settings -->
```go
// settings lists the layers (highest first) and the merged values; keys
// agentio does not model are kept, so nothing is lost on a round trip.
func settings(p printer, a *gemini.Agent, m machine) error {
	h := a.DefaultHome()
	layers, err := a.Settings(h, m.project)
	if err != nil {
		return err
	}
	for _, l := range layers {
		p(fmt.Sprintf("layer %-8s exists=%-5t %s", l.Scope, l.Exists, l.Path))
	}
	user := layers[2].Settings
	p(fmt.Sprintf("user auth=%s approval=%s unknown keys=%v", user.Security.Auth.SelectedType, user.General.DefaultApprovalMode, user.Unknown.Keys()))
	e, err := a.EffectiveSettings(h, m.project)
	if err != nil {
		return err
	}
	d, err := a.Defaults(h, m.project)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("effective model=%s (workspace wins) approval=%s -> autonomy %s", e.Model, e.ApprovalMode, d.Autonomy))
	return nil
}
```

## Step 4 — Conversations and resume

Gemini keys chats by project. `projects.json` maps each absolute project dir to a slug (`Slugify` of the dir's base name, with `-1`, `-2` on a clash), and chats live in `<home>/tmp/<slug>/chats/session-<time>-<id8>.jsonl`. `ProjectSlug(h, dir)` looks a project up the way the CLI does: the registry first, then each `tmp/<slug>/.project_root` marker.

`Sessions(ctx, h, dir)` lists resumable chats newest first. It reads only the head of each file (the metadata line and the first user message) and its last 256 KiB (the newest `$set` summary and update time); legacy single-object `.json` chats are streamed. As in `gemini --list-sessions`, a chat with no user or model message and a subagent chat are skipped, and the title is the summary or else the first prompt that is not a `/command`. `Session.ID` is the full id that `gemini --resume <id>` takes. Resume is per project, so launch it with `Dir` set to the session's project dir.

<!-- snippet: examples/gemini/main.go#conversations -->
```go
// conversations lists resumable chats, newest first, with the id
// `gemini --resume` takes; resume runs in the chat's project dir.
func conversations(ctx context.Context, p printer, a *gemini.Agent, m machine) error {
	h := a.DefaultHome()
	slug, _, err := a.ProjectSlug(h, m.project)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("project slug=%s (Slugify of the base name: %s)", slug, gemini.Slugify("My Project")))
	ss, err := a.Sessions(ctx, h, m.project)
	if err != nil {
		return err
	}
	for _, s := range ss {
		p(fmt.Sprintf("  %s  %-36q legacy=%-5t updated %s", s.ID[:8], s.Title, s.Legacy, s.Updated.Format("2006-01-02T15:04Z")))
	}
	cmd, err := a.Launch(h.Target(), agentio.LaunchRequest{ResumeID: ss[0].ID, Dir: m.project})
	if err != nil {
		return err
	}
	p(fmt.Sprintf("resume: %s (in %s)", cmd, cmd.Dir))
	return nil
}
```

## Step 5 — Instructions, MCP servers, skills and extensions

`InstructionFiles(h, dir)` lists the files the CLI loads, in order: `<home>/GEMINI.md` (or the `context.fileName` names), the project memory `tmp/<slug>/memory/MEMORY.md`, then every context file from the git root down to `dir`. A folder `trustedFolders.json` marks `DO_NOT_TRUST` loads no project file. Files in subdirectories the CLI loads only when a tool touches them are not listed.

`MCPServers(h, dir)` merges `mcpServers` from the settings layers by name, adds the servers of enabled extensions, and applies `mcp-server-enablement.json` and `mcp.excluded`. Env and header values stay in `agentio.EnvValue` (redacted); the neutral view carries env names only. `Skills(h, dir)` reads `<home>/skills`, the home parent's `.agents/skills`, the project's `.gemini/skills` and `.agents/skills`, and each enabled extension's `skills/`; `skills.disabled` marks a skill disabled. `Extensions(h, dir)` reads `extensions/<name>/gemini-extension.json` and applies `extension-enablement.json` to `dir`, the way the CLI does: a plain rule enables, a `!` rule disables, the last match wins.

<!-- snippet: examples/gemini/main.go#instructions -->
```go
// instructionsAndMCP lists the GEMINI.md files loaded in a subdir (root
// first) and the MCP servers that apply, env names only.
func instructionsAndMCP(p printer, a *gemini.Agent, m machine) error {
	h := a.DefaultHome()
	files, err := a.InstructionFiles(h, filepath.Join(m.project, "sub"))
	if err != nil {
		return err
	}
	for _, f := range files {
		p(fmt.Sprintf("instructions %-7s %s", f.Scope, f.Path))
	}
	servers, err := a.MCPServers(h, m.project)
	if err != nil {
		return err
	}
	for _, s := range servers {
		n := s.Neutral()
		enabled := "-"
		if n.Enabled != nil {
			enabled = fmt.Sprint(*n.Enabled)
		}
		p(fmt.Sprintf("mcp %-15s %-5s scope=%-7s enabled=%-5s env=%v", n.Name, n.Transport, n.Scope, enabled, n.EnvNames))
	}
	skills, err := a.Skills(h, m.project)
	if err != nil {
		return err
	}
	for _, s := range skills {
		line := fmt.Sprintf("skill %-10s scope=%-7s enabled=%t", s.Name, s.Scope, s.Enabled)
		if s.Extension != "" {
			line += " extension=" + s.Extension
		}
		p(line)
	}
	return nil
}
```

## Step 6 — Login

`Identity(h)` reads the login without the network. For `oauth-personal` (the Google login) it is logged in when `oauth_creds.json` holds a token, with the email from `google_accounts.json` (an `agentio.Personal`, redacted when printed) and the expiry from `expiry_date`. For `gemini-api-key` and `vertex-ai` it checks the variable names in the Host env and reports per-token billing. The CLI refreshes its own tokens; agentio only reads.

<!-- snippet: examples/gemini/main.go#login -->
```go
// login reads the home's login from disk: the auth type, the cached
// Google login (email redacted) and its expiry. No token is printed.
func login(p printer, a *gemini.Agent) error {
	id, err := a.Identity(a.DefaultHome())
	if err != nil {
		return err
	}
	p(fmt.Sprintf("auth=%s logged-in=%t credential=%s email=%s expires=%s", id.AuthType, id.LoggedIn, id.Credential, id.Email, id.ExpiresAt.Format("2006-01-02")))
	return nil
}
```

## Step 7 — Launch

`Launch(t, r)` builds the command; nothing runs.

| Request | Gemini flags or env |
|---|---|
| `Prompt` | `--prompt-interactive "<prompt>"`: an initial prompt in an interactive session |
| `Headless` with `Prompt` | `--output-format stream-json --prompt "<prompt>"`: `--prompt` is what makes the run non-interactive |
| `Autonomy` read-only, ask, accept-edits, full | `--approval-mode plan`, `default`, `auto_edit`, `yolo` (full needs `Target.AllowDangerous`) |
| `Autonomy` default | no flag, so `general.defaultApprovalMode` applies |
| `Model`, `AddDirs` | `--model`, `--include-directories` per dir |
| `ResumeID` | `--resume <id>` |
| API key credential | `GEMINI_API_KEY`, or `GOOGLE_API_KEY` with provider `vertex` |

Credential scrubbing removes only what would override the home's stored login: `GOOGLE_CLOUD_ACCESS_TOKEN` and `GOOGLE_GENAI_USE_GCA` (together they replace the cached Google login), `GOOGLE_APPLICATION_CREDENTIALS` (used silently when the home's file is invalid) and `GEMINI_CLI_AUTH_OVERRIDE`. `GEMINI_API_KEY` is kept: the home's `selectedType` wins over it, and for an API-key home it is the login.

<!-- snippet: examples/gemini/main.go#launch -->
```go
// launch builds commands; nothing is run. Autonomy maps onto
// --approval-mode, Headless onto --prompt plus stream-json, and a token
// login onto GEMINI_API_KEY.
func launch(p printer, a *gemini.Agent) error {
	t := a.DefaultHome().Target()
	t.AllowDangerous = true
	for _, r := range []agentio.LaunchRequest{
		{Prompt: "explain the parser"},
		{Autonomy: agentio.AutonomyReadOnly, Model: "gemini-2.5-flash"},
		{Autonomy: agentio.AutonomyFull, Headless: true, Prompt: "fix the failing tests", AddDirs: []string{"../lib"}},
	} {
		cmd, err := a.Launch(t, r)
		if err != nil {
			return err
		}
		p(fmt.Sprintf("launch: %s", cmd))
	}
	cmd, err := a.Launch(t, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("example-key")}})
	if err != nil {
		return err
	}
	key, _ := agentio.EnvLookup(cmd.Env, gemini.APIKeyEnv)
	p(fmt.Sprintf("token launch sets %s (value redacted in output: %t)", gemini.APIKeyEnv, !strings.Contains(cmd.String(), key)))
	if _, err := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); err != nil {
		p(fmt.Sprintf("full without AllowDangerous: %v", err))
	}
	return nil
}
```

## Capabilities

| Capability | Gemini |
|---|---|
| install, binary version, process classifier | yes |
| identity, token login (API key) | yes |
| conversations, history checker | yes |
| instruction files, MCP servers, skills, plugins (extensions), config, doctor, home linker | yes |
| usage, subscription | no: quota and tier come from a logged-in Code Assist API call whose responses are not recorded yet |
| cached usage | no: nothing on disk |
| token prober | no: probing spends quota through an unmodelled model call |
| login commander | no: Gemini CLI has no login subcommand |
| history and conversation movers | no: chats are keyed by the slug registry |
| prompt history | no: `logs.json` is recorded only from source |
| live sessions | no: no session lock or pid file |
| turns | no: chats rewrite earlier records (`$rewindTo`, `$patch`) |
| models | no: the catalog lives in the CLI's own defaults |
| run events | no: the stream-json event shape is not recorded yet |

## Pitfalls

- **`GEMINI_CLI_HOME` is the parent.** Setting it to a `.gemini` dir makes Gemini use `<that dir>/.gemini`.
- **Two processes per session.** Gemini relaunches itself as a child with `GEMINI_CLI_NO_RELAUNCH=true`; `ClassifyProcess` matches both, so keep the child when you count sessions.
- **Old chats disappear.** `general.sessionRetention` deletes chats older than 30 days at the CLI's startup by default.
- **The keychain is shared.** With `GEMINI_FORCE_ENCRYPTED_FILE_STORAGE`, the Google login lives in a keychain entry with a fixed name, shared by every home, and `Identity` (which reads `oauth_creds.json`) reports logged out.
- **`.env` files inject keys.** Gemini reads `GEMINI_API_KEY` from `.env` files in the project and the home when it is not in the environment, which agentio does not model.

## Related

- Example: [`examples/gemini`](../../examples/gemini/)
- [Homes](../core/homes.md), [Targets and commands](../core/launch.md), [Logins](../capabilities/logins.md)
