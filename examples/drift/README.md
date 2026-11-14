# drift

The release-routine drift check: decodes every modelled file of every Claude Code and Codex home with the library's own readers and lists what the library does not model yet: unknown keys, unknown enum values, unknown transcript record types, transcript keys newer than the tested range, decode warnings, unknown format versions and read errors, plus the installed version against the tested range. With a saved `claude --help` it also lists long flags missing from `claude/features.tsv`.

Cost: Disk · Read plus one process-table read (live sessions). It never runs a vendor binary, never calls the network and never spends quota; the only secret-store read is Codex `Identity` on a home whose login is kept in the OS keyring. Output names files, keys and enum spellings only, never a value.

By default it reads the real homes of this machine through `agentio.HostFromOS()` (that is what `task drift` runs). `-fixtures` reads the fixture homes in this repository instead; the tests run `run` on a copy of them with an `agenttest` Host.

## Run it

```sh
cd examples && go run ./drift -fixtures | grep -E 'install|home|transcripts|skills|distinct'
```

```text
claude: install not found (agentio: binary not found on PATH: claude)
claude home personal
  transcripts read 2, record versions 2.1.325
claude home work
  transcripts read 1, record versions 2.1.325
codex: install not found (agentio: binary not found on PATH: codex)
codex home 0.162.1
claude	personal	unknown-key	skills	[].Unknown.license	(x1)
246 distinct findings, 0 already in the baseline
```

On your own machine (reads your homes, prints no values):

```sh
task drift
task drift -- -baseline last.tsv -claude-help help.txt
```

Flags:

| Flag | Meaning |
|---|---|
| `-fixtures` | check the repository's fixture homes instead of this machine's |
| `-baseline FILE` | a saved earlier report: list only findings not in it |
| `-claude-help FILE` | a saved `claude --help` text: list long flags missing from `claude/features.tsv` |

Test: `cd examples && go test ./drift`.

## Guides

- [Drift check](../../docs/compat/drift.md)
- [Tested vendor versions](../../docs/compat/compat.md)
- [Feature index](../../docs/reference/features.md#compat-and-drift)
