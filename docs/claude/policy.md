# Claude policy limits

An organisation can restrict what Claude Code may do (remote control, web setup, connector downloads, …). Claude Code caches those limits in `<home>/policy-limits.json` with a stamp file beside it. `a.PolicyLimits(h)` reads both. Use it to explain why a feature is greyed out for a work account.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent and a home (see [The Claude agent](agent.md)).
- Files: `policy-limits.json` and `policy-limits.json.stamp.json` in the home; Disk read.

The snippet comes from [`examples/claudesettings`](../../examples/claudesettings/), which adds a stamp file to the fixture home.

## Step 1 — Read policy limits

`PolicyLimits` returns `nil` when the home has no file. Otherwise `Restrictions` maps a restriction name to `Restriction{Allowed}`, `Defaults` keeps each default as raw JSON, `ComplianceTaints` and `MonitoringNotice` stay raw, and `Stamp` (a `PolicyStamp`: version, kind, the account it was confirmed for, SHA, confirm time, HIPAA flag) is set when the stamp file exists.

<!-- snippet: examples/claudesettings/main.go#policy -->
```go
// showPolicy reads the organisation's policy limits and their stamp.
func showPolicy(p printer, host agentio.Host) error {
	p.section("policy limits")
	a := claude.New(host)
	pl, err := a.PolicyLimits(a.DefaultHome()) // nil when the file is absent
	if err != nil {
		return err
	}
	if pl == nil {
		p.line("no policy limits")
		return nil
	}
	names := make([]string, 0, len(pl.Restrictions))
	for name := range pl.Restrictions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p.line(fmt.Sprintf("restriction %s allowed=%t", name, pl.Restrictions[name].Allowed))
	}
	p.line(fmt.Sprintf("defaults=%d", len(pl.Defaults)))
	if pl.Stamp != nil {
		p.line(fmt.Sprintf("stamp kind=%s identity=%s confirmed=%s", pl.Stamp.Kind, pl.Stamp.Identity, pl.Stamp.ConfirmedAt.Format(time.RFC3339)))
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudesettings
```

The section this page covers:

```text
== policy limits
restriction allow_connector_file_downloads allowed=false
restriction allow_quick_web_setup allowed=false
restriction allow_remote_control allowed=false
restriction enforce_web_search_mcp_isolation allowed=false
defaults=1
stamp kind=org identity=[redacted] confirmed=2026-11-01T10:00:00Z
```

## Pitfalls

- **Signatures are not verified.** The library has no key to check them; treat the file as what Claude Code cached, not as proof of policy.
- `Restrictions` is a map: sort the names yourself for stable output, as the example does.
- `Stamp.Identity` is an account id typed `agentio.Personal`; it prints `[redacted]`.
- `ComplianceTaints`, `MonitoringNotice` and the `Defaults` values are raw JSON because no non-empty sample was recorded; decode them yourself if you rely on a shape.
- Policy limits are separate from managed settings: an admin can also lock settings through the managed layers (see [Claude settings](settings.md#step-3--read-managed-and-remote-settings)).

## Related

- [Claude settings](settings.md), [Claude permissions](permissions.md), [Claude account](account.md)
- Neutral view: [Config layers](../capabilities/config.md)
- Example: [`examples/claudesettings`](../../examples/claudesettings/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
