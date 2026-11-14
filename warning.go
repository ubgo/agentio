package agentcli

import "strings"

// Warning is one drift note a tolerant reader or launch builder returns
// instead of logging or failing (SPEC §5.8). File and Key locate it;
// Detail explains it and never holds a vendor value, secret or personal
// data.
type Warning struct {
	Kind   WarningKind `json:"kind"`
	File   string      `json:"file,omitempty"`
	Key    string      `json:"key,omitempty"`
	Detail string      `json:"detail,omitempty"`
}

// warningSep joins the located parts of a warning in String.
const warningSep = ": "

// String renders "kind: file: key: detail", skipping empty parts. Pure.
func (w Warning) String() string {
	parts := []string{w.Kind.String()}
	for _, p := range []string{w.File, w.Key, w.Detail} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, warningSep)
}

// Warnings is the list a result carries; nil when the read was clean.
type Warnings []Warning

// Add appends a warning. Pure.
func (ws *Warnings) Add(kind WarningKind, file, key, detail string) {
	*ws = append(*ws, Warning{Kind: kind, File: file, Key: key, Detail: detail})
}

// Has reports whether a warning of kind is present. Pure.
func (ws Warnings) Has(kind WarningKind) bool {
	for _, w := range ws {
		if w.Kind == kind {
			return true
		}
	}
	return false
}
