<h1 align="center">agentio</h1>
<p align="center"><strong>Typed Go access to Claude Code, OpenAI Codex and other coding-agent CLIs.</strong></p>
<p align="center">Find every install and home, read settings, MCP servers, skills, plugins, logins, plan usage, conversations and live sessions, and build the command that launches or resumes the agent, from one type-safe Go library.</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.26.4%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.26.4 or newer">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License: Apache-2.0"></a>
</p>

agentio is an open-source Go library that tells a program everything it can know about the coding-agent CLIs on a machine: whether **Claude Code** and **OpenAI Codex** are installed and at which version, where their homes (config directories) are, what is in their settings, `CLAUDE.md` / `AGENTS.md` instruction files, MCP servers, skills and plugins, which account is logged in, how much of the plan's usage is left, which conversations exist and which sessions are running right now. It also builds the exact `Command` that launches, resumes or runs an agent headless, with the right environment for each home. It reads local files through typed structs modelled on real vendor files, never runs a vendor binary or touches the network unless you ask, and never refreshes or prints a login token.

## Contents

- [Why agentio?](#why-agentio)
- [Architecture](#architecture)
- [Install](#install)
- [Quick start](#quick-start)
- [What you can read and build](#what-you-can-read-and-build)
- [Platforms](#platforms)
- [Documentation](#documentation)
- [Examples](#examples)
- [FAQ](#faq)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Why agentio?

Every tool that sits next to a coding agent (a launcher, a usage meter, a session switcher, a dashboard, a backup script) ends up re-learning the same vendor facts: where `~/.claude` and `~/.codex` live and how a second home is selected, how Claude Code names its per-folder history directory, which keychain entry holds the login, which file holds cached usage, how a Codex rollout is laid out, which flags exist in which version. Each tool gets a few of them wrong (a hard-coded home path, a token on argv, a whole transcript read into memory) and each breaks separately when the vendor ships a change.

agentio puts those facts in one tested library, with each vendor fact backed by a fixture home copied from a real install:

- **Type-safe all the way down.** Concrete vendor types (`*claude.Agent`, `*codex.Agent`) when you know the agent; one typed accessor per shared capability (`agentio.UsageOf(a)`, `agentio.ConversationsOf(a)`) when you loop over agents. No `any`, no string-keyed features, no type assertions in your code.
- **Every call states its cost.** Pure, disk read, process run, network read or quota-spending call. Running a vendor binary, spending quota and writing sit behind separate doors (`Runner`, `Prober`, `Writer`), so a read-only loop cannot do any of them by accident.
- **Safe with credentials.** Tokens are a redacting [`Token`](ds:block?id=token-3nk7gtub) type that prints `[redacted]` everywhere, never go on argv, and are never refreshed or written. Emails and account ids are redacted too unless you call [`Reveal()`](ds:block?id=personal-reveal-pjvj854b).
- **Read-only by default.** Writes are explicit [`Writer`](ds:block?id=backupfile-bxsxzm67) methods that back the target up first and return what they copied.
- **No hidden state.** Everything (home dir, environment, clock, HTTP, keychain, process table, exec) comes from one [`Host`](ds:block?id=host-4pbbeq6s) value, so tests point it at fixture homes and one process can inspect several homes side by side.
- **Multiple homes are first class.** Default home, `CLAUDE_CONFIG_DIR` / `CODEX_HOME` homes and any extra home directory, with [the vendor's own env rule applied when launching](ds:block?id=homeenvchange-59nx5j7k).
- **Pay only for what you import.** One Go module per vendor: a Claude-only program never downloads Codex's TOML or SQLite dependencies.
- **Version drift is visible.** Each vendor module declares the CLI versions it was tested against; anything outside the range still works and [returns a warning](ds:block?id=compat-warning-yeqfn6xw). A drift report lists unknown keys, enum values and flags on a real machine.
- **Agents as data.** A simple CLI agent (binary, home variable, resume syntax) is an [`agentio.Spec`](ds:block?id=spec-9rwc3qhn) value, no Go code needed.

| | **agentio** | Parsing vendor files yourself | Shelling out to vendor subcommands |
|---|---|---|---|
| Typed structs for settings, MCP, usage, conversations | ✅ | ⚠️ you write and maintain them | ⚠️ only what the CLI prints as JSON |
| Same code loops over Claude Code and Codex | ✅ typed accessors | ❌ | ❌ |
| Never runs a vendor binary unless asked | ✅ | ✅ | ❌ |
| Never refreshes or rewrites a login | ✅ invariant with tests | ⚠️ depends on care | ❌ some subcommands refresh tokens |
| Tokens kept off argv, logs and JSON | ✅ redacting types | ⚠️ depends on care | ⚠️ depends on care |
| Multiple homes per agent | ✅ | ⚠️ often hard-coded to the default | ⚠️ one env var at a time |
| Testable without a real account | ✅ `Host` + fixture homes + fakes | ⚠️ | ❌ |
| Tracks vendor version drift | ✅ tested ranges, warnings, drift report | ❌ | ❌ |

If you only need one value from one file once, reading it yourself is fine. If your program needs to understand several agents, several homes or several vendor versions, and must not leak or break a login, use agentio.

## Architecture

![Architecture](docs/assets/architecture.svg)

The library has three levels:

- **L1 core** (`agentio.Agent`): what every agent has, including one described only by data: identity, homes, binary lookup and building a launch `Command`.
- **L2 shared capabilities**: concepts both vendors share (install, identity, usage, cached usage, subscription, conversations, prompt history, live sessions, turns, instruction files, MCP servers, skills, plugins, config layers, run events, doctor checks), each a small interface with a typed accessor such as `agentio.UsageOf(a)`.
- **L3 vendor API**: everything else, typed per vendor: Claude Code settings, hooks, subagents, transcripts and stream-json events; Codex `config.toml`, rollouts, SQLite threads and exec events.

## Install

agentio is a set of Go modules. Import only the ones you need:

| Module | Import when you need | Extra dependencies |
|---|---|---|
| `github.com/ubgo/agentio` | the neutral core: `Host`, `Agent`, `Command`, `Token`, capability accessors, `agenttest` | none (stdlib only) |
| `github.com/ubgo/agentio/claude` | Claude Code | none |
| `github.com/ubgo/agentio/codex` | Codex: config, rollouts, usage, sessions, launch | `github.com/pelletier/go-toml/v2` |
| `github.com/ubgo/agentio/codex/state` | Codex's SQLite stores (threads, turns, goals, memories, logs), read-only | `modernc.org/sqlite` (pure Go, no CGO) |
| `github.com/ubgo/agentio/builtin` | every built-in agent at once, plus config-defined agents | whatever `claude` and `codex` pull |

```sh
# Claude Code only
go get github.com/ubgo/agentio/claude

# Codex only
go get github.com/ubgo/agentio/codex

# Every built-in agent in one typed set
go get github.com/ubgo/agentio/builtin

# Codex SQLite stores: only if you read threads or logs from SQLite
go get github.com/ubgo/agentio/codex/state
```

`codex/state` is its own module because it pulls a full SQLite driver. Most programs never need it: `codex` already reads Codex's rollouts, config and usage. `builtin` does not import `codex/state` either.

agentio needs Go 1.26.4 or newer and builds with `CGO_ENABLED=0` on linux, macOS and windows (amd64 and arm64).

## Quick start

This program lists every Claude Code and Codex home on the machine, the three newest conversations in each, the cached plan usage, and the command that would start Claude Code in plan mode. It only reads disk; it runs no vendor binary and makes no network call.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/agentio/claude"
)

func main() {
	ctx := context.Background()

	// The only call that touches the real machine.
	host, err := agentio.HostFromOS()
	if err != nil {
		log.Fatal(err)
	}
	agents, err := builtin.New(host) // Claude Code + Codex; nothing read yet
	if err != nil {
		log.Fatal(err)
	}

	// Neutral loop: every agent, every home, capabilities by typed accessor.
	for _, a := range agents.All() {
		homes, err := a.Homes() // disk read
		if err != nil {
			log.Fatal(err)
		}
		for _, h := range homes {
			fmt.Println(a.Name(), h.Dir())
			if conv, ok := agentio.ConversationsOf(a); ok {
				list, err := conv.Conversations(ctx, h, agentio.ConversationQuery{Limit: 3})
				if err != nil {
					log.Fatal(err)
				}
				for _, c := range list {
					fmt.Printf("  %s  %s\n", c.Updated.Format("2006-01-02"), c.Title)
				}
			}
			if cu, ok := agentio.CachedUsageOf(a); ok {
				u, found, err := cu.CachedUsage(h) // disk only, safe in a loop
				if err == nil && found {
					fmt.Printf("  plan usage: %.0f%% (as of %s)\n", u.MaxPercent(), u.ObservedAt.Format("15:04"))
				}
			}
		}
	}

	// Vendor API: code that knows it is Claude Code gets typed options.
	cmd, err := agents.Claude.Launch(agents.Claude.DefaultHome().Target(), claude.LaunchOptions{
		Model:          "opus",
		PermissionMode: claude.ModePlan,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(cmd.Redacted()) // building is pure; you run it
}
```

Run against the repository's fixture homes (two Claude Code homes and one Codex home), it prints:

```text
Claude Code /home/user/.claude
  2026-11-14  Fixture title 185
  2026-11-14  Fixture title 166
  plan usage: 6% (as of 08:38)
Claude Code /home/user/.claude-work
  2026-11-14  Work home fixture
Codex /home/user/.codex
  2026-11-14  List the files
  2026-11-14  hello
  plan usage: 12% (as of 10:00)
claude --model opus --permission-mode plan
```

A Claude-only program skips `builtin` and uses `claude.New(host)` directly; [`c.Neutral()`](ds:block?id=claude-agent-neutral-eh63bcpc) gives the same neutral view for the L2 accessors.

## What you can read and build

| Area | Claude Code | Codex |
|---|---|---|
| Install, version, tested range, binary lookup | ✅ | ✅ |
| Homes: default, env-selected, extra dirs | ✅ | ✅ |
| Settings and config layers | ✅ `settings.json` scopes, hooks, env, policy limits | ✅ `config.toml`, profiles |
| Instruction files | ✅ `CLAUDE.md`, imports, auto memory | ✅ `AGENTS.md` |
| MCP servers, skills, plugins | ✅ | ✅ |
| Account identity and subscription | ✅ (personal values redacted) | ✅ (personal values redacted) |
| Plan usage: cached on disk or live from the vendor | ✅ | ✅ |
| Conversations, prompt history, turns | ✅ transcripts, read with bounded head and tail | ✅ rollouts, SQLite threads via `codex/state` |
| Live sessions and process classification | ✅ | ✅ |
| Launch, resume, headless commands | ✅ typed `LaunchOptions`, stream-json decoder | ✅ typed options, exec event decoder |
| Explicit writes with backup | ✅ settings, hooks, move history and conversations, link homes | ❌ by design: Codex keeps its own stores consistent, so changes go through Codex's own commands |

The full list, with the cost of every call, is in the [feature index](docs/reference/features.md).

## Platforms

Every feature works on each OS below or returns `agentio.ErrUnsupported` with the reason. The rules follow `Host.GOOS`, so the tests prove the Windows rules on Linux too. Details and per-OS pitfalls: [Platforms](docs/platforms.md).

| | macOS | Linux (glibc, musl) | Windows 10/11 |
|---|---|---|---|
| Install, version, homes, settings, MCP, skills, conversations | ✅ | ✅ | ✅ (`PATHEXT`, `.cmd` shims, `%USERPROFILE%`) |
| Claude login | keychain + `.credentials.json` | `.credentials.json` | `.credentials.json` |
| Codex keyring login | keychain | Secret Service (`secret-tool`) | `ErrUnsupported`; `auth.json` works |
| Managed settings | files + plist | `/etc/claude-code`, `/etc/codex` | files + policy registry |
| Process table, live sessions | `ps` | `/proc` (Alpine included) | PowerShell; no process cwd |
| Home links | ✅ | ✅ | needs Developer Mode, else a typed error |
| Tested | owner's Mac | Ubuntu, and Debian, Alpine, Fedora, Arch containers (`task test:distros`) | compiled and fixture-tested; real runs through the manual `platforms` workflow |

## Documentation

- [Learning path](docs/README.md#learning-path): read the docs in order, from the first `Host` to vendor-specific APIs.
- [Choosing guides](docs/choosing/README.md): which module, which level, which door for your program.
- [Feature index](docs/reference/features.md): every capability and method with its cost.
- [All docs](docs/README.md): getting started, core, capabilities, Claude Code, Codex, testing, compatibility.
- [CHANGELOG](CHANGELOG.md): release history per module.

## Examples

Runnable programs live in [`examples/`](examples/), one per area, each with a README and a test that runs it on fixture homes; the full list is in [All examples](docs/README.md#all-examples). Two of them report on your own machine:

- `examples/agentinfo`: a text or `--json` report of every agent and home on the machine: install, compat, homes, paths, settings, MCP servers, instruction files, skills, plugins, identity (redacted), cached usage, recent conversations, live sessions. Disk and process-table reads only unless you pass `--runner` or `--network`.
- `examples/drift`: lists what the library does not know yet about the vendor versions on this machine (unknown keys, enum values, record types, flags), naming files and keys only, never values.

```sh
cd examples
go run ./agentinfo
```

## FAQ

### How do I read Claude Code or Codex settings and config from Go?

Create a `Host` with `agentio.HostFromOS()`, then `claude.New(host)` or `codex.New(host)`. The vendor agent has typed readers for Claude Code's `settings.json` scopes and Codex's `config.toml`, and `agentio.ConfigOf(a)` gives a neutral view of both.

### Does agentio send data anywhere or need an API key?

No. By default it only reads local files and, for live sessions, the process table. Live usage and subscription calls are separate network methods you call explicitly, and the only quota-spending call sits behind the `Prober` door.

### Can it break my Claude Code or Codex login?

It is designed not to. agentio never refreshes, writes or copies an OAuth token, and never runs a vendor binary unless you call a `Runner` method, because some vendor subcommands refresh tokens and rewrite login files. The default home is launched with the home variable [removed, not set](ds:block?id=homeenvchange-59nx5j7k), so the CLI keeps finding its keychain entry.

### How do I get Claude Code or Codex plan usage (5-hour and weekly limits) in Go?

`agentio.CachedUsageOf(a)` reads the usage the CLI last saved on disk, which is free and safe in a loop. `agentio.UsageOf(a)` or `c.Usage(ctx, home)` asks the vendor once over the network; a 429 comes back as [`ErrRateLimited`](ds:block?id=ratelimiterror-pbyjx7ce), so cache the result instead of polling.

### Does it support several Claude Code accounts or config directories?

Yes. [`a.Homes()`](ds:block?id=core-homes-43a2yjev) finds the default home and other home directories on disk, each launch targets one home, and the vendor's env rule (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`) is applied for you.

### How do I list Claude Code conversations or Codex sessions?

`agentio.ConversationsOf(a)` lists conversations per home, [newest first, filtered by folder, time or count](ds:block?id=conversationquery-8hx6hqtd). Claude Code transcripts are never read whole, only a bounded head and tail. For Codex's SQLite threads, import `codex/state`.

### Which Claude Code and Codex versions are supported?

Each vendor module declares the range it was tested against ([`claude.TestedMin`](ds:block?id=claude-testedmin-8f2tgsg2) / `TestedMax`, `codex.TestedMin` / `TestedMax`) with a fixture home per version. Other versions still work: unknown keys are kept in typed `Unknown` fields and the result carries a warning instead of failing.

### Can I add an agent other than Claude Code and Codex?

A simple CLI agent can be described as data with an `agentio.Spec` and passed to [`builtin.New(host, spec)`](ds:block?id=builtin-new-55hg377w); it gets homes, binary lookup, launch and doctor checks. Full vendor support (usage, conversations) needs a vendor module, modelled on a real install.

### How do I test code that uses agentio without a real account?

Build a `Host` with [`agenttest.NewHost`](ds:block?id=agenttest-newhost-ep2mcqr8) and point it at a copy of a fixture home, with fake keychain, process table and exec runner, and an `httptest` server for network calls. [`agenttest.Conformance`](ds:block?id=agenttest-conformance-n43t4rrx) checks any `agentio.Agent` against the library's invariants.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) for the multi-module setup, `task check`, and the rules every change keeps (real vendor shapes only, type-safe APIs, no hidden state, fixtures instead of real accounts).

## Security

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md), which also lists the library's guarantees for tokens and other secrets.

<!-- docsync definitions: each id below names the Go declaration a sentence in README.md or docs/ cites with a ds:block link; ds check flags the sentence when that code changes. Never put a ds:def in a .go file. -->
<!-- ds:def id=accessnames-aun8gmkk file=enums.go pick=symbol:accessNames owner=@khanakia -->
<!-- ds:def id=agent-476xnza7 file=agent.go pick=symbol:Agent owner=@khanakia -->
<!-- ds:def id=agenttest-conformance-n43t4rrx file=agenttest/conformance.go pick=symbol:Conformance owner=@khanakia -->
<!-- ds:def id=agenttest-failinghttp-8vye27y9 file=agenttest/fake.go pick=symbol:FailingHTTP owner=@khanakia -->
<!-- ds:def id=agenttest-fakeexec-anmuphec file=agenttest/fake.go pick=symbol:FakeExec owner=@khanakia -->
<!-- ds:def id=agenttest-fakeexec-run-cg8fa6zc file=agenttest/fake.go pick=symbol:FakeExec.Run owner=@khanakia -->
<!-- ds:def id=agenttest-newhost-ep2mcqr8 file=agenttest/fake.go pick=symbol:NewHost owner=@khanakia -->
<!-- ds:def id=agenttest-withsystem-dpvygedy file=agenttest/fake.go pick=symbol:WithSystem owner=@khanakia -->
<!-- ds:def id=applyenv-9fc572ee file=command.go pick=symbol:ApplyEnv owner=@khanakia -->
<!-- ds:def id=applyhomelinks-svx7cuvm file=link.go pick=symbol:ApplyHomeLinks owner=@khanakia -->
<!-- ds:def id=applymoveplan-sdj62uyw file=move.go pick=symbol:ApplyMovePlan owner=@khanakia -->
<!-- ds:def id=autonomy-unmarshaltext-ves9gtjm file=enums.go pick=symbol:Autonomy.UnmarshalText owner=@khanakia -->
<!-- ds:def id=autonomynames-upa3mjff file=enums.go pick=symbol:autonomyNames owner=@khanakia -->
<!-- ds:def id=backup-isempty-4q86tar4 file=backup.go pick=symbol:Backup.IsEmpty owner=@khanakia -->
<!-- ds:def id=backup-merge-hg9treax file=backup.go pick=symbol:Backup.Merge owner=@khanakia -->
<!-- ds:def id=backupfile-bxsxzm67 file=backup.go pick=symbol:BackupFile owner=@khanakia -->
<!-- ds:def id=backupmoveitems-g7vzvm4y file=move.go pick=symbol:backupMoveItems owner=@khanakia -->
<!-- ds:def id=backuppath-tfjr5hjr file=backup.go pick=symbol:BackupPath owner=@khanakia -->
<!-- ds:def id=backupstamp-ur2d5sev file=backup.go pick=symbol:backupStamp owner=@khanakia -->
<!-- ds:def id=billingnames-phdn4dkq file=enums.go pick=symbol:billingNames owner=@khanakia -->
<!-- ds:def id=builtin-agents-all-ctdnyzs7 file=builtin/builtin.go pick=symbol:Agents.All owner=@khanakia -->
<!-- ds:def id=builtin-agents-byid-3a82nzqs file=builtin/builtin.go pick=symbol:Agents.ByID owner=@khanakia -->
<!-- ds:def id=builtin-agents-q6nbmbqz file=builtin/builtin.go pick=symbol:Agents owner=@khanakia -->
<!-- ds:def id=builtin-new-55hg377w file=builtin/builtin.go pick=symbol:New owner=@khanakia -->
<!-- ds:def id=capability-cost-u64h92wf file=accessor.go pick=symbol:Capability.Cost owner=@khanakia -->
<!-- ds:def id=capabilitynames-pdt7huwg file=enums.go pick=symbol:capabilityNames owner=@khanakia -->
<!-- ds:def id=capabilityof-t96bhbpq file=accessor.go pick=symbol:capabilityOf owner=@khanakia -->
<!-- ds:def id=capabilitytable-brzshzmm file=accessor.go pick=symbol:capabilityTable owner=@khanakia -->
<!-- ds:def id=checkautonomy-udeuy75m file=core.go pick=symbol:CheckAutonomy owner=@khanakia -->
<!-- ds:def id=checkidvalues-rw58qvqg file=capability.go pick=symbol:CheckIDValues owner=@khanakia -->
<!-- ds:def id=checklinksowner-czbw4f2d file=link.go pick=symbol:CheckLinksOwner owner=@khanakia -->
<!-- ds:def id=checkowner-2237brr8 file=home.go pick=symbol:CheckOwner owner=@khanakia -->
<!-- ds:def id=checkstatusnames-r66q6t8d file=enums.go pick=symbol:checkStatusNames owner=@khanakia -->
<!-- ds:def id=classifyprocess-cddjj5sy file=accessor.go pick=symbol:ClassifyProcess owner=@khanakia -->
<!-- ds:def id=claude-agent-account-fy6cq5vn file=claude/globalconfig.go pick=symbol:Agent.Account owner=@khanakia -->
<!-- ds:def id=claude-agent-cachedusage-h2vr9pps file=claude/usage.go pick=symbol:Agent.CachedUsage owner=@khanakia -->
<!-- ds:def id=claude-agent-cachedusagebody-xz4k7r3g file=claude/globalconfig.go pick=symbol:Agent.CachedUsageBody owner=@khanakia -->
<!-- ds:def id=claude-agent-changelog-uybujjbm file=claude/local.go pick=symbol:Agent.Changelog owner=@khanakia -->
<!-- ds:def id=claude-agent-credentialcontext-tcjbfq3k file=claude/credential.go pick=symbol:Agent.CredentialContext owner=@khanakia -->
<!-- ds:def id=claude-agent-defaulthome-amhusqtv file=claude/agent.go pick=symbol:Agent.DefaultHome owner=@khanakia -->
<!-- ds:def id=claude-agent-detecttokenkind-8826wbv6 file=claude/credential.go pick=symbol:Agent.DetectTokenKind owner=@khanakia -->
<!-- ds:def id=claude-agent-doctor-v2kafdh7 file=claude/doctor.go pick=symbol:Agent.Doctor owner=@khanakia -->
<!-- ds:def id=claude-agent-get-8w45njgb file=claude/usage.go pick=symbol:Agent.get owner=@khanakia -->
<!-- ds:def id=claude-agent-globalconfigpath-nne5tjpb file=claude/globalconfig.go pick=symbol:Agent.globalConfigPath owner=@khanakia -->
<!-- ds:def id=claude-agent-historydir-em755axk file=claude/paths.go pick=symbol:Agent.HistoryDir owner=@khanakia -->
<!-- ds:def id=claude-agent-homemarkersin-z86h282k file=claude/agent.go pick=symbol:Agent.HomeMarkersIn owner=@khanakia -->
<!-- ds:def id=claude-agent-homeof-qtt8m9qe file=claude/agent.go pick=symbol:Agent.HomeOf owner=@khanakia -->
<!-- ds:def id=claude-agent-httpclient-e4beyzpg file=claude/usage.go pick=symbol:Agent.httpClient owner=@khanakia -->
<!-- ds:def id=claude-agent-identity-5vqz5ezp file=claude/credential.go pick=symbol:Agent.Identity owner=@khanakia -->
<!-- ds:def id=claude-agent-install-472s4jn6 file=claude/install.go pick=symbol:Agent.Install owner=@khanakia -->
<!-- ds:def id=claude-agent-installcommand-a9f2e6k8 file=claude/launch.go pick=symbol:Agent.InstallCommand owner=@khanakia -->
<!-- ds:def id=claude-agent-keychainaccount-hv9hcdts file=claude/credential.go pick=symbol:Agent.keychainAccount owner=@khanakia -->
<!-- ds:def id=claude-agent-keychainget-uj32ktdh file=claude/credential.go pick=symbol:Agent.keychainGet owner=@khanakia -->
<!-- ds:def id=claude-agent-lastupdate-yzagpxbk file=claude/install.go pick=symbol:Agent.LastUpdate owner=@khanakia -->
<!-- ds:def id=claude-agent-launch-b9s8bctp file=claude/launch.go pick=symbol:Agent.launch owner=@khanakia -->
<!-- ds:def id=claude-agent-launch-r7fsfwtp file=claude/launch.go pick=symbol:Agent.Launch owner=@khanakia -->
<!-- ds:def id=claude-agent-liveblockers-apwxxpeb file=claude/conversations.go pick=symbol:Agent.liveBlockers owner=@khanakia -->
<!-- ds:def id=claude-agent-logincommand-dnxkn4ar file=claude/launch.go pick=symbol:Agent.LoginCommand owner=@khanakia -->
<!-- ds:def id=claude-agent-mcpaddcommand-p8k42k84 file=claude/launch.go pick=symbol:Agent.MCPAddCommand owner=@khanakia -->
<!-- ds:def id=claude-agent-neutral-eh63bcpc file=claude/neutral.go pick=symbol:Agent.Neutral owner=@khanakia -->
<!-- ds:def id=claude-agent-now-xsb7g62h file=claude/usage.go pick=symbol:Agent.now owner=@khanakia -->
<!-- ds:def id=claude-agent-paths-hkyu9f59 file=claude/paths.go pick=symbol:Agent.Paths owner=@khanakia -->
<!-- ds:def id=claude-agent-planmove-jj5mdn26 file=claude/conversations.go pick=symbol:Agent.PlanMove owner=@khanakia -->
<!-- ds:def id=claude-agent-prober-cgcpxdhn file=claude/credential.go pick=symbol:Agent.Prober owner=@khanakia -->
<!-- ds:def id=claude-agent-processtable-c26mufrf file=claude/sessions.go pick=symbol:Agent.processTable owner=@khanakia -->
<!-- ds:def id=claude-agent-project-bjb4pacx file=claude/agent.go pick=symbol:Agent.Project owner=@khanakia -->
<!-- ds:def id=claude-agent-projectpaths-kqyfxj5m file=claude/paths.go pick=symbol:Agent.ProjectPaths owner=@khanakia -->
<!-- ds:def id=claude-agent-purgecommand-379a3tjz file=claude/launch.go pick=symbol:Agent.PurgeCommand owner=@khanakia -->
<!-- ds:def id=claude-agent-runner-8mqvmpgg file=claude/runner.go pick=symbol:Agent.Runner owner=@khanakia -->
<!-- ds:def id=claude-agent-settingspath-g6smumdx file=claude/settings.go pick=symbol:Agent.settingsPath owner=@khanakia -->
<!-- ds:def id=claude-agent-setuptokencommand-xces3g4z file=claude/launch.go pick=symbol:Agent.SetupTokenCommand owner=@khanakia -->
<!-- ds:def id=claude-agent-subscription-rtkahzpe file=claude/usage.go pick=symbol:Agent.Subscription owner=@khanakia -->
<!-- ds:def id=claude-agent-systempaths-gu2kdp23 file=claude/paths.go pick=symbol:Agent.SystemPaths owner=@khanakia -->
<!-- ds:def id=claude-agent-targetfor-c8cb3m3m file=claude/launch.go pick=symbol:Agent.TargetFor owner=@khanakia -->
<!-- ds:def id=claude-agent-tokenenv-tatsnfw8 file=claude/credential.go pick=symbol:Agent.TokenEnv owner=@khanakia -->
<!-- ds:def id=claude-agent-tokenkinds-rgwudtpt file=claude/credential.go pick=symbol:Agent.TokenKinds owner=@khanakia -->
<!-- ds:def id=claude-agent-tokenlifetime-x3rp4mqq file=claude/credential.go pick=symbol:Agent.TokenLifetime owner=@khanakia -->
<!-- ds:def id=claude-agent-updatecommand-c272nqjs file=claude/launch.go pick=symbol:Agent.UpdateCommand owner=@khanakia -->
<!-- ds:def id=claude-agent-usablecredential-e8k4sjep file=claude/usage.go pick=symbol:Agent.usableCredential owner=@khanakia -->
<!-- ds:def id=claude-agent-usage-zq8vtbvb file=claude/usage.go pick=symbol:Agent.Usage owner=@khanakia -->
<!-- ds:def id=claude-agent-withbaseurl-gud8dj5z file=claude/agent.go pick=symbol:Agent.WithBaseURL owner=@khanakia -->
<!-- ds:def id=claude-agent-writer-bwuq47c6 file=claude/writer.go pick=symbol:Agent.Writer owner=@khanakia -->
<!-- ds:def id=claude-checkfeature-am2yhjzp file=claude/features.go pick=symbol:checkFeature owner=@khanakia -->
<!-- ds:def id=claude-compat-ngsh97c9 file=claude/features.go pick=symbol:Compat owner=@khanakia -->
<!-- ds:def id=claude-conversation-neutral-3uhu3gfe file=claude/conversations.go pick=symbol:Conversation.Neutral owner=@khanakia -->
<!-- ds:def id=claude-credential-expired-k3dc67ku file=claude/credential.go pick=symbol:Credential.Expired owner=@khanakia -->
<!-- ds:def id=claude-credentialenv-kqm45qxy file=claude/agent.go pick=symbol:credentialEnv owner=@khanakia -->
<!-- ds:def id=claude-encodepath-cy277s2h file=claude/paths.go pick=symbol:EncodePath owner=@khanakia -->
<!-- ds:def id=claude-errambiguousid-t68armxm file=claude/conversations.go pick=symbol:ErrAmbiguousID owner=@khanakia -->
<!-- ds:def id=claude-errhistoryexists-23j5925s file=claude/writer.go pick=symbol:ErrHistoryExists owner=@khanakia -->
<!-- ds:def id=claude-extrausageof-g9ejxj4p file=claude/usage.go pick=symbol:ExtraUsageOf owner=@khanakia -->
<!-- ds:def id=claude-feature-string-4gkzaspt file=claude/features.go pick=symbol:Feature.String owner=@khanakia -->
<!-- ds:def id=claude-featureinfo-7cywwf7k file=claude/features_gen.go pick=symbol:FeatureInfo owner=@khanakia -->
<!-- ds:def id=claude-features-ej42vjsy file=claude/features.go pick=symbol:Features owner=@khanakia -->
<!-- ds:def id=claude-featuresource-fcy46dbg file=claude/features.go pick=symbol:FeatureSource owner=@khanakia -->
<!-- ds:def id=claude-featureunavailableerror-c76rq36g file=claude/features.go pick=symbol:FeatureUnavailableError owner=@khanakia -->
<!-- ds:def id=claude-featureunavailableerror-error-duumjrvf file=claude/features.go pick=symbol:FeatureUnavailableError.Error owner=@khanakia -->
<!-- ds:def id=claude-featureunavailableerror-is-anzr93zz file=claude/features.go pick=symbol:FeatureUnavailableError.Is owner=@khanakia -->
<!-- ds:def id=claude-homemarkers-8jbyz7hq file=claude/agent.go pick=symbol:HomeMarkers owner=@khanakia -->
<!-- ds:def id=claude-homemarkers-hvhwcdt5 file=claude/agent.go pick=symbol:homeMarkers owner=@khanakia -->
<!-- ds:def id=claude-identity-neutral-t8ajmes2 file=claude/credential.go pick=symbol:Identity.Neutral owner=@khanakia -->
<!-- ds:def id=claude-installnpmglobal-rnn85kq2 file=claude/install.go pick=symbol:InstallNPMGlobal owner=@khanakia -->
<!-- ds:def id=claude-keychainservices-vae8mc8e file=claude/credential.go pick=symbol:KeychainServices owner=@khanakia -->
<!-- ds:def id=claude-launchfromrequest-tr947xnr file=claude/launch.go pick=symbol:launchFromRequest owner=@khanakia -->
<!-- ds:def id=claude-launchoptions-5jyjfngt file=claude/launch.go pick=symbol:LaunchOptions owner=@khanakia -->
<!-- ds:def id=claude-launchoptions-args-t9vscnsz file=claude/launch.go pick=symbol:LaunchOptions.Args owner=@khanakia -->
<!-- ds:def id=claude-launchoptions-build-78wzjpre file=claude/launch.go pick=symbol:LaunchOptions.build owner=@khanakia -->
<!-- ds:def id=claude-launchoptions-validate-tgdhh8yc file=claude/launch.go pick=symbol:LaunchOptions.validate owner=@khanakia -->
<!-- ds:def id=claude-legacylimits-htdx64yk file=claude/usage.go pick=symbol:legacyLimits owner=@khanakia -->
<!-- ds:def id=claude-limit-label-dr46cv8f file=claude/usage.go pick=symbol:Limit.Label owner=@khanakia -->
<!-- ds:def id=claude-livesession-neutral-enj93y59 file=claude/sessions.go pick=symbol:LiveSession.Neutral owner=@khanakia -->
<!-- ds:def id=claude-livesessionerror-unwrap-parprkp9 file=claude/conversations.go pick=symbol:LiveSessionError.Unwrap owner=@khanakia -->
<!-- ds:def id=claude-methodandversion-8h4tytjj file=claude/install.go pick=symbol:methodAndVersion owner=@khanakia -->
<!-- ds:def id=claude-neutral-launch-q63vggs3 file=claude/neutral.go pick=symbol:neutral.Launch owner=@khanakia -->
<!-- ds:def id=claude-neutral-tokenusage-m94wnsb5 file=claude/neutral.go pick=symbol:neutral.TokenUsage owner=@khanakia -->
<!-- ds:def id=claude-neutral-unsupportedreason-t6rggqu7 file=claude/neutral.go pick=symbol:neutral.UnsupportedReason owner=@khanakia -->
<!-- ds:def id=claude-new-2b7v6ezn file=claude/agent.go pick=symbol:New owner=@khanakia -->
<!-- ds:def id=claude-oauthaccount-jqh4rx7y file=claude/globalconfig.go pick=symbol:OAuthAccount owner=@khanakia -->
<!-- ds:def id=claude-parseauthstatus-3kuerg58 file=claude/runner.go pick=symbol:ParseAuthStatus owner=@khanakia -->
<!-- ds:def id=claude-parseprobeheaders-63ksqmwv file=claude/usage.go pick=symbol:ParseProbeHeaders owner=@khanakia -->
<!-- ds:def id=claude-parseprofile-eg8cknwb file=claude/usage.go pick=symbol:ParseProfile owner=@khanakia -->
<!-- ds:def id=claude-parseusage-zn4jj9gg file=claude/usage.go pick=symbol:ParseUsage owner=@khanakia -->
<!-- ds:def id=claude-parseversion-n4kvb682 file=claude/install.go pick=symbol:ParseVersion owner=@khanakia -->
<!-- ds:def id=claude-planlabel-8fuqam6b file=claude/usage.go pick=symbol:PlanLabel owner=@khanakia -->
<!-- ds:def id=claude-prober-door-bbbbbbbb file=claude/credential.go pick=symbol:Prober owner=@khanakia -->
<!-- ds:def id=claude-quota-door-tokenusage-j2fw66uu file=claude/credential.go pick=symbol:Prober.TokenUsage owner=@khanakia -->
<!-- ds:def id=claude-project-localroot-rkts9std file=claude/agent.go pick=symbol:Agent.localRoot owner=@khanakia -->
<!-- ds:def id=claude-ratelimitwindow-zmkhyjn8 file=claude/stream.go pick=symbol:RateLimitWindow owner=@khanakia -->
<!-- ds:def id=claude-readrawsettings-peagek4s file=claude/writer.go pick=symbol:readRawSettings owner=@khanakia -->
<!-- ds:def id=claude-reasonnohttp-mb8pg2zh file=claude/usage.go pick=symbol:reasonNoHTTP owner=@khanakia -->
<!-- ds:def id=claude-redactin-wfbs8jc7 file=claude/usage.go pick=symbol:redactIn owner=@khanakia -->
<!-- ds:def id=claude-runner-authstatus-ktzmqmsf file=claude/runner.go pick=symbol:Runner.AuthStatus owner=@khanakia -->
<!-- ds:def id=claude-runner-automodeconfig-ed2d3mz9 file=claude/runner.go pick=symbol:Runner.AutoModeConfig owner=@khanakia -->
<!-- ds:def id=claude-runner-automodedefaults-8p2z5nv4 file=claude/runner.go pick=symbol:Runner.AutoModeDefaults owner=@khanakia -->
<!-- ds:def id=claude-runner-backgroundagents-2943yj66 file=claude/runner.go pick=symbol:Runner.BackgroundAgents owner=@khanakia -->
<!-- ds:def id=claude-runner-binaryversion-p4863j5t file=claude/runner.go pick=symbol:Runner.BinaryVersion owner=@khanakia -->
<!-- ds:def id=claude-runner-dbm6t8tp file=claude/runner.go pick=symbol:Runner owner=@khanakia -->
<!-- ds:def id=claude-runner-marketplaces-f5xgxq7b file=claude/runner.go pick=symbol:Runner.Marketplaces owner=@khanakia -->
<!-- ds:def id=claude-runner-plugins-5mz2n2aw file=claude/runner.go pick=symbol:Runner.Plugins owner=@khanakia -->
<!-- ds:def id=claude-runner-run-af4j3992 file=claude/runner.go pick=symbol:Runner.run owner=@khanakia -->
<!-- ds:def id=claude-setuptokenlifetime-rwu3ayz3 file=claude/credential.go pick=symbol:SetupTokenLifetime owner=@khanakia -->
<!-- ds:def id=claude-sharedhomefiles-3vpa9mqk file=claude/agent.go pick=symbol:sharedHomeFiles owner=@khanakia -->
<!-- ds:def id=claude-spec-yq7hsrb6 file=claude/agent.go pick=symbol:Spec owner=@khanakia -->
<!-- ds:def id=claude-testedmin-8f2tgsg2 file=claude/features.go pick=symbol:TestedMin owner=@khanakia -->
<!-- ds:def id=claude-usage-sbsaf2c4 file=claude/usage.go pick=symbol:Usage owner=@khanakia -->
<!-- ds:def id=claude-versionwarnings-82bmgcbc file=claude/launch.go pick=symbol:versionWarnings owner=@khanakia -->
<!-- ds:def id=claude-wirecredentials-fx9rcyeb file=claude/credential.go pick=symbol:wireCredentials owner=@khanakia -->
<!-- ds:def id=claude-writer-addhook-y3s8tkg6 file=claude/writer.go pick=symbol:Writer.AddHook owner=@khanakia -->
<!-- ds:def id=claude-writer-applyhomelinks-39hcpw7p file=claude/writer.go pick=symbol:Writer.ApplyHomeLinks owner=@khanakia -->
<!-- ds:def id=claude-writer-applymove-tpwp98h7 file=claude/writer.go pick=symbol:Writer.ApplyMove owner=@khanakia -->
<!-- ds:def id=claude-writer-applypatch-utnp7z3r file=claude/writer.go pick=symbol:Writer.applyPatch owner=@khanakia -->
<!-- ds:def id=claude-writer-movehistory-3n8zhjz3 file=claude/writer.go pick=symbol:Writer.MoveHistory owner=@khanakia -->
<!-- ds:def id=claude-writer-removehooks-x8p95azs file=claude/writer.go pick=symbol:Writer.RemoveHooks owner=@khanakia -->
<!-- ds:def id=claude-writer-setprojectsetting-9xh95gnh file=claude/writer.go pick=symbol:Writer.SetProjectSetting owner=@khanakia -->
<!-- ds:def id=claude-writer-setusersetting-hfm5v3t7 file=claude/writer.go pick=symbol:Writer.SetUserSetting owner=@khanakia -->
<!-- ds:def id=claude-writer-t5gq26t8 file=claude/writer.go pick=symbol:Writer owner=@khanakia -->
<!-- ds:def id=claude-writer-writerawsettings-6sjvkkgu file=claude/writer.go pick=symbol:Writer.writeRawSettings owner=@khanakia -->
<!-- ds:def id=codex-agent-appserver-784gkx4q file=codex/launch.go pick=symbol:Agent.AppServer owner=@khanakia -->
<!-- ds:def id=codex-agent-archive-yztnawg5 file=codex/launch.go pick=symbol:Agent.Archive owner=@khanakia -->
<!-- ds:def id=codex-agent-cachedusage-8nmze7z6 file=codex/conversations.go pick=symbol:Agent.CachedUsage owner=@khanakia -->
<!-- ds:def id=codex-agent-checks-kj5pjpxm file=codex/doctor.go pick=symbol:Agent.Checks owner=@khanakia -->
<!-- ds:def id=codex-agent-configfilewarnings-z5usj78f file=codex/config.go pick=symbol:Agent.ConfigFileWarnings owner=@khanakia -->
<!-- ds:def id=codex-agent-configlayers-9expgqan file=codex/effective.go pick=symbol:Agent.ConfigLayers owner=@khanakia -->
<!-- ds:def id=codex-agent-conversations-gnmdnpjc file=codex/conversations.go pick=symbol:Agent.Conversations owner=@khanakia -->
<!-- ds:def id=codex-agent-credentialstore-7q7gydxk file=codex/auth.go pick=symbol:Agent.credentialStore owner=@khanakia -->
<!-- ds:def id=codex-agent-daemonsocket-5k6q3q87 file=codex/process.go pick=symbol:Agent.DaemonSocket owner=@khanakia -->
<!-- ds:def id=codex-agent-defaulthome-54qemjw6 file=codex/codex.go pick=symbol:Agent.DefaultHome owner=@khanakia -->
<!-- ds:def id=codex-agent-delete-3kpvc6vh file=codex/launch.go pick=symbol:Agent.Delete owner=@khanakia -->
<!-- ds:def id=codex-agent-doctorcommand-49nfu5hm file=codex/launch.go pick=symbol:Agent.DoctorCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-effective-zsyuwnnc file=codex/effective.go pick=symbol:Agent.Effective owner=@khanakia -->
<!-- ds:def id=codex-agent-exec-k7qxsu6g file=codex/launch.go pick=symbol:Agent.Exec owner=@khanakia -->
<!-- ds:def id=codex-agent-execresume-n3xtc4vz file=codex/launch.go pick=symbol:Agent.ExecResume owner=@khanakia -->
<!-- ds:def id=codex-agent-featurecommand-w47qatyu file=codex/launch.go pick=symbol:Agent.FeatureCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-finish-xtvrjgys file=codex/launch.go pick=symbol:Agent.finish owner=@khanakia -->
<!-- ds:def id=codex-agent-fork-rtfwu5vc file=codex/launch.go pick=symbol:Agent.Fork owner=@khanakia -->
<!-- ds:def id=codex-agent-homeat-5kpz3t54 file=codex/codex.go pick=symbol:Agent.HomeAt owner=@khanakia -->
<!-- ds:def id=codex-agent-homeenv-rtwbj8nt file=codex/codex.go pick=symbol:Agent.HomeEnv owner=@khanakia -->
<!-- ds:def id=codex-agent-homefromenv-hgwbx2cn file=codex/codex.go pick=symbol:Agent.HomeFromEnv owner=@khanakia -->
<!-- ds:def id=codex-agent-homeof-xjk5xqdc file=codex/codex.go pick=symbol:Agent.HomeOf owner=@khanakia -->
<!-- ds:def id=codex-agent-homes-nffucnw6 file=codex/codex.go pick=symbol:Agent.Homes owner=@khanakia -->
<!-- ds:def id=codex-agent-identity-j4hp7h58 file=codex/auth.go pick=symbol:Agent.Identity owner=@khanakia -->
<!-- ds:def id=codex-agent-identitycontext-gdq8smc7 file=codex/auth.go pick=symbol:Agent.IdentityContext owner=@khanakia -->
<!-- ds:def id=codex-agent-install-7wqjtcqk file=codex/install.go pick=symbol:Agent.Install owner=@khanakia -->
<!-- ds:def id=codex-agent-installationid-dp47g2yq file=codex/codex.go pick=symbol:Agent.InstallationID owner=@khanakia -->
<!-- ds:def id=codex-agent-inventory-ukxqm5eh file=codex/doctor.go pick=symbol:Agent.Inventory owner=@khanakia -->
<!-- ds:def id=codex-agent-ishome-28msd42y file=codex/codex.go pick=symbol:Agent.IsHome owner=@khanakia -->
<!-- ds:def id=codex-agent-keyringentry-zd3j66a7 file=codex/auth.go pick=symbol:Agent.KeyringEntry owner=@khanakia -->
<!-- ds:def id=codex-agent-layers-yeww4cy8 file=codex/effective.go pick=symbol:Agent.layers owner=@khanakia -->
<!-- ds:def id=codex-agent-loadauth-hsxkaah9 file=codex/auth.go pick=symbol:Agent.loadAuth owner=@khanakia -->
<!-- ds:def id=codex-agent-loadkeyring-2zu65fku file=codex/auth.go pick=symbol:Agent.loadKeyring owner=@khanakia -->
<!-- ds:def id=codex-agent-logincommand-q3tdrhcq file=codex/auth.go pick=symbol:Agent.LoginCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-logoutcommand-gcygwwqe file=codex/auth.go pick=symbol:Agent.LogoutCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-mcpaddcommand-yajah85d file=codex/launch.go pick=symbol:Agent.MCPAddCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-mcpserver-ggfsjmwy file=codex/launch.go pick=symbol:Agent.MCPServer owner=@khanakia -->
<!-- ds:def id=codex-agent-moveunsupportedreason-yn4v8j7h file=codex/conversations.go pick=symbol:Agent.MoveUnsupportedReason owner=@khanakia -->
<!-- ds:def id=codex-agent-neutral-hbg9hqd3 file=codex/neutral.go pick=symbol:Agent.Neutral owner=@khanakia -->
<!-- ds:def id=codex-agent-new-kykg9kgg file=codex/launch.go pick=symbol:Agent.New owner=@khanakia -->
<!-- ds:def id=codex-agent-paths-37cc6b4n file=codex/paths.go pick=symbol:Agent.Paths owner=@khanakia -->
<!-- ds:def id=codex-agent-pathswith-tqqdjxqc file=codex/paths.go pick=symbol:Agent.PathsWith owner=@khanakia -->
<!-- ds:def id=codex-agent-plaincommand-v7yqu9ez file=codex/codex.go pick=symbol:Agent.plainCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-pluginlistcommand-59jvvkz7 file=codex/skills.go pick=symbol:Agent.PluginListCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-profilecheck-eqn24h4u file=codex/doctor.go pick=symbol:Agent.profileCheck owner=@khanakia -->
<!-- ds:def id=codex-agent-profilefiles-uczzc9je file=codex/effective.go pick=symbol:Agent.ProfileFiles owner=@khanakia -->
<!-- ds:def id=codex-agent-profiles-vhcadz8n file=codex/hooks.go pick=symbol:Agent.Profiles owner=@khanakia -->
<!-- ds:def id=codex-agent-queue-9kwzc756 file=codex/launch.go pick=symbol:Agent.Queue owner=@khanakia -->
<!-- ds:def id=codex-agent-resume-s8xtppwf file=codex/launch.go pick=symbol:Agent.Resume owner=@khanakia -->
<!-- ds:def id=codex-agent-resumelast-tm3kcsgb file=codex/launch.go pick=symbol:Agent.ResumeLast owner=@khanakia -->
<!-- ds:def id=codex-agent-review-s6fb5u6d file=codex/launch.go pick=symbol:Agent.Review owner=@khanakia -->
<!-- ds:def id=codex-agent-runner-v8k7es62 file=codex/install.go pick=symbol:Agent.Runner owner=@khanakia -->
<!-- ds:def id=codex-agent-subscription-zk2v5smc file=codex/usage.go pick=symbol:Agent.Subscription owner=@khanakia -->
<!-- ds:def id=codex-agent-systemconfigpaths-bztdnt4n file=codex/effective.go pick=symbol:Agent.systemConfigPaths owner=@khanakia -->
<!-- ds:def id=codex-agent-targetfor-nzpuwqwd file=codex/launch.go pick=symbol:Agent.TargetFor owner=@khanakia -->
<!-- ds:def id=codex-agent-unarchive-yuphtxzv file=codex/launch.go pick=symbol:Agent.Unarchive owner=@khanakia -->
<!-- ds:def id=codex-agent-updatecache-7c64td97 file=codex/install.go pick=symbol:Agent.UpdateCache owner=@khanakia -->
<!-- ds:def id=codex-agent-updatecommand-mmv9gxga file=codex/install.go pick=symbol:Agent.UpdateCommand owner=@khanakia -->
<!-- ds:def id=codex-agent-usage-bnqunb32 file=codex/usage.go pick=symbol:Agent.Usage owner=@khanakia -->
<!-- ds:def id=codex-agent-withbaseurl-exqfdthx file=codex/codex.go pick=symbol:Agent.WithBaseURL owner=@khanakia -->
<!-- ds:def id=codex-applyidclaims-m54n9gf8 file=codex/auth.go pick=symbol:applyIDClaims owner=@khanakia -->
<!-- ds:def id=codex-builder-flag-f2rrxpyh file=codex/launch.go pick=symbol:builder.flag owner=@khanakia -->
<!-- ds:def id=codex-builder-global-82whnmvu file=codex/launch.go pick=symbol:builder.global owner=@khanakia -->
<!-- ds:def id=codex-builder-profile-wmpsnv3m file=codex/launch.go pick=symbol:builder.profile owner=@khanakia -->
<!-- ds:def id=codex-checkfeature-8e67wy9d file=codex/compat.go pick=symbol:checkFeature owner=@khanakia -->
<!-- ds:def id=codex-compat-s6qau3qq file=codex/compat.go pick=symbol:Compat owner=@khanakia -->
<!-- ds:def id=codex-config-4zvr4t68 file=codex/config.go pick=symbol:Config owner=@khanakia -->
<!-- ds:def id=codex-config-featureoverrides-wqauyv6a file=codex/config.go pick=symbol:Config.FeatureOverrides owner=@khanakia -->
<!-- ds:def id=codex-config-legacyprofilenames-hqw2wzbk file=codex/config.go pick=symbol:Config.LegacyProfileNames owner=@khanakia -->
<!-- ds:def id=codex-config-profileconflict-yrme7f58 file=codex/config.go pick=symbol:Config.ProfileConflict owner=@khanakia -->
<!-- ds:def id=codex-config-trustof-bhxj6vhr file=codex/agentsmd.go pick=symbol:Config.TrustOf owner=@khanakia -->
<!-- ds:def id=codex-decodeentriesinto-6fcyfvdp file=codex/config.go pick=symbol:decodeEntriesInto owner=@khanakia -->
<!-- ds:def id=codex-enumopt-sp5njn2z file=codex/tomldecode.go pick=symbol:enumOpt owner=@khanakia -->
<!-- ds:def id=codex-envapikey-mmsb33zd file=codex/codex.go pick=symbol:EnvAPIKey owner=@khanakia -->
<!-- ds:def id=codex-execoptions-b7s5mn6w file=codex/launch.go pick=symbol:ExecOptions owner=@khanakia -->
<!-- ds:def id=codex-execprompt-29smbrfm file=codex/launch.go pick=symbol:execPrompt owner=@khanakia -->
<!-- ds:def id=codex-feature-u2cyacgu file=codex/compat.go pick=symbol:Feature owner=@khanakia -->
<!-- ds:def id=codex-featureflag-daqfcd4n file=codex/install.go pick=symbol:FeatureFlag owner=@khanakia -->
<!-- ds:def id=codex-featureinfo-zr2w6mdt file=codex/features_gen.go pick=symbol:FeatureInfo owner=@khanakia -->
<!-- ds:def id=codex-features-sgjqr7gr file=codex/features_gen.go pick=symbol:Features owner=@khanakia -->
<!-- ds:def id=codex-featureunavailableerror-error-mhyfpqrm file=codex/compat.go pick=symbol:FeatureUnavailableError.Error owner=@khanakia -->
<!-- ds:def id=codex-featureunavailableerror-is-rgu84dcc file=codex/compat.go pick=symbol:FeatureUnavailableError.Is owner=@khanakia -->
<!-- ds:def id=codex-featureunavailableerror-vhup5my8 file=codex/compat.go pick=symbol:FeatureUnavailableError owner=@khanakia -->
<!-- ds:def id=codex-home-hsdrs8z7 file=codex/codex.go pick=symbol:Home owner=@khanakia -->
<!-- ds:def id=codex-home-target-4s8v9g29 file=codex/codex.go pick=symbol:Home.Target owner=@khanakia -->
<!-- ds:def id=codex-identity-neutral-8g73697s file=codex/auth.go pick=symbol:Identity.Neutral owner=@khanakia -->
<!-- ds:def id=codex-keyringaccount-pfgfsmdf file=codex/auth.go pick=symbol:KeyringAccount owner=@khanakia -->
<!-- ds:def id=codex-keyringerror-5yhnmn95 file=codex/auth.go pick=symbol:KeyringError owner=@khanakia -->
<!-- ds:def id=codex-launchoptions-q4zej67x file=codex/launch.go pick=symbol:LaunchOptions owner=@khanakia -->
<!-- ds:def id=codex-mcpserver-f2ettbda file=codex/config.go pick=symbol:MCPServer owner=@khanakia -->
<!-- ds:def id=codex-modelprovider-d7mxgc59 file=codex/config.go pick=symbol:ModelProvider owner=@khanakia -->
<!-- ds:def id=codex-neutral-unsupportedreason-evgu3az7 file=codex/neutral.go pick=symbol:neutral.UnsupportedReason owner=@khanakia -->
<!-- ds:def id=codex-new-cv22qtg5 file=codex/codex.go pick=symbol:New owner=@khanakia -->
<!-- ds:def id=codex-newbuilder-45cx4x37 file=codex/launch.go pick=symbol:newBuilder owner=@khanakia -->
<!-- ds:def id=codex-opt-d24avffs file=codex/tomldecode.go pick=symbol:Opt owner=@khanakia -->
<!-- ds:def id=codex-overridestrings-qyfnckfz file=codex/launch.go pick=symbol:OverrideStrings owner=@khanakia -->
<!-- ds:def id=codex-packageversion-gts9xx4s file=codex/install.go pick=symbol:packageVersion owner=@khanakia -->
<!-- ds:def id=codex-parseauth-4se37c7r file=codex/auth.go pick=symbol:ParseAuth owner=@khanakia -->
<!-- ds:def id=codex-parseconfig-bbrucwhw file=codex/config.go pick=symbol:ParseConfig owner=@khanakia -->
<!-- ds:def id=codex-parsefeaturelist-ptd3dvzf file=codex/install.go pick=symbol:ParseFeatureList owner=@khanakia -->
<!-- ds:def id=codex-parseoverride-csjwphzj file=codex/launch.go pick=symbol:ParseOverride owner=@khanakia -->
<!-- ds:def id=codex-parseplan-btvpuu2q file=codex/auth.go pick=symbol:ParsePlan owner=@khanakia -->
<!-- ds:def id=codex-parseusage-67d5nukv file=codex/usage.go pick=symbol:ParseUsage owner=@khanakia -->
<!-- ds:def id=codex-parseversion-zx9z9rav file=codex/install.go pick=symbol:ParseVersion owner=@khanakia -->
<!-- ds:def id=codex-paths-mgfjercg file=codex/paths.go pick=symbol:Paths owner=@khanakia -->
<!-- ds:def id=codex-paths-profileconfig-k6ajnjam file=codex/paths.go pick=symbol:Paths.ProfileConfig owner=@khanakia -->
<!-- ds:def id=codex-plan-displayname-bfdt6s7g file=codex/auth.go pick=symbol:Plan.DisplayName owner=@khanakia -->
<!-- ds:def id=codex-plan-isworkspace-rf9e4tu9 file=codex/auth.go pick=symbol:Plan.IsWorkspace owner=@khanakia -->
<!-- ds:def id=codex-projectdeniedroots-n2zvt35j file=codex/effective.go pick=symbol:projectDeniedRoots owner=@khanakia -->
<!-- ds:def id=codex-projectroot-645w589u file=codex/agentsmd.go pick=symbol:projectRoot owner=@khanakia -->
<!-- ds:def id=codex-readconfigpath-dfhqgu47 file=codex/config.go pick=symbol:readConfigPath owner=@khanakia -->
<!-- ds:def id=codex-runner-476qyke4 file=codex/install.go pick=symbol:Runner owner=@khanakia -->
<!-- ds:def id=codex-runner-binaryversion-fz7k9npd file=codex/install.go pick=symbol:Runner.BinaryVersion owner=@khanakia -->
<!-- ds:def id=codex-runner-bundledmodels-cpzy9yw7 file=codex/install.go pick=symbol:Runner.BundledModels owner=@khanakia -->
<!-- ds:def id=codex-runner-features-7eabzur3 file=codex/install.go pick=symbol:Runner.Features owner=@khanakia -->
<!-- ds:def id=codex-runner-loginstatus-xg3qprt5 file=codex/install.go pick=symbol:Runner.LoginStatus owner=@khanakia -->
<!-- ds:def id=codex-runner-run-wwh9d3qf file=codex/install.go pick=symbol:Runner.run owner=@khanakia -->
<!-- ds:def id=codex-shellenvironmentpolicy-armg7sww file=codex/config.go pick=symbol:ShellEnvironmentPolicy owner=@khanakia -->
<!-- ds:def id=codex-spec-ut8me6cu file=codex/codex.go pick=symbol:Spec owner=@khanakia -->
<!-- ds:def id=codex-state-clamplimit-hfwegkws file=codex/state/db.go pick=symbol:clampLimit owner=@khanakia -->
<!-- ds:def id=codex-state-db-close-zsgjpsr6 file=codex/state/db.go pick=symbol:DB.Close owner=@khanakia -->
<!-- ds:def id=codex-state-db-conn-hd4czcs6 file=codex/state/db.go pick=symbol:DB.conn owner=@khanakia -->
<!-- ds:def id=codex-state-db-goal-krbpqfcr file=codex/state/stores.go pick=symbol:DB.Goal owner=@khanakia -->
<!-- ds:def id=codex-state-db-items-px499grq file=codex/state/history.go pick=symbol:DB.Items owner=@khanakia -->
<!-- ds:def id=codex-state-db-logs-6duhjxu2 file=codex/state/stores.go pick=symbol:DB.Logs owner=@khanakia -->
<!-- ds:def id=codex-state-db-memories-99nm34aa file=codex/state/stores.go pick=symbol:DB.Memories owner=@khanakia -->
<!-- ds:def id=codex-state-db-memoryjobs-c2g7z8mg file=codex/state/stores.go pick=symbol:DB.MemoryJobs owner=@khanakia -->
<!-- ds:def id=codex-state-db-path-md2jjs9y file=codex/state/db.go pick=symbol:DB.Path owner=@khanakia -->
<!-- ds:def id=codex-state-db-projects-nxrrpgj2 file=codex/state/threads.go pick=symbol:DB.Projects owner=@khanakia -->
<!-- ds:def id=codex-state-db-queue-jeaufcxk file=codex/state/stores.go pick=symbol:DB.Queue owner=@khanakia -->
<!-- ds:def id=codex-state-db-ready-p24c56dj file=codex/state/db.go pick=symbol:DB.ready owner=@khanakia -->
<!-- ds:def id=codex-state-db-schemaversion-rmgwh9hv file=codex/state/db.go pick=symbol:DB.SchemaVersion owner=@khanakia -->
<!-- ds:def id=codex-state-db-subagents-pkqv7bsk file=codex/state/threads.go pick=symbol:DB.SubAgents owner=@khanakia -->
<!-- ds:def id=codex-state-db-thread-ju2qdyus file=codex/state/threads.go pick=symbol:DB.Thread owner=@khanakia -->
<!-- ds:def id=codex-state-db-threads-aukevzn9 file=codex/state/threads.go pick=symbol:DB.Threads owner=@khanakia -->
<!-- ds:def id=codex-state-db-turns-23y7cksp file=codex/state/history.go pick=symbol:DB.Turns owner=@khanakia -->
<!-- ds:def id=codex-state-dsn-8wm86fpy file=codex/state/db.go pick=symbol:dsn owner=@khanakia -->
<!-- ds:def id=codex-state-file-maxknownmigration-wh72f5ht file=codex/state/db.go pick=symbol:File.MaxKnownMigration owner=@khanakia -->
<!-- ds:def id=codex-state-open-uc95e9x7 file=codex/state/db.go pick=symbol:Open owner=@khanakia -->
<!-- ds:def id=codex-state-scanthread-kngeb5w2 file=codex/state/threads.go pick=symbol:scanThread owner=@khanakia -->
<!-- ds:def id=codex-state-thread-conversation-n596sbdw file=codex/state/threads.go pick=symbol:Thread.Conversation owner=@khanakia -->
<!-- ds:def id=codex-state-thread-r6p6ea25 file=codex/state/threads.go pick=symbol:Thread owner=@khanakia -->
<!-- ds:def id=codex-state-turn-bv9j4pzn file=codex/state/history.go pick=symbol:Turn owner=@khanakia -->
<!-- ds:def id=codex-testedmin-vxn77phg file=codex/compat.go pick=symbol:TestedMin owner=@khanakia -->
<!-- ds:def id=codex-tomldecoder-keep-cd6urynz file=codex/tomldecode.go pick=symbol:tomlDecoder.keep owner=@khanakia -->
<!-- ds:def id=codex-tomldecoder-mismatch-z7r3y8m5 file=codex/tomldecode.go pick=symbol:tomlDecoder.mismatch owner=@khanakia -->
<!-- ds:def id=codex-tomlraw-j7dab432 file=codex/toml.go pick=symbol:TOMLRaw owner=@khanakia -->
<!-- ds:def id=codex-usage-adqs3xpc file=codex/usage.go pick=symbol:Usage owner=@khanakia -->
<!-- ds:def id=codex-usageendpoint-2rx9qqg7 file=codex/usage.go pick=symbol:UsageEndpoint owner=@khanakia -->
<!-- ds:def id=codex-usagefromsnapshot-8sff5ghq file=codex/conversations.go pick=symbol:usageFromSnapshot owner=@khanakia -->
<!-- ds:def id=codex-vendorprimaryl-pjy6hvnt file=codex/usage.go pick=symbol:vendorPrimaryL owner=@khanakia -->
<!-- ds:def id=codex-writer-tax23vch file=codex/codex.go pick=symbol:Writer owner=@khanakia -->
<!-- ds:def id=command-c4wdgewr file=command.go pick=symbol:Command owner=@khanakia -->
<!-- ds:def id=command-marshaljson-gy7a7qs2 file=command.go pick=symbol:Command.MarshalJSON owner=@khanakia -->
<!-- ds:def id=command-redacted-qbsw6442 file=command.go pick=symbol:Command.Redacted owner=@khanakia -->
<!-- ds:def id=command-secretenvnames-72t7wfqg file=command.go pick=symbol:Command.SecretEnvNames owner=@khanakia -->
<!-- ds:def id=command-string-uxssqggr file=command.go pick=symbol:Command.String owner=@khanakia -->
<!-- ds:def id=comparepre-u6e53wq3 file=version.go pick=symbol:comparePre owner=@khanakia -->
<!-- ds:def id=compat-check-yqab46pa file=version.go pick=symbol:Compat.Check owner=@khanakia -->
<!-- ds:def id=compat-warning-yeqfn6xw file=version.go pick=symbol:Compat.Warning owner=@khanakia -->
<!-- ds:def id=compatstatusnames-8wen9ft3 file=enums.go pick=symbol:compatStatusNames owner=@khanakia -->
<!-- ds:def id=configonlyreason-gbavx5nz file=fromspec.go pick=symbol:configOnlyReason owner=@khanakia -->
<!-- ds:def id=configreader-y3cxtpm2 file=capability.go pick=symbol:ConfigReader owner=@khanakia -->
<!-- ds:def id=configscopenames-8qqr6u4z file=enums.go pick=symbol:configScopeNames owner=@khanakia -->
<!-- ds:def id=conversation-7mywvcsh file=capability.go pick=symbol:Conversation owner=@khanakia -->
<!-- ds:def id=conversationquery-8hx6hqtd file=capability.go pick=symbol:ConversationQuery owner=@khanakia -->
<!-- ds:def id=copytree-2u9s9bp6 file=backup.go pick=symbol:CopyTree owner=@khanakia -->
<!-- ds:def id=core-applyhomelinks-q5mmu4xp file=core.go pick=symbol:Core.ApplyHomeLinks owner=@khanakia -->
<!-- ds:def id=core-checks-8dj5h3dt file=core.go pick=symbol:Core.Checks owner=@khanakia -->
<!-- ds:def id=core-command-m93k4eug file=core.go pick=symbol:Core.Command owner=@khanakia -->
<!-- ds:def id=core-defaulthome-6f4pqnem file=core.go pick=symbol:Core.DefaultHome owner=@khanakia -->
<!-- ds:def id=core-envchanges-f8hrn3we file=core.go pick=symbol:Core.EnvChanges owner=@khanakia -->
<!-- ds:def id=core-homeat-c4u3jwew file=core.go pick=symbol:Core.HomeAt owner=@khanakia -->
<!-- ds:def id=core-homefromenv-jk7mncfj file=core.go pick=symbol:Core.HomeFromEnv owner=@khanakia -->
<!-- ds:def id=core-homename-ep9s5ega file=core.go pick=symbol:Core.homeName owner=@khanakia -->
<!-- ds:def id=core-homes-43a2yjev file=core.go pick=symbol:Core.Homes owner=@khanakia -->
<!-- ds:def id=core-ishome-nxgj8ve9 file=core.go pick=symbol:Core.IsHome owner=@khanakia -->
<!-- ds:def id=core-launch-v8h5ys7c file=core.go pick=symbol:Core.Launch owner=@khanakia -->
<!-- ds:def id=core-logincommand-3b2a9mth file=core.go pick=symbol:Core.LoginCommand owner=@khanakia -->
<!-- ds:def id=core-logoutcommand-ztcp3jsm file=core.go pick=symbol:Core.LogoutCommand owner=@khanakia -->
<!-- ds:def id=core-lookpath-ke3ntet7 file=core.go pick=symbol:Core.LookPath owner=@khanakia -->
<!-- ds:def id=core-planhomelinks-zybhmbpb file=core.go pick=symbol:Core.PlanHomeLinks owner=@khanakia -->
<!-- ds:def id=core-runner-4jzfzpz9 file=core.go pick=symbol:Core.Runner owner=@khanakia -->
<!-- ds:def id=costclassnames-x4rv84vr file=enums.go pick=symbol:costClassNames owner=@khanakia -->
<!-- ds:def id=credentialsourcenames-8g8hwb67 file=enums.go pick=symbol:credentialSourceNames owner=@khanakia -->
<!-- ds:def id=darwinprocesses-h9y8tzsg file=hostos_procs.go pick=symbol:parseDarwinPS owner=@khanakia -->
<!-- ds:def id=defaulthomename-2x8teypk file=core.go pick=symbol:DefaultHomeName owner=@khanakia -->
<!-- ds:def id=enumparse-z445n2y9 file=enum.go pick=symbol:enumParse owner=@khanakia -->
<!-- ds:def id=enumstring-57pf7m8z file=enum.go pick=symbol:enumString owner=@khanakia -->
<!-- ds:def id=enumunmarshal-d6njwv72 file=enum.go pick=symbol:enumUnmarshal owner=@khanakia -->
<!-- ds:def id=enumvalues-dwg2jdmz file=enum.go pick=symbol:enumValues owner=@khanakia -->
<!-- ds:def id=envchange-value-43pazd3a file=command.go pick=symbol:EnvChange.Value owner=@khanakia -->
<!-- ds:def id=envlookup-ef3y8bwk file=host.go pick=symbol:EnvLookup owner=@khanakia -->
<!-- ds:def id=envsetsecret-tf8w23qe file=command.go pick=symbol:EnvSetSecret owner=@khanakia -->
<!-- ds:def id=envvalue-sjcyhx97 file=secret.go pick=symbol:EnvValue owner=@khanakia -->
<!-- ds:def id=errbilledpertoken-6maccp3u file=errors.go pick=symbol:ErrBilledPerToken owner=@khanakia -->
<!-- ds:def id=errdangerousnotallowed-cfd5h3du file=errors.go pick=symbol:ErrDangerousNotAllowed owner=@khanakia -->
<!-- ds:def id=errexpired-yj9h93tv file=errors.go pick=symbol:ErrExpired owner=@khanakia -->
<!-- ds:def id=errfeatureunavailable-fzzq5s2u file=errors.go pick=symbol:ErrFeatureUnavailable owner=@khanakia -->
<!-- ds:def id=errforeignhome-z79t6z5e file=errors.go pick=symbol:ErrForeignHome owner=@khanakia -->
<!-- ds:def id=errinvalidid-typkqvpz file=errors.go pick=symbol:ErrInvalidID owner=@khanakia -->
<!-- ds:def id=errinvalidspec-835a58xh file=errors.go pick=symbol:ErrInvalidSpec owner=@khanakia -->
<!-- ds:def id=errmoveblocked-8a8jj7sq file=errors.go pick=symbol:ErrMoveBlocked owner=@khanakia -->
<!-- ds:def id=errnotfound-cusd5chu file=errors.go pick=symbol:ErrNotFound owner=@khanakia -->
<!-- ds:def id=errnotinstalled-e8y46yuj file=errors.go pick=symbol:ErrNotInstalled owner=@khanakia -->
<!-- ds:def id=errnotloggedin-hd6d747t file=errors.go pick=symbol:ErrNotLoggedIn owner=@khanakia -->
<!-- ds:def id=errratelimited-6rsdy999 file=errors.go pick=symbol:ErrRateLimited owner=@khanakia -->
<!-- ds:def id=errscopelimited-6v2zr8df file=errors.go pick=symbol:ErrScopeLimited owner=@khanakia -->
<!-- ds:def id=errunauthorized-bx364rgx file=errors.go pick=symbol:ErrUnauthorized owner=@khanakia -->
<!-- ds:def id=errunknownagent-fbp52357 file=errors.go pick=symbol:ErrUnknownAgent owner=@khanakia -->
<!-- ds:def id=errunknownversion-k45eb95g file=errors.go pick=symbol:ErrUnknownVersion owner=@khanakia -->
<!-- ds:def id=errunsupported-e8zgsckh file=errors.go pick=symbol:ErrUnsupported owner=@khanakia -->
<!-- ds:def id=examples-drift-codexdrift-xspx77gd file=examples/drift/main.go pick=symbol:codexDrift owner=@khanakia -->
<!-- ds:def id=examples-drift-finding-baselinekey-bm3savct file=examples/drift/main.go pick=symbol:finding.baselineKey owner=@khanakia -->
<!-- ds:def id=examples-drift-fixturehost-sm5ek39g file=examples/drift/main.go pick=symbol:fixtureHost owner=@khanakia -->
<!-- ds:def id=examples-drift-helpdrift-bd9jjgwy file=examples/drift/main.go pick=symbol:helpDrift owner=@khanakia -->
<!-- ds:def id=examples-drift-pickhost-k35cnr56 file=examples/drift/main.go pick=symbol:pickHost owner=@khanakia -->
<!-- ds:def id=examples-drift-report-loadbaseline-4xbs9yxq file=examples/drift/main.go pick=symbol:report.loadBaseline owner=@khanakia -->
<!-- ds:def id=examples-drift-report-print-pgxsfup3 file=examples/drift/main.go pick=symbol:report.print owner=@khanakia -->
<!-- ds:def id=examples-drift-run-xbppew9s file=examples/drift/main.go pick=symbol:run owner=@khanakia -->
<!-- ds:def id=examples-drift-transcriptdrift-3bba7jgq file=examples/drift/main.go pick=symbol:transcriptDrift owner=@khanakia -->
<!-- ds:def id=examples-drift-transcriptsample-22745jby file=examples/drift/main.go pick=symbol:transcriptSample owner=@khanakia -->
<!-- ds:def id=examples-drift-versionline-4gjggnvh file=examples/drift/main.go pick=symbol:versionLine owner=@khanakia -->
<!-- ds:def id=examples-drift-walker-read-nhu7788k file=examples/drift/main.go pick=symbol:walker.read owner=@khanakia -->
<!-- ds:def id=examples-drift-walker-walk-22f5frvq file=examples/drift/main.go pick=symbol:walker.walk owner=@khanakia -->
<!-- ds:def id=extractunknown-jcccq46h file=value.go pick=symbol:ExtractUnknown owner=@khanakia -->
<!-- ds:def id=extractunknownfor-yr8rmrzq file=value.go pick=symbol:ExtractUnknownFor owner=@khanakia -->
<!-- ds:def id=featurespan-ckruwege file=version.go pick=symbol:FeatureSpan owner=@khanakia -->
<!-- ds:def id=featurespan-in-vh7rmz94 file=version.go pick=symbol:FeatureSpan.In owner=@khanakia -->
<!-- ds:def id=featurespan-known-h6gygtav file=version.go pick=symbol:FeatureSpan.Known owner=@khanakia -->
<!-- ds:def id=fromspec-4tcd24bd file=fromspec.go pick=symbol:FromSpec owner=@khanakia -->
<!-- ds:def id=home-marshaljson-gwtr45uj file=home.go pick=symbol:Home.MarshalJSON owner=@khanakia -->
<!-- ds:def id=home-string-ry64gtnm file=home.go pick=symbol:Home.String owner=@khanakia -->
<!-- ds:def id=home-target-heq5sahh file=home.go pick=symbol:Home.Target owner=@khanakia -->
<!-- ds:def id=homeenvchange-59nx5j7k file=command.go pick=symbol:HomeEnvChange owner=@khanakia -->
<!-- ds:def id=homelink-a86nv2s4 file=capability.go pick=symbol:HomeLink owner=@khanakia -->
<!-- ds:def id=homelinker-ek5zfyg7 file=capability.go pick=symbol:HomeLinker owner=@khanakia -->
<!-- ds:def id=homelinkerof-fk3qdrnr file=accessor.go pick=symbol:HomeLinkerOf owner=@khanakia -->
<!-- ds:def id=host-4pbbeq6s file=host.go pick=symbol:Host owner=@khanakia -->
<!-- ds:def id=host-systempath-7azvt62d file=host.go pick=symbol:Host.SystemPath owner=@khanakia -->
<!-- ds:def id=hostfromos-yvmqkxwe file=hostos.go pick=symbol:HostFromOS owner=@khanakia -->
<!-- ds:def id=installmethod-isknown-yqgegrt9 file=capability.go pick=symbol:InstallMethod.IsKnown owner=@khanakia -->
<!-- ds:def id=installmethodvalues-ru5s5eaq file=capability.go pick=symbol:InstallMethodValues owner=@khanakia -->
<!-- ds:def id=instructionscopenames-ncxf2q89 file=enums.go pick=symbol:instructionScopeNames owner=@khanakia -->
<!-- ds:def id=isexecutable-395kvm4u file=core.go pick=symbol:isExecutable owner=@khanakia -->
<!-- ds:def id=jsonkeys-pjrw66pn file=value.go pick=symbol:JSONKeys owner=@khanakia -->
<!-- ds:def id=loginflownames-a7bkqn6w file=enums.go pick=symbol:loginFlowNames owner=@khanakia -->
<!-- ds:def id=lookpath-f56nfyzk file=core.go pick=symbol:LookPath owner=@khanakia -->
<!-- ds:def id=lookpathall-u48etgc3 file=core.go pick=symbol:LookPathAll owner=@khanakia -->
<!-- ds:def id=mcpserver-dpx2w5v3 file=capability.go pick=symbol:MCPServer owner=@khanakia -->
<!-- ds:def id=mcptransportvalues-gvqnntsb file=capability.go pick=symbol:MCPTransportValues owner=@khanakia -->
<!-- ds:def id=moveplan-3777j6wu file=move.go pick=symbol:MovePlan owner=@khanakia -->
<!-- ds:def id=moverequest-evw38rsj file=move.go pick=symbol:MoveRequest owner=@khanakia -->
<!-- ds:def id=mustversion-tdb8tu6r file=version.go pick=symbol:MustVersion owner=@khanakia -->
<!-- ds:def id=newcommand-78p36jg6 file=command.go pick=symbol:NewCommand owner=@khanakia -->
<!-- ds:def id=newcore-dqu45h82 file=core.go pick=symbol:NewCore owner=@khanakia -->
<!-- ds:def id=newhome-u52u4c93 file=home.go pick=symbol:NewHome owner=@khanakia -->
<!-- ds:def id=newtoken-xgarx7tt file=secret.go pick=symbol:NewToken owner=@khanakia -->
<!-- ds:def id=opt-get-8uc2tt52 file=value.go pick=symbol:Opt[T].Get owner=@khanakia -->
<!-- ds:def id=opt-iszero-jhcnvsst file=value.go pick=symbol:Opt[T].IsZero owner=@khanakia -->
<!-- ds:def id=opt-marshaljson-sdueyhgc file=value.go pick=symbol:Opt[T].MarshalJSON owner=@khanakia -->
<!-- ds:def id=opt-nj39byj3 file=value.go pick=symbol:Opt owner=@khanakia -->
<!-- ds:def id=opt-or-x7yr72u4 file=value.go pick=symbol:Opt[T].Or owner=@khanakia -->
<!-- ds:def id=opt-unmarshaljson-6u6vzm5j file=value.go pick=symbol:Opt[T].UnmarshalJSON owner=@khanakia -->
<!-- ds:def id=oshttptimeout-3nmmj9j5 file=hostos.go pick=symbol:osHTTPTimeout owner=@khanakia -->
<!-- ds:def id=osprocesses-processes-q5hpmdr9 file=hostos_procs.go pick=symbol:osProcesses.Processes owner=@khanakia -->
<!-- ds:def id=osrunner-a75g5r5m file=hostos.go pick=symbol:osRunner owner=@khanakia -->
<!-- ds:def id=osrunner-run-hm7czx3q file=hostos.go pick=symbol:osRunner.Run owner=@khanakia -->
<!-- ds:def id=ossecrets-get-ux5ua2mu file=hostos.go pick=symbol:osSecrets.Get owner=@khanakia -->
<!-- ds:def id=parseid-rfund9ym file=id.go pick=symbol:ParseID owner=@khanakia -->
<!-- ds:def id=parseversion-7jguuyeq file=version.go pick=symbol:ParseVersion owner=@khanakia -->
<!-- ds:def id=parseversionat-wdv3pw7u file=version.go pick=symbol:parseVersionAt owner=@khanakia -->
<!-- ds:def id=personal-reveal-pjvj854b file=secret.go pick=symbol:Personal.Reveal owner=@khanakia -->
<!-- ds:def id=personal-yfzy8bms file=secret.go pick=symbol:Personal owner=@khanakia -->
<!-- ds:def id=planhomelinks-yg65h8jg file=link.go pick=symbol:PlanHomeLinks owner=@khanakia -->
<!-- ds:def id=processkindvalues-vjn5j6xg file=capability.go pick=symbol:ProcessKindValues owner=@khanakia -->
<!-- ds:def id=processmatch-match-6phmdfg3 file=spec.go pick=symbol:ProcessMatch.Match owner=@khanakia -->
<!-- ds:def id=ratelimiterror-pbyjx7ce file=errors.go pick=symbol:RateLimitError owner=@khanakia -->
<!-- ds:def id=raw-j94jwyct file=value.go pick=symbol:Raw owner=@khanakia -->
<!-- ds:def id=redact-8fhquzvy file=secret.go pick=symbol:redact owner=@khanakia -->
<!-- ds:def id=retryafter-v8wbfwra file=errors.go pick=symbol:RetryAfter owner=@khanakia -->
<!-- ds:def id=rolenames-dm4j429f file=enums.go pick=symbol:roleNames owner=@khanakia -->
<!-- ds:def id=rollbackmove-6gfza3bz file=move.go pick=symbol:rollbackMove owner=@khanakia -->
<!-- ds:def id=runeventtypenames-6vyhtd7t file=enums.go pick=symbol:runEventTypeNames owner=@khanakia -->
<!-- ds:def id=runner-run-5rn5nerj file=core.go pick=symbol:Runner.Run owner=@khanakia -->
<!-- ds:def id=runner-version-87cgudkv file=core.go pick=symbol:Runner.Version owner=@khanakia -->
<!-- ds:def id=scalar-v4y3hks8 file=value.go pick=symbol:Scalar owner=@khanakia -->
<!-- ds:def id=scrubenv-4rdtd62j file=command.go pick=symbol:ScrubEnv owner=@khanakia -->
<!-- ds:def id=secretnamesof-7pd7kykk file=command.go pick=symbol:secretNamesOf owner=@khanakia -->
<!-- ds:def id=sensitivenameparts-55jafnj3 file=command.go pick=symbol:sensitiveNameParts owner=@khanakia -->
<!-- ds:def id=sessionkindvalues-dz2kaxw9 file=capability.go pick=symbol:SessionKindValues owner=@khanakia -->
<!-- ds:def id=sessionstatusnames-9ja6re25 file=enums.go pick=symbol:sessionStatusNames owner=@khanakia -->
<!-- ds:def id=skillscopevalues-43djff4u file=capability.go pick=symbol:SkillScopeValues owner=@khanakia -->
<!-- ds:def id=some-y3g2rs4p file=value.go pick=symbol:Some owner=@khanakia -->
<!-- ds:def id=spec-9rwc3qhn file=spec.go pick=symbol:Spec owner=@khanakia -->
<!-- ds:def id=spec-validate-chxkxnh2 file=spec.go pick=symbol:Spec.Validate owner=@khanakia -->
<!-- ds:def id=specagent-hascapability-tzc86rcg file=fromspec.go pick=symbol:specAgent.HasCapability owner=@khanakia -->
<!-- ds:def id=specagent-unsupportedreason-jp9ga9ft file=fromspec.go pick=symbol:specAgent.UnsupportedReason owner=@khanakia -->
<!-- ds:def id=supported-33dzcvq6 file=accessor.go pick=symbol:Supported owner=@khanakia -->
<!-- ds:def id=token-3nk7gtub file=secret.go pick=symbol:Token owner=@khanakia -->
<!-- ds:def id=token-format-wcqqk5x2 file=secret.go pick=symbol:Token.Format owner=@khanakia -->
<!-- ds:def id=token-iszero-9tp4s4fb file=secret.go pick=symbol:Token.IsZero owner=@khanakia -->
<!-- ds:def id=token-logvalue-55q96ejj file=secret.go pick=symbol:Token.LogValue owner=@khanakia -->
<!-- ds:def id=token-reveal-8wam7b8k file=secret.go pick=symbol:Token.Reveal owner=@khanakia -->
<!-- ds:def id=token-unmarshaljson-5rwgenx7 file=secret.go pick=symbol:Token.UnmarshalJSON owner=@khanakia -->
<!-- ds:def id=tokenboundary-e9swk823 file=version.go pick=symbol:tokenBoundary owner=@khanakia -->
<!-- ds:def id=tokenkindnames-xdc3hc3t file=enums.go pick=symbol:tokenKindNames owner=@khanakia -->
<!-- ds:def id=trustnames-5zu82hd6 file=enums.go pick=symbol:trustNames owner=@khanakia -->
<!-- ds:def id=unknown-2h4bh82e file=value.go pick=symbol:Unknown owner=@khanakia -->
<!-- ds:def id=unknown-keys-j2ud6s3f file=value.go pick=symbol:Unknown.Keys owner=@khanakia -->
<!-- ds:def id=unsupported-zhw69bda file=accessor.go pick=symbol:Unsupported owner=@khanakia -->
<!-- ds:def id=unsupportederror-tub9t75m file=errors.go pick=symbol:UnsupportedError owner=@khanakia -->
<!-- ds:def id=usage-maxpercent-b8fwrpb4 file=capability.go pick=symbol:Usage.MaxPercent owner=@khanakia -->
<!-- ds:def id=usagesourcenames-hagrpw52 file=enums.go pick=symbol:usageSourceNames owner=@khanakia -->
<!-- ds:def id=version-6rz8b62k file=version.go pick=symbol:Version owner=@khanakia -->
<!-- ds:def id=version-compare-fnfvjejx file=version.go pick=symbol:Version.Compare owner=@khanakia -->
<!-- ds:def id=version-iszero-k7c3px3j file=version.go pick=symbol:Version.IsZero owner=@khanakia -->
<!-- ds:def id=version-marshaltext-a4cb9h34 file=version.go pick=symbol:Version.MarshalText owner=@khanakia -->
<!-- ds:def id=version-string-q29jucvt file=version.go pick=symbol:Version.String owner=@khanakia -->
<!-- ds:def id=version-unmarshaltext-ejqmnmf4 file=version.go pick=symbol:Version.UnmarshalText owner=@khanakia -->
<!-- ds:def id=warning-je4hy5xg file=warning.go pick=symbol:Warning owner=@khanakia -->
<!-- ds:def id=warning-string-yxkaha8r file=warning.go pick=symbol:Warning.String owner=@khanakia -->
<!-- ds:def id=warningkindnames-a4xj32ut file=enums.go pick=symbol:warningKindNames owner=@khanakia -->
<!-- ds:def id=warnings-2cpaszv5 file=warning.go pick=symbol:Warnings owner=@khanakia -->
<!-- ds:def id=warnings-add-pv3bqymk file=warning.go pick=symbol:Warnings.Add owner=@khanakia -->
<!-- ds:def id=warnings-has-e3uyt56b file=warning.go pick=symbol:Warnings.Has owner=@khanakia -->
<!-- ds:def id=window-class-sgsfcuv5 file=capability.go pick=symbol:Window.Class owner=@khanakia -->
<!-- ds:def id=windowclassnames-757ahrk7 file=enums.go pick=symbol:windowClassNames owner=@khanakia -->

<!-- ds:def id=host-getenv-qyjtfrc3 file=host.go pick=symbol:Host.Getenv owner=@khanakia -->
<!-- ds:def id=host-lookpath-4k46e3ag file=osrules.go pick=symbol:Host.LookPath owner=@khanakia -->
<!-- ds:def id=host-newcommand-pu5aa4ej file=osrules.go pick=symbol:Host.NewCommand owner=@khanakia -->
<!-- ds:def id=shimtarget-ubxq8m56 file=shim.go pick=symbol:ShimTarget owner=@khanakia -->

## License

[Apache License 2.0](LICENSE).
