package codex

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// AGENTS.md discovery constants (core/src/agents_md.rs at rust-v0.162.1).
const (
	fileAgentsOverride = "AGENTS.override.md"
	defaultRootMarker  = ".git"
	// defaultProjectDocMaxBytes is upstream's project_doc_max_bytes default.
	defaultProjectDocMaxBytes = 32 << 10
)

// InstructionFile is one instruction file Codex would load, in load order.
type InstructionFile struct {
	Path  string
	Scope agentio.InstructionScope // User | Project
	Kind  string                   // "AGENTS.override.md" | "AGENTS.md" | a fallback name
	Bytes int64
	// Truncated: the project_doc_max_bytes budget ends inside this file
	// (or it is skipped entirely when Bytes == 0 budget remained).
	Truncated bool
}

// Neutral maps onto agentio.InstructionFile. Pure.
func (f InstructionFile) Neutral() agentio.InstructionFile {
	return agentio.InstructionFile{Path: f.Path, Scope: f.Scope, Kind: f.Kind, Bytes: f.Bytes}
}

// projectRoot walks up from dir to the nearest ancestor containing any
// project_root_markers entry (default .git); an explicit empty list means
// cwd only. ok is false when no marker is found.
func projectRoot(dir string, c Config) (string, bool) {
	markers := []string{defaultRootMarker}
	if c.ProjectRootMarkersSet {
		markers = c.ProjectRootMarkers
	}
	dir = filepath.Clean(dir)
	if len(markers) == 0 {
		return dir, false
	}
	for cur := dir; ; {
		for _, m := range markers {
			if _, err := os.Lstat(filepath.Join(cur, m)); err == nil {
				return cur, true
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return dir, false
		}
		cur = parent
	}
}

// dirsRootToCwd lists root, its descendants down to cwd (root first).
func dirsRootToCwd(root, cwd string) []string {
	root, cwd = filepath.Clean(root), filepath.Clean(cwd)
	var out []string
	for cur := cwd; ; {
		out = append(out, cur)
		if cur == root {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return []string{cwd}
		}
		cur = parent
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// ProjectRoot is the project root Codex uses for dir (effective
// project_root_markers of the user config). found=false: no marker, Codex
// reads only dir itself. Disk · Read.
func (a *Agent) ProjectRoot(h Home, dir string) (string, bool, error) {
	c, _, err := readConfigPath(a.Paths(h).Config)
	if err != nil {
		return "", false, err
	}
	root, ok := projectRoot(dir, c)
	return root, ok, nil
}

// InstructionFiles lists, in load order, <home>/AGENTS.md (user) and per
// directory from the project root down to dir the first existing of
// AGENTS.override.md, AGENTS.md, then each project_doc_fallback_filenames
// entry. Project docs are skipped for an untrusted folder; the
// project_doc_max_bytes budget marks the file where it runs out. Disk · Read.
func (a *Agent) InstructionFiles(h Home, dir string) ([]InstructionFile, error) {
	p := a.Paths(h)
	c, _, err := readConfigPath(p.Config)
	if err != nil {
		return nil, err
	}
	var out []InstructionFile
	if st, err := os.Stat(p.AgentsMD); err == nil && st.Mode().IsRegular() {
		out = append(out, InstructionFile{Path: p.AgentsMD, Scope: agentio.InstructionUser, Kind: fileAgentsMD, Bytes: st.Size()})
	}
	if dir == "" {
		return out, nil
	}
	if t, ok := c.TrustOf(dir); ok && t == TrustUntrusted {
		return out, nil
	}
	names := []string{fileAgentsOverride, fileAgentsMD}
	for _, n := range c.ProjectDocFallbackFilenames {
		if n == "" || n == "." || n == ".." || strings.ContainsAny(n, "/\x00") {
			continue
		}
		addName(&names, n)
	}
	budget := int64(defaultProjectDocMaxBytes)
	if c.ProjectDocMaxBytes.Set {
		budget = c.ProjectDocMaxBytes.V
	}
	root, _ := projectRoot(dir, c)
	for _, d := range dirsRootToCwd(root, dir) {
		for _, n := range names {
			path := filepath.Join(d, n)
			st, err := os.Stat(path)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			f := InstructionFile{Path: path, Scope: agentio.InstructionProject, Kind: n, Bytes: st.Size()}
			if st.Size() > budget {
				f.Truncated = true
			}
			budget = max(0, budget-st.Size())
			out = append(out, f)
			break
		}
	}
	return out, nil
}

// TrustOf returns the trust level of the nearest configured ancestor of
// dir (longest-prefix match over [projects]). Pure.
func (c Config) TrustOf(dir string) (TrustLevel, bool) {
	dir = filepath.Clean(dir)
	best, bestLen := TrustLevel(""), -1
	for k, p := range c.Projects {
		if !p.TrustLevel.Set {
			continue
		}
		key := filepath.Clean(k)
		if dir == key || strings.HasPrefix(dir, key+string(filepath.Separator)) || key == string(filepath.Separator) {
			if len(key) > bestLen {
				best, bestLen = p.TrustLevel.V, len(key)
			}
		}
	}
	return best, bestLen >= 0
}
