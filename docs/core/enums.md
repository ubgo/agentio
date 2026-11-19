# Enums

agentio never uses bare strings for a fixed set of values. Sets the library owns are closed enums: a `uint8` type with `String()`, `MarshalText`, `UnmarshalText`, a `XValues()` list and a [`ParseX()`](ds:block?id=enumparse-z445n2y9) function; [unknown text is an error](ds:block?id=enumunmarshal-d6njwv72). Sets that come from a vendor and may grow (install methods, MCP transports, process kinds) are open string types with an [`IsKnown()`](ds:block?id=installmethod-isknown-yqgegrt9) method; an unknown value decodes and is kept. You want this page to read an enum from config, print one, or build help text that lists every value.

## Before you start

- Module: `github.com/ubgo/agentio` (stdlib only).

## Step 1 — Parse an enum from text

[`ParseX(s)`](ds:block?id=enumparse-z445n2y9) returns `(X, bool)`. In a struct, a closed enum decodes from its wire name through [`UnmarshalText`](ds:block?id=enumunmarshal-d6njwv72) and rejects anything else; it encodes as its name, never as a number.

<!-- snippet: examples/values/enums.go#parse-enum -->
```go
// launchConfig is a caller config read from JSON or YAML. Closed-set enums
// decode from their wire names and reject anything else.
type launchConfig struct {
	Autonomy agentio.Autonomy `json:"autonomy"`
}

// parseEnums parses closed sets (library-owned, unknown text is an error)
// and open sets (vendor-derived strings, unknown values kept). Pure.
func parseEnums(p printer) error {
	a, ok := agentio.ParseAutonomy("accept-edits")
	p.line(fmt.Sprintf("parse autonomy: %s ok=%t", a, ok))
	_, ok = agentio.ParseAutonomy("yolo")
	p.line(fmt.Sprintf("parse autonomy yolo: ok=%t", ok))

	var cfg launchConfig
	if err := json.Unmarshal([]byte(`{"autonomy":"read-only"}`), &cfg); err != nil {
		return err
	}
	p.line(fmt.Sprintf("json autonomy: %s", cfg.Autonomy))
	err := json.Unmarshal([]byte(`{"autonomy":"yolo"}`), &cfg)
	p.line(fmt.Sprintf("json autonomy yolo: %v", err))
	out, err := json.Marshal(launchConfig{Autonomy: agentio.AutonomyFull})
	if err != nil {
		return err
	}
	p.line("json out: " + string(out))
	p.line(fmt.Sprintf("out of range: %s", agentio.Autonomy(42)))

	// Open sets: a value a newer vendor version adds still decodes; IsKnown
	// says whether the library has a name for it.
	for _, m := range []agentio.InstallMethod{agentio.InstallNPM, "mise"} {
		p.line(fmt.Sprintf("install method %s: known=%t", m, m.IsKnown()))
	}
	return nil
}
```

The closed sets, with their wire names in declaration order:

| Type | Values |
|---|---|
| [`Access`](ds:block?id=accessnames-aun8gmkk) | `read`, `write` |
| [`Autonomy`](ds:block?id=autonomynames-upa3mjff) | `default`, `read-only`, `ask`, `accept-edits`, `full` |
| [`Billing`](ds:block?id=billingnames-phdn4dkq) | `unknown`, `subscription`, `per-token` |
| [`Capability`](ds:block?id=capabilitynames-pdt7huwg) | 28 values, `installer` … `launch-config` ([Shared capabilities and accessors](../capabilities/accessors.md)) |
| [`CheckStatus`](ds:block?id=checkstatusnames-r66q6t8d) | `skip`, `ok`, `info`, `warn`, `fail` |
| [`CompatStatus`](ds:block?id=compatstatusnames-8wen9ft3) | `unknown`, `in-range`, `newer`, `older` |
| [`Concern`](ds:block?id=concernnames-r7c4n2lq) | `none`, `model`, `prompt`, `add-dir`, `autonomy`, `headless`, `resume`, `credential`, `settings` |
| [`ConfigScope`](ds:block?id=configscopenames-8qqr6u4z) | `unknown`, `managed`, `user`, `project`, `local`, `profile`, `cli`, `plugin` |
| [`CostClass`](ds:block?id=costclassnames-x4rv84vr) | `pure`, `disk`, `process`, `network`, `quota` |
| [`CredentialSource`](ds:block?id=credentialsourcenames-8g8hwb67) | `none`, `keychain`, `file`, `keyring`, `env` |
| [`InstructionScope`](ds:block?id=instructionscopenames-ncxf2q89) | `managed`, `user`, `project`, `local`, `auto`, `imported` |
| [`LoginFlow`](ds:block?id=loginflownames-a7bkqn6w) | `browser`, `device`, `api-key-stdin`, `access-token-stdin` |
| [`Role`](ds:block?id=rolenames-dm4j429f) | `user`, `assistant` |
| [`RunEventType`](ds:block?id=runeventtypenames-6vyhtd7t) | `other`, `started`, `message`, `tool`, `usage`, `done`, `error` |
| [`SessionStatus`](ds:block?id=sessionstatusnames-9ja6re25) | `unknown`, `idle`, `busy` |
| [`SettingsDelivery`](ds:block?id=settingsdeliverynames-r7d8v3kp) | `file`, `inline` |
| [`TokenKind`](ds:block?id=tokenkindnames-xdc3hc3t) | `unknown`, `oauth-token`, `api-key`, `api-key-helper` |
| [`Trust`](ds:block?id=trustnames-5zu82hd6) | `unknown`, `trusted`, `untrusted` |
| [`UsageSource`](ds:block?id=usagesourcenames-hagrpw52) | `unknown`, `server`, `cache`, `transcript`, `probe` |
| [`WarningKind`](ds:block?id=warningkindnames-a4xj32ut) | `untested-version`, `version-unknown`, `type-changed`, `old-key-name`, `unknown-record`, `unreadable`, `unknown-key` |
| [`WindowClass`](ds:block?id=windowclassnames-757ahrk7) | `unknown`, `short`, `day`, `week`, `month` |

The open sets, each with `XValues()` listing the names the library knows and `IsKnown()`:

| Type | Known values |
|---|---|
| [`InstallMethod`](ds:block?id=installmethodvalues-ru5s5eaq) | `npm`, `bun`, `pnpm`, `brew`, `native`, `standalone`, `volta`, `unknown` (vendor packages add their own, such as [`claude.InstallNPMGlobal`](ds:block?id=claude-installnpmglobal-rnn85kq2)) |
| [`MCPTransport`](ds:block?id=mcptransportvalues-gvqnntsb) | `stdio`, `http`, `sse`, `ws` |
| [`ProcessKind`](ds:block?id=processkindvalues-vjn5j6xg) | `cli`, `exec`, `app-server`, `mcp-server`, `desktop`, `bridge`, `launcher` |
| [`SessionKind`](ds:block?id=sessionkindvalues-dz2kaxw9) | `interactive`, `bg` |
| [`SkillScope`](ds:block?id=skillscopevalues-43djff4u) | `user`, `project`, `plugin`, `managed`, `system` |
| [`CheckID`](ds:block?id=checkidvalues-rw58qvqg) | `binary-found`, `home`, `home-env`, `api-billing-env`, `version-tested`, `login` |

## Step 2 — List every value

[`XValues()`](ds:block?id=enumvalues-dwg2jdmz) returns every value in declaration order, the canonical list for help text, flag validation and table headers:

<!-- snippet: examples/values/enums.go#list-enum -->
```go
// listEnums prints every value of a set, the canonical list for help text
// and validation, in declaration (wire) order. Pure.
func listEnums(p printer) error {
	p.line(fmt.Sprintf("autonomy values: %v", agentio.AutonomyValues()))
	p.line(fmt.Sprintf("cost classes: %v", agentio.CostClassValues()))
	p.line(fmt.Sprintf("process kinds: %v", agentio.ProcessKindValues()))
	p.line(fmt.Sprintf("capabilities: %d, first %s", len(agentio.CapabilityValues()), agentio.CapabilityValues()[0]))
	return nil
}
```

## Try it

```sh
cd examples && go run ./values
```

The lines for this page:

```text
parse autonomy: accept-edits ok=true
parse autonomy yolo: ok=false
json autonomy: read-only
json autonomy yolo: agentio: unknown autonomy "yolo"
json out: {"autonomy":"full"}
out of range: autonomy(42)
install method npm: known=true
install method mise: known=false
autonomy values: [default read-only ask accept-edits full]
cost classes: [pure disk process network quota]
process kinds: [cli exec app-server mcp-server desktop bridge launcher]
capabilities: 32, first installer
```

## Pitfalls

- **The zero value is the first name.** [`var a agentio.Autonomy`](ds:block?id=autonomynames-upa3mjff) is `default`, `var k agentio.TokenKind` is `unknown`; most sets start with a safe or unknown value for that reason, but [`Access`](ds:block?id=accessnames-aun8gmkk) starts with `read` and `Role` with `user`.
- **An out-of-range number prints, it does not panic.** [`agentio.Autonomy(42).String()`](ds:block?id=enumstring-57pf7m8z) is `autonomy(42)`, so a corrupted value is greppable; `ParseX` and `UnmarshalText` never produce one.
- **Closed sets reject unknown text; open sets keep it.** Decoding `"yolo"` into an [`Autonomy`](ds:block?id=autonomy-unmarshaltext-ves9gtjm) fails; decoding `"mise"` into an [`InstallMethod`](ds:block?id=installmethod-isknown-yqgegrt9) succeeds and `IsKnown()` is false. Do not treat an unknown open value as an error: a newer vendor version may have added it.
- **Values are append-only.** New values are added at the end, so the wire names are stable; never persist the numeric value, use the name.
- **[`Capability`](ds:block?id=capabilitynames-pdt7huwg) has 32 values**, one per shared capability; `agentio.Supported(a)` and the accessors are on [Shared capabilities and accessors](../capabilities/accessors.md).

## Related

- [Optional values, unknown keys and warnings](values.md)
- [Targets and commands](launch.md#step-3--check-the-autonomy-level)
- [Shared capabilities and accessors](../capabilities/accessors.md)
- Example: [`examples/values`](../../examples/values/)
- [Feature index](../reference/features.md)
