package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// TrustLevel is one trustedFolders.json value (core TrustLevel).
// Vendor-owned open set.
type TrustLevel string

// TrustLevel values.
const (
	TrustFolder TrustLevel = "TRUST_FOLDER"
	// TrustParent trusts the rule path's parent dir.
	TrustParent TrustLevel = "TRUST_PARENT"
	DoNotTrust  TrustLevel = "DO_NOT_TRUST"
)

// TrustedFoldersPath is the trust file: GEMINI_CLI_TRUSTED_FOLDERS_PATH,
// else <home>/trustedFolders.json. cost: Pure.
func (a *Agent) TrustedFoldersPath(h Home) string {
	if p, ok := a.host.Getenv(TrustedFoldersEnv); ok && p != "" {
		return p
	}
	return h.Path(trustedFoldersFile)
}

// TrustedFolders reads trustedFolders.json, a map of path to TrustLevel
// (source-derived: core LoadedTrustedFolders; comments allowed). A missing
// file is an empty map. cost: Disk · Read.
func (a *Agent) TrustedFolders(h Home) (map[string]TrustLevel, error) {
	p := a.TrustedFoldersPath(h)
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]TrustLevel{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gemini: read %s: %w", p, err)
	}
	out := map[string]TrustLevel{}
	if err := json.Unmarshal(StripJSONComments(agentio.StripBOM(data)), &out); err != nil {
		return nil, fmt.Errorf("gemini: decode %s: %w", p, err)
	}
	return out, nil
}

// TrustOf applies the CLI's rule to dir: the longest rule path whose
// effective dir (the parent for TRUST_PARENT) contains dir decides;
// TrustUnknown when no rule matches. Symlinks are not resolved here (the
// CLI resolves them). cost: Pure.
func TrustOf(rules map[string]TrustLevel, dir string) agentio.Trust {
	dir = filepath.Clean(dir)
	best, bestLen := TrustLevel(""), -1
	for rule, level := range rules {
		eff := filepath.Clean(rule)
		if level == TrustParent {
			eff = filepath.Dir(eff)
		}
		if !within(dir, eff) || len(rule) <= bestLen {
			continue
		}
		best, bestLen = level, len(rule)
	}
	switch best {
	case DoNotTrust:
		return agentio.TrustUntrusted
	case TrustFolder, TrustParent:
		return agentio.TrustTrusted
	}
	return agentio.TrustUnknown
}

// within reports p == dir or p under dir. cost: Pure.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(filepath.ToSlash(rel), "../")
}

// ProjectTrust reads dir's trust in h. Folder trust off
// (security.folderTrust.enabled false in user settings) trusts every dir.
// cost: Disk · Read.
func (a *Agent) ProjectTrust(h Home, dir string) (agentio.Trust, error) {
	user, err := ReadSettingsFile(agentio.ConfigUser, h.UserSettingsPath())
	if err != nil {
		return agentio.TrustUnknown, err
	}
	if sec := user.Settings.Security; sec != nil && sec.FolderTrust != nil && sec.FolderTrust.Enabled != nil && !*sec.FolderTrust.Enabled {
		return agentio.TrustTrusted, nil
	}
	rules, err := a.TrustedFolders(h)
	if err != nil {
		return agentio.TrustUnknown, err
	}
	return TrustOf(rules, dir), nil
}

// Effective merges the settings layers for the keys agentio reads, with
// the CLI's precedence (system > workspace > user > system defaults).
type Effective struct {
	// Model is model.name; "" = the CLI default ("auto").
	Model string
	// ApprovalMode is general.defaultApprovalMode; "" = default.
	ApprovalMode ApprovalMode
	// AuthType is security.auth.selectedType; "" = chosen from env vars
	// (headless) or asked for (interactive).
	AuthType AuthType
	// ContextFileNames are the instruction file names (GEMINI.md default).
	ContextFileNames []string
	// MCPServers merges mcpServers by name, a higher layer replacing a
	// lower layer's entry with the same name.
	MCPServers map[string]MCPServerConfig
	// MCPScopes is the layer each MCPServers entry came from.
	MCPScopes map[string]agentio.ConfigScope
	// MCPSources is the file each MCPServers entry came from.
	MCPSources map[string]string
}

// EffectiveSettings merges Settings(h, dir). The workspace layer is used
// even when the folder is untrusted (the CLI would skip it); check
// ProjectTrust when that matters. cost: Disk · Read.
func (a *Agent) EffectiveSettings(h Home, dir string) (Effective, error) {
	layers, err := a.Settings(h, dir)
	if err != nil {
		return Effective{}, err
	}
	e := Effective{MCPServers: map[string]MCPServerConfig{}, MCPScopes: map[string]agentio.ConfigScope{}, MCPSources: map[string]string{}}
	for _, l := range slices.Backward(layers) { // lowest first, higher layers overwrite
		s := l.Settings
		if s.Model != nil && s.Model.Name != "" {
			e.Model = s.Model.Name
		}
		if s.General != nil && s.General.DefaultApprovalMode != "" {
			e.ApprovalMode = s.General.DefaultApprovalMode
		}
		if s.Security != nil && s.Security.Auth != nil && s.Security.Auth.SelectedType != "" {
			e.AuthType = s.Security.Auth.SelectedType
		}
		if s.Context != nil && len(s.Context.FileName) > 0 {
			e.ContextFileNames = slices.Clone(s.Context.FileName)
		}
		for name, srv := range s.MCPServers {
			e.MCPServers[name], e.MCPScopes[name], e.MCPSources[name] = srv, l.Scope, l.Path
		}
	}
	if len(e.ContextFileNames) == 0 {
		e.ContextFileNames = []string{defaultContextFile}
	}
	return e, nil
}

// ConfigLayers lists the settings files for h and dir, highest precedence
// first. cost: Disk · Read.
func (a *Agent) ConfigLayers(h Home, dir string) ([]agentio.ConfigLayer, error) {
	layers, err := a.Settings(h, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.ConfigLayer, 0, len(layers))
	for _, l := range layers {
		out = append(out, agentio.ConfigLayer{Scope: l.Scope, Path: l.Path, Exists: l.Exists})
	}
	return out, nil
}

// Defaults are the effective model and approval mode. cost: Disk · Read.
func (a *Agent) Defaults(h Home, dir string) (agentio.Defaults, error) {
	e, err := a.EffectiveSettings(h, dir)
	if err != nil {
		return agentio.Defaults{}, err
	}
	d := agentio.Defaults{Model: e.Model}
	if lvl, ok := e.ApprovalMode.Autonomy(); ok {
		d.Autonomy = lvl
	}
	return d, nil
}
