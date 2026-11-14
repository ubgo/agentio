# Host: the only door to the machine

A `Host` is the machine agentio looks at: the user home, the base environment, the clock, an HTTP client, the keychain, the process table and a way to run a program. No function in the library reads `$HOME`, `os.Environ`, the keychain, the clock or the network except through the Host you pass in. That is what lets a test point every input at a fixture, and lets one program inspect two machines' worth of homes side by side.

## Before you start

- Module: `github.com/ubgo/agentio` (stdlib only).
- Nothing on disk is required to build a Host; `HostFromOS` only resolves the home dir.

## Step 1 — Build a Host from the OS

`agentio.HostFromOS` is the one constructor that touches the real machine. It fills `UserHome` from `os.UserHomeDir`, `Env` from `os.Environ`, `Now` with `time.Now`, an HTTP client with a 30 second timeout, the macOS keychain (a not-found store elsewhere), the process table (`/proc` on Linux, `ps` on macOS) and an `os/exec` runner.

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

Build it once at program start and pass it down; every agent constructor (`claude.New`, `codex.New`, `builtin.New`, `agentio.FromSpec`) takes it.

## Step 2 — Build a Host by hand

`Host` is a plain struct, so tests and examples fill it themselves. Each field is one input:

| Field | Type | Used for | nil / empty means |
|---|---|---|---|
| `UserHome` | `string` | where default homes live (`~/.claude`, `~/.codex`) | relative paths resolve against "" |
| `User` | `string` | the OS login name; Claude keychain lookups name it as the account | service-only lookup |
| `Env` | `[]string` (`K=V`) | `PATH` lookups, the home variable, the base env of every built `Command` | empty child env, nothing found on PATH |
| `Now` | `func() time.Time` | expiry checks, backup names, `Retry-After` dates | zero time |
| `HTTP` | `*http.Client` | usage, subscription and probe calls | differs per vendor, see Pitfalls |
| `Secrets` | `SecretStore` | keychain / keyring reads (`Get(ctx, service, account)`) | no secret is ever found |
| `Processes` | `ProcessLister` | live sessions, process classification (`Processes`, `Cwd`) | no process is alive |
| `Exec` | `ProcessRunner` | running the vendor binary, only from Runner doors (`Run` returns a `ProcessResult`) | Runner calls return an error |

A minimal fixture Host needs only a home and an env:

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

Read the Host's environment with `Host.Getenv`, or any `K=V` slice with `agentio.EnvLookup`. Both return the last entry of a name, which is the one a child process sees, and tell an empty value apart from an absent one:

<!-- snippet: examples/quickstart/main.go#host-getenv -->
```go
// hostEnv reads the Host's base env, never the process env. Getenv and
// EnvLookup return the LAST entry of a name (what a child process sees)
// and tell "K=" (present, empty) apart from absent. Pure.
func hostEnv(p printer, h agentio.Host) {
	_, hasPath := h.Getenv("PATH")
	p.line(fmt.Sprintf("host: user=%s PATH set=%t", h.User, hasPath))
	env := []string{"MODE=a", "MODE=b", "EMPTY="}
	mode, _ := agentio.EnvLookup(env, "MODE")
	empty, emptySet := agentio.EnvLookup(env, "EMPTY")
	_, missingSet := agentio.EnvLookup(env, "MISSING")
	p.line(fmt.Sprintf("host: MODE=%s EMPTY=%q set=%t MISSING set=%t", mode, empty, emptySet, missingSet))
}
```

The three interfaces are small enough to implement yourself (for example over a secrets manager), but for tests the `agenttest` package already has fakes for each one (`FakeSecrets`, `FakeProcesses`, `FakeExec`) and `agenttest.NewHost` builds a fixture Host whose network and exec fail loudly. See [agenttest fakes](../testing/agenttest.md).

`ProcessRunner.Run` reports a non-zero exit in `ProcessResult.ExitCode` with a nil error; its error is only for "could not run at all" (missing binary, cancelled context). `SecretStore.Get` returns `found == false` with a nil error when there is no entry.

## Step 3 — Find a binary on PATH

agentio finds a binary by walking the `PATH` value itself. It never asks a shell, because a shell may answer with an alias or a function wrapper instead of a file. `LookPath` returns the first regular executable file, `LookPathAll` every hit in order (the first is what runs, the rest are shadowed copies):

<!-- snippet: examples/quickstart/main.go#find-binary -->
```go
// findBinary walks a PATH value the way Agent.LookPath does: the first hit
// is what runs, later hits are shadowed copies. Disk · Read.
func findBinary(p printer, pathList string) {
	if first, err := agentio.LookPath(pathList, claude.Binary); err == nil {
		p.line(fmt.Sprintf("lookpath: first=%s", first))
	}
	for i, hit := range agentio.LookPathAll(pathList, claude.Binary) {
		p.line(fmt.Sprintf("lookpath: hit[%d]=%s", i, hit))
	}
}
```

Every agent has `Agent.LookPath()`, which does the same with its own binary name and `Host.Env`'s `PATH`. A miss is an error matching `agentio.ErrNotInstalled`:

<!-- snippet: examples/quickstart/main.go#not-installed -->
```go
// notInstalled shows the answer for a machine without claude: Install and
// LookPath return an error matching agentio.ErrNotInstalled. Disk · Read.
func notInstalled(p printer, userHome string) error {
	a := claude.New(fixtureHost(userHome, ""))
	_, err := a.LookPath()
	if !errors.Is(err, agentio.ErrNotInstalled) {
		return fmt.Errorf("empty PATH: got %v, want ErrNotInstalled", err)
	}
	p.line(fmt.Sprintf("empty PATH: %v", err))
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

`go run ./quickstart -os` prints the same report from `HostFromOS` for your own machine.

## Pitfalls

- **Only `HostFromOS` reads the real machine.** If a test sees your real homes, something called it; build the Host by hand or with `agenttest.NewHost`.
- **PATH lookups skip empty and relative entries.** An empty `PATH` element means "current dir" to a shell; agentio never runs a binary from the current dir, so `PATH=:/usr/bin` only searches `/usr/bin`.
- **An absolute name is checked as is.** `LookPath(path, "/opt/claude/bin/claude")` ignores `PATH` and only checks that file.
- **A file must be executable to count.** A non-executable `claude` file earlier on PATH is skipped, the same as the shell does.
- **Duplicate env entries: the last wins.** `EnvLookup` and `Getenv` return the last `K=` entry; `ApplyEnv` removes every duplicate when it changes a name ([Targets and commands](launch.md#step-4--change-the-child-environment)).
- **`Exec` is only reached through a Runner door.** Building a launch command never runs anything; holding a Runner is the explicit opt-in to process calls ([Doors](../capabilities/doors.md)).
- **A nil `HTTP` means "no network".** Every vendor network call (Claude usage, profile and probe, Codex usage) returns `ErrUnsupported` with the reason `Host.HTTP is nil`; the library never builds a client of its own, `HostFromOS` is the one place that does. In a test, prefer `agenttest.FailingHTTP` so an unexpected request fails loudly instead of returning a quiet `ErrUnsupported`.
- **On macOS the process table comes from `ps`.** Arguments are split on spaces, so an argument that contains a space shows up as two.

## Related

- [Your first agent](../getting-started/first-agent.md)
- [Agent, Spec and Core](agent-and-spec.md)
- [agenttest fakes](../testing/agenttest.md)
- [Testing your own code](../testing/your-code.md)
- Example: [`examples/quickstart`](../../examples/quickstart/)
- [Feature index](../reference/features.md)
