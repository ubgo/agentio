# Optional values, unknown keys and warnings

Vendor files change between CLI versions: keys appear, disappear and change type. agentio models them with three small tools. [`Opt[T]`](ds:block?id=opt-nj39byj3) tells "key absent" apart from "key set to the zero value". [`Unknown`](ds:block?id=unknown-2h4bh82e) keeps every key a struct does not model, so nothing a newer CLI writes is silently dropped. [`Warnings`](ds:block?id=warnings-2cpaszv5) collects drift notes (a key with the wrong type, a record kind the reader skipped) that a tolerant reader returns instead of failing or logging. You meet all three in vendor results; this page shows how to read them and how to use them in your own types.

## Before you start

- Module: `github.com/ubgo/agentio` (stdlib only).
- Nothing on disk; the example decodes JSON literals shaped like vendor files.

## Step 1 — Read an optional field

[`Opt[T]`](ds:block?id=opt-nj39byj3) is a presence-tracked scalar with two fields, `V` and `Set`. [An absent key and a JSON](ds:block?id=opt-unmarshaljson-6u6vzm5j) `null` both leave it unset; [`Get()`](ds:block?id=opt-get-8uc2tt52) returns the value and whether it was set, [`Or(def)`](ds:block?id=opt-or-x7yr72u4) falls back, [`Some(v)`](ds:block?id=some-y3g2rs4p) builds a set one. [It marshals as](ds:block?id=opt-marshaljson-sdueyhgc) `null` when unset, or drops out with the [`omitzero`](ds:block?id=opt-iszero-jhcnvsst) tag option. `T` is limited to the [`Scalar`](ds:block?id=scalar-v4y3hks8) union (`bool`, `string`, `int`, `int32`, `int64`, `uint8`, `uint16`, `uint32`, `uint64`, `float64` and types over them, plus `time.Time`): structs are pointers and lists or maps are nil when absent, so only scalars need it.

<!-- snippet: examples/values/values.go#optional-field -->
```go
// prefs models three keys a vendor version could drop. Opt tells "absent"
// apart from the zero value: a missing "verbose" is not false.
type prefs struct {
	Theme   agentio.Opt[string] `json:"theme,omitzero"`
	Verbose agentio.Opt[bool]   `json:"verbose,omitzero"`
	Retries agentio.Opt[int]    `json:"retries,omitzero"`
}

// optionalFields decodes a file that sets one key, nulls one and omits one.
// Pure.
func optionalFields(p printer) error {
	var pr prefs
	if err := json.Unmarshal([]byte(`{"theme":"dark","retries":null}`), &pr); err != nil {
		return err
	}
	theme, ok := pr.Theme.Get()
	p.line(fmt.Sprintf("opt: theme=%q set=%t", theme, ok))
	p.line(fmt.Sprintf("opt: verbose set=%t or(true)=%t", pr.Verbose.Set, pr.Verbose.Or(true)))
	p.line(fmt.Sprintf("opt: retries set=%t (null reads as unset)", pr.Retries.Set))

	pr.Verbose = agentio.Some(false)
	out, err := json.Marshal(pr)
	if err != nil {
		return err
	}
	p.line("opt: json=" + string(out))
	return nil
}
```

## Step 2 — Find unknown keys

`Unknown` is a `map[string]json.RawMessage` of keys a struct does not model; vendor types carry one in a field named `Unknown`, and [`Unknown.Keys()`](ds:block?id=unknown-keys-j2ud6s3f) returns the names sorted. [`ExtractUnknownFor(data, type)`](ds:block?id=extractunknownfor-yr8rmrzq) computes it from the struct's own json tags (so a new field never needs a second list), [`ExtractUnknown(data, known)`](ds:block?id=extractunknown-jcccq46h) takes an explicit list, and [`JSONKeys(type)`](ds:block?id=jsonkeys-pjrw66pn) returns the modelled keys. [`Raw`](ds:block?id=raw-j94jwyct) (`json.RawMessage`) is the type for a vendor payload kept untyped.

<!-- snippet: examples/values/values.go#unknown-keys -->
```go
// statusFile models two keys of a vendor JSON file; anything else the
// vendor adds lands in Unknown instead of being dropped.
type statusFile struct {
	Model   string          `json:"model"`
	Effort  string          `json:"effort"`
	Unknown agentio.Unknown `json:"-"`
}

// unknownKeys finds the keys the struct does not model. ExtractUnknownFor
// takes the known keys from the struct's json tags, so a new field never
// needs a second list. Values stay raw JSON. Pure.
func unknownKeys(p printer) error {
	data := []byte(`{"model":"opus","effort":"high","fastMode":true,"theme":{"name":"dark"}}`)
	var s statusFile
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	unknown, err := agentio.ExtractUnknownFor(data, reflect.TypeFor[statusFile]())
	if err != nil {
		return err
	}
	s.Unknown = unknown
	p.line(fmt.Sprintf("unknown: modelled=%v", agentio.JSONKeys(reflect.TypeFor[statusFile]())))
	p.line(fmt.Sprintf("unknown: keys=%v", s.Unknown.Keys()))
	theme := s.Unknown["theme"] // an agentio.Raw: the key's JSON bytes, kept verbatim
	p.line("unknown: theme raw=" + string(theme))

	same, err := agentio.ExtractUnknown(data, []string{"model", "effort", "fastMode", "theme"})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("unknown: none left is nil=%t", same == nil))
	return nil
}
```

## Step 3 — Collect warnings

A [`Warning`](ds:block?id=warning-je4hy5xg) has a `Kind` (`WarningKind`), and optionally the `File` and `Key` it is about and a `Detail`. `Warnings` is the list a result carries, nil after a clean read; [`Add`](ds:block?id=warnings-add-pv3bqymk) appends, [`Has(kind)`](ds:block?id=warnings-has-e3uyt56b) checks for a kind, [`String()`](ds:block?id=warning-string-yxkaha8r) renders `kind: file: key: detail`. [The kinds:](ds:block?id=warningkindnames-a4xj32ut) `untested-version`, `version-unknown`, `type-changed`, `old-key-name`, `unknown-record`, `unreadable`.

<!-- snippet: examples/values/values.go#collect-warnings -->
```go
// collectWarnings builds the drift notes a tolerant reader returns instead
// of failing or logging. Detail never holds a vendor value. Pure.
func collectWarnings(p printer) error {
	var ws agentio.Warnings
	p.line(fmt.Sprintf("warnings: clean read is nil=%t", ws == nil))
	ws.Add(agentio.WarnTypeChanged, "settings.json", "cleanupPeriodDays", "expected a number")
	ws.Add(agentio.WarnUnknownRecord, "history.jsonl", "", "record type \"snapshot\" skipped")
	for _, w := range ws {
		p.line("warning: " + w.String())
	}
	p.line(fmt.Sprintf("warnings: has type-changed=%t has untested-version=%t",
		ws.Has(agentio.WarnTypeChanged), ws.Has(agentio.WarnUntestedVersion)))
	out, err := json.Marshal(ws[0])
	if err != nil {
		return err
	}
	p.line("warnings: json=" + string(out))
	return nil
}
```

## Try it

```sh
cd examples && go run ./values
```

The lines for this page:

```text
opt: theme="dark" set=true
opt: verbose set=false or(true)=true
opt: retries set=false (null reads as unset)
opt: json={"theme":"dark","verbose":false}
unknown: modelled=[model effort]
unknown: keys=[fastMode theme]
unknown: theme raw={"name":"dark"}
unknown: none left is nil=true
warnings: clean read is nil=true
warning: type-changed: settings.json: cleanupPeriodDays: expected a number
warning: unknown-record: history.jsonl: record type "snapshot" skipped
warnings: has type-changed=true has untested-version=false
warnings: json={"kind":"type-changed","file":"settings.json","key":"cleanupPeriodDays","detail":"expected a number"}
```

## Pitfalls

- [**Unset is not zero.**](ds:block?id=opt-nj39byj3) `pr.Verbose.V` is `false` both when the file says `false` and when the key is missing; read `Set` (or `Get`) before trusting `V`.
- **[`null`](ds:block?id=opt-unmarshaljson-6u6vzm5j) reads as unset.** A vendor file that writes `"retries": null` gives the same `Opt` as one that omits the key.
- **Without [`omitzero`](ds:block?id=opt-marshaljson-sdueyhgc), an unset `Opt` marshals as `null`.** Add `json:",omitzero"` to drop it from your output.
- **A wrong-typed value fails only that field.** [`Opt.UnmarshalJSON`](ds:block?id=opt-unmarshaljson-6u6vzm5j) returns an error for `"verbose": "yes"`; vendor readers turn it into a `type-changed` warning for that key and keep reading the rest of the file.
- **[`ExtractUnknown`](ds:block?id=extractunknown-jcccq46h) returns nil when nothing is left**, not an empty map; `len(u) == 0` covers both.
- **[`JSONKeys`](ds:block?id=jsonkeys-pjrw66pn) skips `json:"-"` fields and fields of type `Unknown`**, and walks embedded structs. A field without a json tag counts under its Go name, as `encoding/json` does.
- **[`Warning.Detail`](ds:block?id=warning-je4hy5xg) never holds a vendor value**, a secret or personal data, so warnings are safe to log or print.
- **Warnings are data, not logs.** The library never logs; read [`Warnings`](ds:block?id=warnings-2cpaszv5) from results (also `Command.Warnings` from launch builders) and decide yourself.

## Related

- [Versions](versions.md)
- [Errors](errors.md)
- [Enums](enums.md)
- [Drift report](../compat/drift.md)
- Example: [`examples/values`](../../examples/values/)
- [Feature index](../reference/features.md)
