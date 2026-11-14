---
name: Bug report
about: agentio reads, decodes or builds something wrong
title: "[bug] "
labels: bug
---

**What happened**

A clear description of the bug.

**Module and call**

Which module (`agentio`, `claude`, `codex`, `codex/state`, `builtin`) and which function or method.

**Steps to reproduce**

A minimal Go program or test, ideally against a fixture home or a temp dir.

**Expected behavior**

What you expected instead.

**Environment**

- agentio module version(s): <!-- from go.mod -->
- Go version: <!-- go version -->
- OS / arch:
- Vendor CLI version: <!-- claude --version / codex --version -->

**Vendor file shape (if a decode is wrong)**

Paste the smallest part of the file that triggers it, with every value replaced (names, ids, emails, paths, tokens). Never paste a real token, credential file or transcript.
