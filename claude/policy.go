package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/ubgo/agentio"
)

// Restriction is one policy restriction.
type Restriction struct {
	Allowed bool `json:"allowed"`
}

// PolicyStamp is policy-limits.json.stamp.json. Identity is an account id.
type PolicyStamp struct {
	V           int
	Kind        string
	Identity    agentio.Personal
	SHA         string
	ConfirmedAt time.Time
	HIPAASeen   bool
}

type wireStamp struct {
	V           int              `json:"v"`
	Kind        string           `json:"kind"`
	Identity    agentio.Personal `json:"identity"`
	SHA         string           `json:"sha"`
	ConfirmedAt time.Time        `json:"confirmed_at"`
	HIPAASeen   bool             `json:"hipaa_seen"`
}

// PolicyLimits is policy-limits.json. Signatures are not verified (no key);
// compliance_taints and monitoring_notice stay raw (no non-empty sample).
type PolicyLimits struct {
	Restrictions     map[string]Restriction
	ComplianceTaints []agentio.Raw
	MonitoringNotice agentio.Raw
	Defaults         map[string]agentio.Raw
	Stamp            *PolicyStamp
	Unknown          agentio.Unknown
}

type wirePolicy struct {
	Restrictions     map[string]Restriction     `json:"restrictions"`
	ComplianceTaints []json.RawMessage          `json:"compliance_taints"`
	MonitoringNotice json.RawMessage            `json:"monitoring_notice"`
	Defaults         map[string]json.RawMessage `json:"defaults"`
}

// PolicyLimits reads policy-limits.json and its stamp; nil when absent.
// cost: Disk · Read.
func (a *Agent) PolicyLimits(h Home) (*PolicyLimits, error) {
	p := filepath.Join(h.Dir(), policyLimitsFile)
	data, found, err := readFileBounded(p, maxResponseBytes)
	if err != nil || !found {
		return nil, err
	}
	var w wirePolicy
	unknown, _, err := decodeLenient(policyLimitsFile, data, reflect.TypeFor[wirePolicy](), func(b []byte) error { w = wirePolicy{}; return json.Unmarshal(b, &w) })
	if err != nil {
		return nil, fmt.Errorf("claude: %s: %w", p, err)
	}
	pl := &PolicyLimits{Restrictions: w.Restrictions, MonitoringNotice: rawOrNil(w.MonitoringNotice), Unknown: unknown}
	for _, t := range w.ComplianceTaints {
		pl.ComplianceTaints = append(pl.ComplianceTaints, agentio.Raw(t))
	}
	if w.Defaults != nil {
		pl.Defaults = make(map[string]agentio.Raw, len(w.Defaults))
		for k, v := range w.Defaults {
			pl.Defaults[k] = agentio.Raw(v)
		}
	}
	var st wireStamp
	if readJSONInto(p+stampSuffix, func(b []byte) error { return json.Unmarshal(b, &st) }) {
		pl.Stamp = &PolicyStamp{V: st.V, Kind: st.Kind, Identity: st.Identity, SHA: st.SHA, ConfirmedAt: st.ConfirmedAt, HIPAASeen: st.HIPAASeen}
	}
	return pl, nil
}
