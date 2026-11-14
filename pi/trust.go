package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// TrustDecisions is trust.json: saved project-trust decisions keyed by
// canonical dir. A value is true (trusted), false (untrusted) or null (no
// decision; pi keeps walking up), PKG/dist/core/trust-manager.js:77-100.
type TrustDecisions map[string]*bool

// ReadTrust reads h's trust.json; an absent file is an empty map.
// Disk · Read.
func (a *Agent) ReadTrust(h Home) (TrustDecisions, error) {
	p := a.Paths(h).Trust
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil {
		return nil, fmt.Errorf("pi: read %s: %w", p, err)
	}
	if !found {
		return TrustDecisions{}, nil
	}
	var td TrustDecisions
	if err := json.Unmarshal(data, &td); err != nil {
		return nil, fmt.Errorf("pi: decode %s: %w", p, err)
	}
	return td, nil
}

// ProjectTrust answers whether pi loads dir's project resources, in pi's
// order (PKG/docs/security.md:59-82, trust-manager.js:25-40): the nearest
// saved true/false decision walking up from the canonical dir, else the
// global defaultProjectTrust ("always" trusts, "never" refuses, "ask" and
// absent are unknown: pi asks at a terminal and skips project resources
// in print/json mode). A --approve or extension decision at launch is not
// on disk and so not seen here. Disk · Read.
func (a *Agent) ProjectTrust(h Home, dir string) (agentio.Trust, error) {
	abs, err := absDir(dir)
	if err != nil || abs == "" {
		return agentio.TrustUnknown, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	td, err := a.ReadTrust(h)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	windows := a.host.GOOS == agentio.OSWindows
	for d := abs; ; d = filepath.Dir(d) {
		for k, v := range td {
			if v != nil && samePathKey(k, d, windows) {
				if *v {
					return agentio.TrustTrusted, nil
				}
				return agentio.TrustUntrusted, nil
			}
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	s, _, _, err := a.ReadSettings(h)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	switch s.DefaultProjectTrust.Or(TrustAsk) {
	case TrustAlways:
		return agentio.TrustTrusted, nil
	case TrustNever:
		return agentio.TrustUntrusted, nil
	}
	return agentio.TrustUnknown, nil
}

// samePathKey compares a trust.json key with a dir the way pi does:
// case-folded on Windows (paths.js:21-43). Pure.
func samePathKey(key, dir string, windows bool) bool {
	k := filepath.Clean(key)
	if windows {
		return strings.EqualFold(k, dir)
	}
	return k == dir
}

// fileExists reports an existing file or dir. Disk · Read.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
