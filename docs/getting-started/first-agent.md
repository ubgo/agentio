# Your first agent

This page builds the smallest useful agentio program: make a `Host`, create the Claude Code agent on it, read how Claude Code is installed, and list its homes (config dirs such as `~/.claude` and `~/.claude-work`). Every later page builds on these four steps. The code is [`examples/quickstart`](../../examples/quickstart/), which runs on a fixture home so you can try it without Claude Code installed.

## Before you start

- `go get github.com/ubgo/agentio github.com/ubgo/agentio/claude` ([Install agentio](install.md)).
- For the real machine: Claude Code installed and at least one home on disk. For the example: nothing, it brings its own fixture.

Imports used on this page:

```go
import (
	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)
```

## Step 1 — Build a Host

A `Host` is everything the library may look at: the user home, the base environment, the clock, an HTTP client, the keychain, the process table and a way to run programs. Nothing in agentio reads [`$HOME`](ds:block?id=host-4pbbeq6s) or `os.Environ` behind your back. On the real machine one call builds it:

<!-- snippet: examples/quickstart/main.go#host-from-os -->
```go
// osHost is the one constructor that reads the real machine: $HOME,
// os.Environ(), the real clock, the keychain, the process table and an
// os/exec runner. Disk · Read (it only resolves the home dir).
func osHost() (agentio.Host, error) {
	h, err := agentio.HostFromOS()
	if err != nil {
		return agentio.Host{}, fmt.Errorf("host: %w", err)
	}
	return h, nil
}

```

For tests and examples you fill the struct yourself and point it at a fixture. Fields you leave nil mean "none": [no keychain entry is ever found](ds:block?id=claude-agent-keychainget-uj32ktdh), no process is listed or run.

<!-- snippet: examples/quickstart/main.go#build-host -->
```go
// fixtureHost builds a Host by hand. Every field is an input the library
// would otherwise take from the machine; a nil Secrets, Processes or Exec
// means "none": keychain reads find nothing, no process is ever listed or
// run. Pure.
func fixtureHost(userHome, pathList string) agentio.Host {
	return agentio.Host{
		UserHome: userHome,
		User:     fixtureUser,
		// Env is the base env every child command starts from. PATH here is
		// what LookPath walks; no shell is ever asked.
		Env:  []string{"PATH=" + pathList, "HOME=" + userHome},
		Now:  func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) },
		HTTP: &http.Client{Timeout: httpTimeout},
	}
}
```

## Step 2 — Create the Claude agent

[`claude.New`](ds:block?id=claude-new-2b7v6ezn) binds Claude Code to a Host. It does no I/O and returns no error, so it is cheap to call wherever you need it.

<!-- snippet: examples/quickstart/main.go#create-agent -->
```go
// newAgent creates Claude Code on h. New is Pure: no I/O and no error, so
// it is safe to call per request.
func newAgent(h agentio.Host) *claude.Agent {
	return claude.New(h)
}
```

`*claude.Agent` is the rich, Claude-specific API. When you want the same code to work for every agent, take its neutral view with [`Neutral()`](ds:block?id=claude-agent-neutral-eh63bcpc):

<!-- snippet: examples/quickstart/main.go#neutral-view -->
```go
// neutralView prints the vendor-neutral view: the agentio.Agent every
// agent shares, for code that loops over agents. Pure.
func neutralView(p printer, a *claude.Agent) {
	n := a.Neutral()
	p.line(fmt.Sprintf("neutral: id=%s name=%q binary=%s", n.ID(), n.Name(), n.Spec().Binary))
}
```

## Step 3 — Read the install

[`Install`](ds:block?id=claude-agent-install-472s4jn6) finds `claude` by walking `PATH` from `Host.Env` (never a shell), resolves symlinks, and reads the version from disk (the npm `package.json` or the native versions folder name). It never runs the binary.

<!-- snippet: examples/quickstart/main.go#read-install -->
```go
// readInstall reports how claude is installed. Disk · Read: PATH walk,
// symlinks, package.json and ~/.claude.json; it never runs the binary.
func readInstall(p printer, a *claude.Agent) error {
	inst, err := a.Install()
	if errors.Is(err, agentio.ErrNotInstalled) {
		p.line("install: claude is not on PATH")
		return nil
	}
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	p.line(fmt.Sprintf("install: path=%s", inst.Path))
	p.line(fmt.Sprintf("install: real=%s", inst.RealPath))
	p.line(fmt.Sprintf("install: method=%s version=%s", inst.Method, inst.Version))
	for _, s := range inst.Shadowed {
		p.line(fmt.Sprintf("install: shadowed=%s", s))
	}
	return nil
}
```

## Step 4 — List the homes

[`Homes`](ds:block?id=core-homes-43a2yjev) returns the default home `~/.claude` first, then every `~/.claude-<name>` that holds a home marker file, then the home `CLAUDE_CONFIG_DIR` selects if it is not already listed.

<!-- snippet: examples/quickstart/main.go#list-homes -->
```go
// listHomes prints every Claude home: ~/.claude first, then each
// ~/.claude-<name> that holds a home marker, then CLAUDE_CONFIG_DIR's.
// Disk · Read.
func listHomes(p printer, a *claude.Agent) error {
	homes, err := a.Homes()
	if err != nil {
		return fmt.Errorf("homes: %w", err)
	}
	for _, h := range homes {
		p.line(fmt.Sprintf("home: name=%s default=%t dir=%s", h.Name(), h.IsDefault(), h.Dir()))
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./quickstart
```

```text
host: user=user PATH set=true
host: MODE=b EMPTY="" set=true MISSING set=false
install: path=$TMP/bin/claude
install: real=$TMP/lib/node_modules/@anthropic-ai/claude-code/cli.js
install: method=npm-global version=2.1.325
install: shadowed=$TMP/old/bin/claude
home: name=personal default=true dir=~/.claude
home: name=work default=false dir=~/.claude-work
neutral: id=claude name="Claude Code" binary=claude
lookpath: first=$TMP/bin/claude
lookpath: hit[0]=$TMP/bin/claude
lookpath: hit[1]=$TMP/old/bin/claude
empty PATH: agentio: binary not found on PATH: claude
```

The rest of the output belongs to [Agent, Spec and Core](../core/agent-and-spec.md). To run the same report against your own machine (read-only), pass `-os`:

```sh
cd examples && go run ./quickstart -os
```

## Pitfalls

- **No `claude` on PATH is an answer, not a crash.** `Install` and [`LookPath`](ds:block?id=lookpath-f56nfyzk) return an error matching `agentio.ErrNotInstalled`; check it with `errors.Is` and carry on reading homes, which only need the files.
- **PATH comes from `Host.Env`.** A Host built by hand with no `PATH` entry [finds nothing](ds:block?id=core-lookpath-ke3ntet7), even when `claude` is installed. [`HostFromOS`](ds:block?id=hostfromos-yvmqkxwe) copies the process environment.
- **The version is the on-disk one.** A Homebrew cask install reports an [unknown (zero) version](ds:block?id=claude-methodandversion-8h4tytjj) because there is no version file to read; running `claude --version` is a separate, explicit call through the Runner door ([Doors](../capabilities/doors.md)).
- **[`Install.Shadowed`](ds:block?id=claude-agent-install-472s4jn6) lists other copies on PATH.** Only the first one runs; a stale second copy is a common cause of "I updated but nothing changed".
- **The default home is called `personal`.** Its `Name()` is [`agentio.DefaultHomeName`](ds:block?id=defaulthomename-2x8teypk); other homes [take their name from the folder](ds:block?id=core-homename-ep9s5ega) (`~/.claude-work` is `work`).

## Related

- [Host: the only door to the machine](../core/host.md)
- [Agent, Spec and Core](../core/agent-and-spec.md)
- [Homes](../core/homes.md)
- [The Claude agent](../claude/agent.md)
- [Install, version and identity](../capabilities/install-identity.md)
- Example: [`examples/quickstart`](../../examples/quickstart/)
- [Feature index](../reference/features.md)
