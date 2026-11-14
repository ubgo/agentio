# testing

A small "your own code" program written to be tested, plus tests that show every part of `agenttest`.

- [main.go](main.go): `collect` loops over every built-in agent (and any config-defined `Spec`) on an injected `agentio.Host` and counts homes, conversations and capabilities. `main` runs it on a Host over the fixture homes checked into this repository, read in place; it never calls `HostFromOS`.
- [main_test.go](main_test.go): `agenttest.NewHost` with `WithUserHome` / `WithEnv` / `WithSecrets` / `WithProcesses` / `WithExec` / `WithHTTP`, `CopyFixture` and `WriteFile`, `FakeSecrets` (lookup order through `Calls`), `FakeProcesses` (live sessions), `FakeExec` (Runner `BinaryVersion`), `FailExec`, an `httptest` server for Claude usage, `FailingHTTP` guarding a Disk read, `agenttest.Conformance` on both vendors, and a loop over agents with a toy `Spec` agent.

Cost: Disk · Read of the fixture homes (and temp copies in the tests). No vendor binary is run, no real network or keychain is used.

## Run it

```sh
cd examples && go run ./testing
```

```text
claude homes=[personal work] conversations=3 capabilities=26
codex homes=[0.162.1] conversations=2 capabilities=22
gemini homes=[] conversations=0 capabilities=14
pi homes=[] conversations=0 capabilities=13
```

```sh
cd examples && go test -count=1 ./testing
```

```text
ok  	github.com/ubgo/agentio/examples/testing	0.075s
```

## Guides

- [Fakes and fixture hosts](../../docs/testing/agenttest.md)
- [The conformance suite](../../docs/testing/conformance.md)
- [Fixture homes](../../docs/testing/fixtures.md)
- [Testing your own code](../../docs/testing/your-code.md)
- [Feature index](../../docs/reference/features.md#testing-agenttest-and-fixtures)
