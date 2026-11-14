package main

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ubgo/agentio"
)

// snippet:start optional-field
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

// snippet:end optional-field

// snippet:start unknown-keys
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

// snippet:end unknown-keys

// snippet:start collect-warnings
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

// snippet:end collect-warnings
