package gemini

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ubgo/agentio"
)

// projectsDoc is projects.json: {"projects": {"<abs project path>": "<slug>"}}
// (core ProjectRegistry; observed live on the first run).
type projectsDoc struct {
	Projects map[string]string `json:"projects"`
}

// slugFallback is the slug of a project whose base name has no
// alphanumerics (core ProjectRegistry.slugify).
const slugFallback = "project"

// Slugify is the CLI's project slug for a dir base name: lower case, every
// byte outside [a-z0-9] becomes "-", runs of "-" collapse, ends are
// trimmed, and an empty result is "project" (core ProjectRegistry.slugify).
// The CLI appends -1, -2 ... when the slug is taken by another path, so a
// slug alone never identifies a project: look it up (ProjectSlug). Pure.
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return slugFallback
	}
	return s
}

// ProjectHash is the projectHash field the CLI writes into every session:
// the sha256 hex of the absolute project dir (core getProjectHash). It
// was also the tmp/ dir name before the slug registry. Pure.
func ProjectHash(absDir string) string {
	sum := sha256.Sum256([]byte(absDir))
	return hex.EncodeToString(sum[:])
}

// registryKey is how the CLI keys projects.json: the resolved absolute
// path, lower-cased on Windows (core ProjectRegistry.normalizePath).
// cost: Pure.
func registryKey(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("gemini: project %s: %w", dir, err)
	}
	if runtime.GOOS == agentio.OSWindows {
		abs = strings.ToLower(abs)
	}
	return abs, nil
}

// Projects reads projects.json: project dir to slug. A missing file is an
// empty map. cost: Disk · Read.
func (a *Agent) Projects(h Home) (map[string]string, error) {
	data, err := os.ReadFile(h.Path(projectsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gemini: read projects: %w", err)
	}
	var doc projectsDoc
	if err := json.Unmarshal(agentio.StripBOM(data), &doc); err != nil {
		return nil, fmt.Errorf("gemini: decode %s: %w", h.Path(projectsFile), err)
	}
	if doc.Projects == nil {
		doc.Projects = map[string]string{}
	}
	return doc.Projects, nil
}

// ProjectSlug finds dir's slug as the CLI does: projects.json first, then
// the .project_root marker of each tmp/<slug> dir (the CLI's own fallback
// when the registry lacks the path). ok false when the project never ran
// in this home. cost: Disk · Read.
func (a *Agent) ProjectSlug(h Home, dir string) (string, bool, error) {
	key, err := registryKey(dir)
	if err != nil {
		return "", false, err
	}
	reg, err := a.Projects(h)
	if err != nil {
		return "", false, err
	}
	if slug, ok := reg[key]; ok && slug != "" {
		return slug, true, nil
	}
	roots, err := a.projectRoots(h)
	if err != nil {
		return "", false, err
	}
	for slug, root := range roots {
		if k, err := registryKey(root); err == nil && k == key {
			return slug, true, nil
		}
	}
	return "", false, nil
}

// projectRoots reads tmp/<slug>/.project_root for every project dir: slug
// to the absolute project path the CLI recorded. cost: Disk · Read.
func (a *Agent) projectRoots(h Home) (map[string]string, error) {
	entries, err := os.ReadDir(h.Path(tmpDir))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gemini: read %s: %w", h.Path(tmpDir), err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(h.Path(tmpDir), e.Name(), projectRootFile))
		if err != nil {
			continue // tmp/bin and dirs of other tools have no marker
		}
		out[e.Name()] = strings.TrimSpace(string(b))
	}
	return out, nil
}

// ProjectTmpDir is <home>/tmp/<slug>: the per-project runtime dir holding
// chats/, logs.json and checkpoints. cost: Pure.
func (h Home) ProjectTmpDir(slug string) string { return h.Path(tmpDir, slug) }
