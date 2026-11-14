# Homes

A home is one config dir of one agent: `~/.claude`, `~/.claude-work`, `~/.codex`. Each home has its own login, history and settings, and a person often keeps several (personal and work accounts). `agentio.Home` is the handle for one: it knows its folder, its name, whether it is the default, and which agent owns it. You want this page to list a user's homes, open one by folder, or make sure a home is never passed to the wrong agent.

## Before you start

- Modules: `github.com/ubgo/agentio`, plus `claude` (and `codex` for the owner check) as in the example.
- A user home with at least one agent home. The example uses the fixture in `claude/testdata/homes/2.1.325/userhome`, which has `.claude` and `.claude-work`.

## Step 1 — Get the default home

[`DefaultHome()`](ds:block?id=core-defaulthome-6f4pqnem) is Pure: it joins `Host.UserHome` with the Spec's `DefaultHome` and does not check the disk. A `Home`'s fields are unexported, so it can only come from an agent (or [`agentio.NewHome`](ds:block?id=newhome-u52u4c93), meant for vendor packages and `Core`); that is how it always knows its owner.

<!-- snippet: examples/homes/main.go#default-home -->
```go
// defaultHome prints the default home and the Home accessors. A Home is
// built only by an agent (or NewHome in vendor code), so it always knows
// which agent owns it. Pure.
func defaultHome(p printer, a *claude.Agent) {
	def := a.Neutral().DefaultHome()
	p.line(fmt.Sprintf("default: agent=%s name=%s default=%t", def.Agent(), def.Name(), def.IsDefault()))
	p.line(fmt.Sprintf("default: dir=%s", def.Dir()))
	p.line(fmt.Sprintf("default: path=%s", def.Path(settingsFileName)))
	p.line(fmt.Sprintf("default: name is DefaultHomeName: %t", def.Name() == agentio.DefaultHomeName))
	var zero agentio.Home
	p.line(fmt.Sprintf("zero home: IsZero=%t", zero.IsZero()))
}
```

| Method | Meaning |
|---|---|
| `Agent()` | the owning agent's ID |
| `Dir()` | [the absolute, cleaned folder](ds:block?id=newhome-u52u4c93) |
| `Name()` | a short name: `personal` ([`agentio.DefaultHomeName`](ds:block?id=defaulthomename-2x8teypk)) for the default, else from the folder (`~/.claude-work` is `work`) |
| `IsDefault()` | the default home, launched with the home variable absent |
| `Path(elem...)` | a path inside the home |
| `IsZero()` | a `Home` that was never built |
| `String()` | [`agent:dir`](ds:block?id=home-string-ry64gtnm), for messages |
| `Target()` | a plain launch target ([Targets and commands](launch.md)) |

A `Home` marshals to JSON as [`{"agent", "dir", "is_default", "name"}`](ds:block?id=home-marshaljson-gwtr45uj).

## Step 2 — List every home

[`Agent.Homes()`](ds:block?id=core-homes-43a2yjev) lists homes on disk: the default first, then every `ExtraHomes` glob hit sorted (`~/.claude-*` for Claude), then the home the home variable selects if it is not already listed. A folder counts only when it holds [at least one home marker](ds:block?id=core-ishome-nxgj8ve9), so look-alikes such as `~/.claude-worktrees` are skipped.

<!-- snippet: examples/homes/main.go#list-homes -->
```go
// listHomes lists homes on disk through the neutral Agent: the default
// first, then extra homes (~/.claude-*) sorted, then the env-selected one.
// Only folders holding a home marker count. Disk · Read.
func listHomes(p printer, a agentio.Agent) error {
	homes, err := a.Homes()
	if err != nil {
		return fmt.Errorf("homes: %w", err)
	}
	for _, h := range homes {
		p.line(fmt.Sprintf("homes: %s name=%s default=%t", h, h.Name(), h.IsDefault()))
	}
	return nil
}
```

[`HomeFromEnv()`](ds:block?id=core-homefromenv-jk7mncfj) returns the home that `Host.Env` selects through the agent's home variable (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`). It reads the Host's env, never the process env:

<!-- snippet: examples/homes/main.go#home-from-env -->
```go
// homeFromEnv prints the home the Host's base env selects through the
// agent's home variable (CLAUDE_CONFIG_DIR). false when it is absent or
// empty. Pure: it reads Host.Env, never os.Getenv.
func homeFromEnv(p printer, a agentio.Agent) {
	h, ok := a.HomeFromEnv()
	p.line(fmt.Sprintf("from env: ok=%t name=%s dir=%s", ok, h.Name(), h.Dir()))
}
```

[`IsHome(dir)`](ds:block?id=core-ishome-nxgj8ve9) asks the same marker question for any folder:

<!-- snippet: examples/homes/main.go#is-home -->
```go
// isHome asks whether a folder is a home of this agent: it must exist and
// hold at least one marker (settings.json, projects/, .credentials.json,
// .claude.json for Claude). A missing folder is (false, nil). Disk · Read.
func isHome(p printer, a agentio.Agent, userHome string) error {
	for _, rel := range []string{workHomeRel, projectDirRel, "missing"} {
		ok, err := a.IsHome(filepath.Join(userHome, rel))
		if err != nil {
			return err
		}
		p.line(fmt.Sprintf("is home: %s=%t", rel, ok))
	}
	return nil
}
```

## Step 3 — Open a home by folder

When the user names a folder (a profile file, a `--home` flag, a `CLAUDE_CONFIG_DIR` value), build its Home with the vendor `HomeAt` or [`Core.HomeAt`](ds:block?id=core-homeat-c4u3jwew). It is Pure: a relative folder resolves against `Host.UserHome` (on a windows Host a rooted folder without a drive, `\work\h`, lands on the user home's drive, as Windows resolves it), the default folder gives back the default home, and the folder does not have to exist yet (useful before creating a new home).

<!-- snippet: examples/homes/main.go#open-by-folder -->
```go
// openByFolder builds a Home for a folder the caller names. HomeAt is Pure:
// it never checks the disk, so it also opens a home that does not exist
// yet. A relative folder resolves against Host.UserHome; the default folder
// gives back the default home.
func openByFolder(p printer, a *claude.Agent, h agentio.Host) error {
	for _, dir := range []string{workHomeRel, ciHomeDir, ".claude"} {
		home := a.HomeAt(dir)
		p.line(fmt.Sprintf("home at %s: name=%s default=%t dir=%s", dir, home.Name(), home.IsDefault(), home.Dir()))
	}
	// Core.HomeAt does the same for any agent described by a Spec.
	core, err := agentio.NewCore(agentio.Spec{ID: toyID, Name: "Toy", Binary: "toy", HomeEnv: "TOY_HOME", DefaultHome: ".toy"}, h)
	if err != nil {
		return err
	}
	toy := core.HomeAt(".toy-staging")
	p.line(fmt.Sprintf("core home at .toy-staging: %s name=%s", toy, toy.Name()))
	return nil
}
```

## Step 4 — Check who owns a home

Every neutral method that takes a Home first checks that it belongs to the agent. [`agentio.CheckOwner(id, h)`](ds:block?id=checkowner-2237brr8) and `Core.Owns(h)` return an error matching `ErrForeignHome` for another agent's home; the vendor conversion [`claude.Agent.HomeOf`](ds:block?id=claude-agent-homeof-qtt8m9qe) applies the same check.

<!-- snippet: examples/homes/main.go#check-owner -->
```go
// checkOwner shows the guard every neutral method applies: a home of
// another agent is refused with ErrForeignHome instead of being read as if
// it were this agent's. Pure.
func checkOwner(p printer, a *claude.Agent, other agentio.Agent) error {
	codexHome := other.DefaultHome()
	err := agentio.CheckOwner(claude.ID, codexHome)
	p.line(fmt.Sprintf("check owner: foreign=%t err=%v", errors.Is(err, agentio.ErrForeignHome), err))
	// The vendor conversion applies the same check.
	if _, err := a.HomeOf(codexHome); !errors.Is(err, agentio.ErrForeignHome) {
		return fmt.Errorf("HomeOf(codex home) = %v, want ErrForeignHome", err)
	}
	own := a.Neutral().DefaultHome()
	p.line(fmt.Sprintf("check owner: own home err=%v", agentio.CheckOwner(claude.ID, own)))
	return nil
}
```

## Try it

```sh
cd examples && go run ./homes
```

```text
default: agent=claude name=personal default=true
default: dir=~/.claude
default: path=~/.claude/settings.json
default: name is DefaultHomeName: true
zero home: IsZero=true
homes: claude:~/.claude name=personal default=true
homes: claude:~/.claude-work name=work default=false
from env: ok=true name=work dir=~/.claude-work
is home: .claude-work=true
is home: project=false
is home: missing=false
home at .claude-work: name=work default=false dir=~/.claude-work
home at /srv/agents/.claude-ci: name=ci default=false dir=/srv/agents/.claude-ci
home at .claude: name=personal default=true dir=~/.claude
core home at .toy-staging: toy:~/.toy-staging name=staging
check owner: foreign=true err=agentio: home belongs to another agent: codex home passed to claude
check owner: own home err=<nil>
```

`~` stands for the fixture user home.

## Pitfalls

- **[`DefaultHome` and `HomeAt` never touch the disk.](ds:block?id=core-homeat-c4u3jwew)** They return a Home for a folder that may not exist. Use `Homes` or `IsHome` when you need "is there really a home here".
- **A missing folder is not an error.** `IsHome` returns [`(false, nil)`](ds:block?id=core-ishome-nxgj8ve9) for a folder that does not exist or is a file; only a real stat failure (permissions) is an error.
- **[The default home is launched with the variable ABSENT.](ds:block?id=homeenvchange-59nx5j7k)** `IsDefault()` homes get `CLAUDE_CONFIG_DIR` removed from the child env, never set to `~/.claude`: Claude Code would switch keychain entries and look logged out ([Targets and commands](launch.md#step-4--change-the-child-environment)).
- **[An empty home variable counts as absent.](ds:block?id=core-homefromenv-jk7mncfj)** `CLAUDE_CONFIG_DIR=` makes `HomeFromEnv` return false, as the vendor CLI falls back to the default home.
- **[`HomeAt` names the home from its folder.](ds:block?id=core-homename-ep9s5ega)** `.claude-ci` becomes `ci` because it starts with the default folder name plus `-`; any other folder takes its base name without the leading dot. For an agent whose home variable names the parent (`HomeEnvNamesParent`), `~/gemini-work/.gemini` takes the parent's name, `gemini-work`.
- **Never build a Home of one agent and pass it to another.** You get [`ErrForeignHome`](ds:block?id=checkowner-2237brr8), not a silent read of the wrong files. Get homes from the agent that will use them.

## Related

- [Agent, Spec and Core](agent-and-spec.md)
- [Targets and commands](launch.md)
- [Backups, moves and home links](backup-move-link.md)
- [The Claude agent](../claude/agent.md#step-2--pick-a-home)
- [The Codex agent](../codex/agent.md#step-2--pick-a-home)
- Example: [`examples/homes`](../../examples/homes/)
- [Feature index](../reference/features.md)
