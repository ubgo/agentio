package main

import (
	"encoding/json"
	"fmt"

	"github.com/ubgo/agentio"
)

// snippet:start parse-enum
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

// snippet:end parse-enum

// snippet:start list-enum
// listEnums prints every value of a set, the canonical list for help text
// and validation, in declaration (wire) order. Pure.
func listEnums(p printer) error {
	p.line(fmt.Sprintf("autonomy values: %v", agentio.AutonomyValues()))
	p.line(fmt.Sprintf("cost classes: %v", agentio.CostClassValues()))
	p.line(fmt.Sprintf("process kinds: %v", agentio.ProcessKindValues()))
	p.line(fmt.Sprintf("capabilities: %d, first %s", len(agentio.CapabilityValues()), agentio.CapabilityValues()[0]))
	return nil
}

// snippet:end list-enum
