# Host: the only door to the machine

A `Host` is the machine agentio looks at: the user home, the base environment, the clock, an HTTP client, the keychain, the process table and a way to run a program. No function in the library reads `$HOME`, [`os.Environ`](ds:block?id=host-4pbbeq6s), the keychain, the clock or the network except through the Host you pass in. That is what lets a test point every input at a fixture, and lets one program inspect two machines' worth of homes side by side.

## Before you start

- Module: `github.com/ubgo/agentio` (stdlib only).
- Nothing on disk is required to build a Host; `HostFromOS` [only resolves the home dir](ds:block?id=hostfromos-yvmqkxwe).

## Step 1 — Build a Host from the OS

[`agentio.HostFromOS`](ds:block?id=hostfromos-yvmqkxwe) is the one constructor that touches the real machine. It fills `UserHome` from `os.UserHomeDir`, `Env` from `os.Environ`, `Now` with `time.Now`, an HTTP client with a [30 second timeout](ds:block?id=oshttptimeout-3nmmj9j5), the OS secret store (the macOS keychain through `security`, the Linux Secret Service through `secret-tool`, and on Windows a store whose `Get` returns `ErrUnsupported`), the process table [(`/proc` on Linux, `ps` on macOS, PowerShell `Win32_Process` on Windows)](ds:block?id=osprocesses-processes-q5hpmdr9), an `os/exec` runner, `GOOS` from `runtime.GOOS` and `SystemRoot` (`/` on unix, the system drive root on Windows).

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

Build it once at program start and pass it down; every agent constructor (`claude.New`, `codex.New`, `builtin.New`, [`agentio.FromSpec`](ds:block?id=fromspec-4tcd24bd)) takes it.

## Step 2 — Build a Host by hand

`Host` is a plain struct, so tests and examples fill it themselves. Each field is one input:

| Field | Type | Used for | nil / empty means |
|---|---|---|---|
| `UserHome` | `string` | where default homes live (`~/.claude`, `~/.codex`) | relative paths resolve against "" |
| `User` | `string` | the OS login name; Claude keychain lookups name it as the account | [service-only lookup](ds:block?id=claude-agent-keychainget-uj32ktdh) |
| `Env` | `[]string` (`K=V`) | `PATH` lookups, the home variable, the base env of every built `Command` | empty child env, nothing found on PATH |
| `Now` | `func() time.Time` | expiry checks, backup names, `Retry-After` dates | [zero time](ds:block?id=claude-agent-now-xsb7g62h) |
| `HTTP` | `*http.Client` | usage, subscription and probe calls | [every network call returns `ErrUnsupported`](ds:block?id=claude-agent-httpclient-e4beyzpg) |
| `Secrets` | `SecretStore` | keychain / keyring reads (`Get(ctx, service, account)`) | [no secret is ever found](ds:block?id=claude-agent-keychainget-uj32ktdh) |
| `Processes` | `ProcessLister` | live sessions, process classification and process sessions (`Processes`, `Cwd`; optionally `Env`) | no process is alive |
| `Exec` | `ProcessRunner` | running the vendor binary, only from Runner doors (`Run` returns a `ProcessResult`) | [Runner calls return an error](ds:block?id=runner-run-5rn5nerj) |
| `GOOS` | `string` | which OS's rules apply (`linux`, `darwin`, `windows`): system and managed locations, `PATH`/`PATHEXT` lookup, env name case, which credential stores a vendor reads | [no system locations], and the `PATH` and env rules of the OS the program was built for(ds:block?id=host-systempath-7azvt62d) |
| `SystemRoot` | `string` | the root system and managed paths resolve under (`/etc/claude-code`, `/etc/codex`); [`Host.SystemPath(rel)`](ds:block?id=host-systempath-7azvt62d) joins it | no system locations: managed settings, managed `CLAUDE.md` and MCP files and `/etc/codex` layers are never read |

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

Read the Host's environment with `Host.Getenv`, or any `K=V` slice with [`agentio.EnvLookup`](ds:block?id=envlookup-ef3y8bwk). Both return the last entry of a name, which is the one a child process sees, and tell an empty value apart from an absent one:

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

The three interfaces are small enough to implement yourself (for example over a secrets manager), but for tests the `agenttest` package already has fakes for each one (`FakeSecrets`, `FakeProcesses`, `FakeExec`) and [`agenttest.NewHost`](ds:block?id=agenttest-newhost-ep2mcqr8) builds a fixture Host whose network and exec fail loudly. See [agenttest fakes](../testing/agenttest.md).

One `Processes` call is one read of the whole table. Each `agentio.Process` row carries `PID`, `PPID`, `Start` (the pid-reuse guard), `Exe`, `Argv`, the unsplit `CommandLine`, `RSS` in bytes, the controlling terminal `TTY` without `/dev/` and the `CPUTime` used so far; a zero value means unknown. `Process.CommandString()` is the raw line, or `Argv` joined when the table gave none. `Process.Cwd` is empty from the table (a cwd costs a run per process on macOS); set it when you looked the cwd up yourself and readers reuse it. A lister may also implement `ProcessEnvReader` (one pid's start environment) and `ProcessEnvBatchReader` (many pids in one read); `HostFromOS` implements both on Linux and macOS. `Host.ProcessesAlive` re-checks many recorded pids against one table read. What each OS fills, and how a row is joined to a conversation, is in [Process sessions and stop plans](../capabilities/process-sessions.md).

[`ProcessRunner.Run`](ds:block?id=osrunner-run-hm7czx3q) reports a non-zero exit in `ProcessResult.ExitCode` with a nil error; its error is only for "could not run at all" (missing binary, cancelled context). [`SecretStore.Get`](ds:block?id=ossecrets-get-ux5ua2mu) returns `found == false` with a nil error when there is no entry.

## Step 3 — Find a binary on PATH

agentio finds a binary by walking the `PATH` value itself. It never asks a shell, because a shell may answer with an alias or a function wrapper instead of a file. [`LookPath`](ds:block?id=lookpath-f56nfyzk) returns the first runnable file under the running OS's rules (a regular file with an exec bit; on Windows any regular file, with the `PATHEXT` extensions tried for a bare name), [`LookPathAll`](ds:block?id=lookpathall-u48etgc3) every hit in order (the first is what runs, the rest are shadowed copies):

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

The free functions use the separator of the OS the program was built for. To follow the Host instead, use [`Host.LookPath`](ds:block?id=host-lookpath-4k46e3ag) and `Host.LookPathAll`, which read `PATH` and `PATHEXT` from `Host.Env` and apply `Host.GOOS`'s rules: on Windows `;` separates entries, a bare name is tried with every `PATHEXT` extension (`codex.cmd`, `claude.exe`) and no exec bit is needed. `LookPathAllFor(goos, pathList, pathExt, name)` is the same with the OS as an argument. Every agent's `LookPath` and `Install` use the Host form ([Platforms](../platforms.md)).

Every agent has [`Agent.LookPath()`](ds:block?id=core-lookpath-ke3ntet7), which does the same with its own binary name and `Host.Env`'s `PATH`. A miss is an error matching `agentio.ErrNotInstalled`:

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

- **Only [`HostFromOS`](ds:block?id=hostfromos-yvmqkxwe) reads the real machine.** If a test sees your real homes, something called it; build the Host by hand or with `agenttest.NewHost`.
- **PATH lookups [skip empty and relative entries](ds:block?id=lookpathall-u48etgc3).** An empty `PATH` element means "current dir" to a shell; agentio never runs a binary from the current dir, so `PATH=:/usr/bin` only searches `/usr/bin`.
- **[An absolute name is checked as is.](ds:block?id=lookpathall-u48etgc3)** `LookPath(path, "/opt/claude/bin/claude")` ignores `PATH` and only checks that file.
- **[A file must be runnable to count.](ds:block?id=isexecutable-395kvm4u)** A non-executable `claude` file earlier on PATH is skipped, the same as the shell does. Windows has no exec bit, so there any regular file counts, and `codex` finds `codex.cmd` (npm) or `claude.exe` (native installer) through `PATHEXT`.
- **Windows PATH entries may be rooted or quoted.** An entry without a drive (`\tools`) is on the current drive, never relative to the current dir, so it is searched; quotes around an entry (`"C:\Program Files\nodejs"`) are removed first.
- **Env names follow `Host.GOOS`.** On Windows `Host.Getenv("PATH")` also finds `Path=...`, and a Command built for a windows Host removes every spelling of a name it changes (`EnvLookupFor`, `ApplyEnvFor`); elsewhere names are case-sensitive.
- **Duplicate env entries: the last wins.** [`EnvLookup`](ds:block?id=envlookup-ef3y8bwk) and `Getenv` return the last `K=` entry; [`ApplyEnv`](ds:block?id=applyenv-9fc572ee) removes every duplicate when it changes a name ([Targets and commands](launch.md#step-4--change-the-child-environment)).
- **System locations need `GOOS` and `SystemRoot`.** [Only `HostFromOS` fills them](ds:block?id=hostfromos-yvmqkxwe), so a Host you build yourself never reads `/etc/claude-code` or `/etc/codex` and fixture output cannot change with the machine. To test managed files, put them under a temp dir and pass [`agenttest.WithSystem("linux", dir)`](ds:block?id=agenttest-withsystem-dpvygedy).
- **[`Exec` is only reached through a Runner door.](ds:block?id=runner-run-5rn5nerj)** Building a launch command never runs anything; holding a Runner is the explicit opt-in to process calls ([Doors](../capabilities/doors.md)).
- **A nil `HTTP` means "no network".** Every vendor network call (Claude usage, profile and probe, Codex usage) returns `ErrUnsupported` with the reason [`Host.HTTP is nil`](ds:block?id=claude-reasonnohttp-mb8pg2zh); the library never builds a client of its own, `HostFromOS` is the one place that does. In a test, prefer [`agenttest.FailingHTTP`](ds:block?id=agenttest-failinghttp-8vye27y9) so an unexpected request fails loudly instead of returning a quiet `ErrUnsupported`.
- **On macOS the process table comes from `ps`.** [Arguments are split on spaces](ds:block?id=darwinprocesses-h9y8tzsg), so an argument that contains a space shows up as two in `Argv`; `CommandLine` keeps the unsplit line, and process matching tests it.

## Related

- [Your first agent](../getting-started/first-agent.md)
- [Agent, Spec and Core](agent-and-spec.md)
- [agenttest fakes](../testing/agenttest.md)
- [Testing your own code](../testing/your-code.md)
- Example: [`examples/quickstart`](../../examples/quickstart/)
- [Feature index](../reference/features.md)
