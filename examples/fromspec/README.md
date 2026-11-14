# Config-defined agents example

Describes a made-up coding agent, "Acme Code", as a plain `agentio.Spec`, shows what `Spec.Validate` refuses, builds it with `agentio.FromSpec`, and uses it like a built-in: homes, `agentio.Supported` and `agentio.Unsupported`, launch and resume commands from the templates (and the refusals for headless and full autonomy), the login command, process classification and the L1 doctor checks.

Everything runs in a temp folder with an inert fake `acme` binary; nothing is executed.

## Run

```sh
cd examples && go run ./fromspec
```

Test:

```sh
cd examples && go test ./fromspec/...
```

## Output

```text
== validate ==
agentio: invalid spec: acme: resume args [--resume] lack {id}
agentio: invalid spec: acme: default home "/etc/acme" must be relative to the user home true
== build ==
home personal default=true ~/.acme
home work default=false ~/.acme-work
supported: [installer binary-version login-commander process-classifier home-linker doctor]
usage: config-only agent: a spec describes L1 only; this needs code
without login args: login commander false (spec has no login args)
== launch ==
personal: acme (ACME_HOME set=false )
personal: acme --model acme-large --message "fix the build" (ACME_HOME set=false )
work: acme --resume c-42 (ACME_HOME set=true ~/.acme-work)
headless: agentio: unsupported: acme: headless: not described by the agent spec (unsupported=true)
full autonomy: agentio: full autonomy needs Target.AllowDangerous
== login, process, doctor ==
login: acme login, logout: acme logout
device login: agentio: unsupported: acme: login flow device: not described by the agent spec
pid 10: match=true kind=cli session=true
pid 11: match=false kind= session=false
check binary-found    ok   /opt/bin/acme
check home            ok   ~/.acme
check api-billing-env warn ACME_API_KEY set: launches bill per token
```

## Guides

- [Config-defined agents](../../docs/capabilities/fromspec.md)
- [Built-in agents](../../docs/capabilities/builtin.md#step-3--add-config-defined-agents)
- [Feature index](../../docs/reference/features.md)
