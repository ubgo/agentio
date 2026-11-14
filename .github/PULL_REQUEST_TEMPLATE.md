<!-- Thanks for contributing to agentio. Read CONTRIBUTING.md first. -->

## What and why

<!-- What does this change and why? Link the issue it closes. -->
Closes #

## Modules touched

- [ ] `agentio` (root core)
- [ ] `claude`
- [ ] `codex`
- [ ] `codex/state`
- [ ] `builtin`
- [ ] `examples`
- [ ] docs only

## Type of change

- [ ] Bug fix
- [ ] New vendor shape (file, key, flag or endpoint)
- [ ] New capability or API
- [ ] Vendor version drift (new tested version)
- [ ] Refactor / internal
- [ ] Docs

## Checklist

- [ ] `task check` passes (gofmt, vet, staticcheck, race tests, cross-compile, standalone builds)
- [ ] `task docs:check` passes (docscheck and `ds check`); flagged sentences were re-checked and acked with `ds ack`
- [ ] New vendor structs are modelled on a real file or response, recorded as a fixture with every value replaced
- [ ] No real account data, tokens, emails, machine names or local paths in code, fixtures or docs
- [ ] Every new function states its cost (Pure, Disk, Process, Network, Quota) and access (Read, Write)
- [ ] Nothing new reads `$HOME`, the environment, the clock, the keychain or the network except through `Host`
- [ ] Any new write goes through a `Writer` door and backs the target up first
- [ ] CHANGELOG updated for user-facing changes
