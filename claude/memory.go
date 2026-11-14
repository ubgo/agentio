package claude

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/khanakia/agentcli/frontmatter"
)

// MemoryKind is the kind of a memory (instruction) file.
type MemoryKind string

// Memory kinds (CLAUDE_API §7).
const (
	MemoryManaged     MemoryKind = "managed"
	MemoryUser        MemoryKind = "user"
	MemoryUserRule    MemoryKind = "user-rule"
	MemoryProject     MemoryKind = "project"
	MemoryProjectRule MemoryKind = "project-rule"
	MemoryLocal       MemoryKind = "local"
	MemoryAgentsMD    MemoryKind = "agents-md"
	MemoryAuto        MemoryKind = "auto"
	MemoryAgentMemory MemoryKind = "agent-memory"
	MemoryImport      MemoryKind = "import"
)

// IsKnown reports a documented kind. cost: Pure.
func (k MemoryKind) IsKnown() bool {
	switch k {
	case MemoryManaged, MemoryUser, MemoryUserRule, MemoryProject, MemoryProjectRule, MemoryLocal, MemoryAgentsMD,
		MemoryAuto, MemoryAgentMemory, MemoryImport:
		return true
	}
	return false
}

// MemoryFile is one instruction file in load order.
type MemoryFile struct {
	Kind     MemoryKind
	Path     string
	RealPath string // symlinks resolved
	// PathsGlob is a rule's frontmatter `paths`.
	PathsGlob []string
	// Imports are the parsed @refs; followed only when asked.
	Imports []Import
	Size    int64
}

// Import is one `@path` reference inside a memory file.
type Import struct {
	Raw      string
	Resolved string
	Line     int
	External bool // outside the project and the home
	Exists   bool
}

// Import parsing and bounds.
const (
	importMark      = "@"
	homeMark        = "~"
	codeFence       = "```"
	maxImportDepth  = 5
	memoryReadBytes = 1 << 20
)

// ParseImports returns the `@path` lines of a memory body, skipping fenced
// code. Raw only; Resolved is left for the caller. cost: Pure.
func ParseImports(body []byte) []Import {
	var out []Import
	inFence := false
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, scanStartBytes), memoryReadBytes)
	for n := 1; sc.Scan(); n++ {
		t := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(t, codeFence) {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(t, importMark) {
			continue
		}
		ref := strings.Fields(strings.TrimPrefix(t, importMark))
		if len(ref) == 0 {
			continue
		}
		out = append(out, Import{Raw: ref[0], Line: n})
	}
	return out
}

// resolveImport resolves a raw ref against the importing file.
func (a *Agent) resolveImport(raw, fromFile string) string {
	switch {
	case raw == homeMark || strings.HasPrefix(raw, homeMark+"/"):
		return filepath.Join(a.host.UserHome, strings.TrimPrefix(raw, homeMark))
	case filepath.IsAbs(raw):
		return filepath.Clean(raw)
	}
	return filepath.Join(filepath.Dir(fromFile), raw)
}

// within reports whether p is dir or under it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// memoryFile stats and parses one file; ok is false when it does not exist.
func (a *Agent) memoryFile(kind MemoryKind, path string, roots []string) (MemoryFile, bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return MemoryFile{}, false
	}
	m := MemoryFile{Kind: kind, Path: path, RealPath: path, Size: fi.Size()}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		m.RealPath = real
	}
	data, _, err := readFileBounded(path, memoryReadBytes)
	if err != nil {
		return m, true
	}
	doc, body, err := frontmatter.Parse(data)
	if err == nil {
		m.PathsGlob = doc.List(fmPaths)
	} else {
		body = data
	}
	for _, imp := range ParseImports(body) {
		imp.Resolved = a.resolveImport(imp.Raw, path)
		imp.Exists = fileExists(imp.Resolved)
		imp.External = true
		for _, r := range roots {
			if within(imp.Resolved, r) {
				imp.External = false
			}
		}
		m.Imports = append(m.Imports, imp)
	}
	return m, true
}

// markdownIn lists *.md in dir, sorted.
func markdownIn(dir string) []string {
	entries, err := sortedEntries(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), markdownExt) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// ancestors lists dirs from the filesystem root down to dir.
func ancestors(dir string) []string {
	var out []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		out = append([]string{d}, out...)
		if filepath.Dir(d) == d {
			return out
		}
	}
}

// MemoryFiles lists the instruction files Claude Code loads for p in load
// order: managed CLAUDE.md, user CLAUDE.md and rules, project files from
// the filesystem root down to p.Dir (CLAUDE.md, .claude/CLAUDE.md,
// .claude/rules/*.md, AGENTS.md), then CLAUDE.local.md. follow expands
// @imports depth-capped at 5. cost: Disk · Read.
func (a *Agent) MemoryFiles(h Home, p Project, follow bool) ([]MemoryFile, error) {
	roots := []string{p.Dir, h.Dir(), a.host.UserHome}
	if p.RepoRoot != "" {
		roots = append(roots, p.RepoRoot)
	}
	var out []MemoryFile
	listed := map[string]bool{}
	// add skips a file already listed under an earlier layer: a project
	// under the user home would otherwise list ~/.claude/CLAUDE.md twice.
	add := func(kind MemoryKind, path string) {
		if m, ok := a.memoryFile(kind, path, roots); ok && !listed[m.RealPath] {
			listed[m.RealPath] = true
			out = append(out, m)
		}
	}
	add(MemoryManaged, a.SystemPaths().ManagedMemory)
	add(MemoryUser, filepath.Join(h.Dir(), memoryFileName))
	for _, r := range markdownIn(filepath.Join(h.Dir(), rulesDir)) {
		add(MemoryUserRule, r)
	}
	for _, d := range ancestors(p.Dir) {
		add(MemoryProject, filepath.Join(d, memoryFileName))
		add(MemoryProject, filepath.Join(d, dotClaudeDir, memoryFileName))
		for _, r := range markdownIn(filepath.Join(d, dotClaudeDir, rulesDir)) {
			add(MemoryProjectRule, r)
		}
		add(MemoryAgentsMD, filepath.Join(d, agentsMDFile))
	}
	add(MemoryLocal, filepath.Join(p.Dir, localMemoryFile))
	if !follow {
		return out, nil
	}
	seen := map[string]bool{}
	for _, m := range out {
		seen[m.RealPath] = true
	}
	var expanded []MemoryFile
	var expand func(m MemoryFile, depth int)
	expand = func(m MemoryFile, depth int) {
		expanded = append(expanded, m)
		if depth >= maxImportDepth {
			return
		}
		for _, imp := range m.Imports {
			if !imp.Exists {
				continue
			}
			child, ok := a.memoryFile(MemoryImport, imp.Resolved, roots)
			if !ok || seen[child.RealPath] {
				continue
			}
			seen[child.RealPath] = true
			expand(child, depth+1)
		}
	}
	for _, m := range out {
		expand(m, 0)
	}
	return expanded, nil
}

// AutoMemory lists the project's auto-memory files (HistoryDir/memory/*.md,
// MEMORY.md is the index). cost: Disk · Read.
func (a *Agent) AutoMemory(h Home, p Project) ([]MemoryFile, error) {
	dir := a.ProjectPaths(h, p).AutoMemory
	var out []MemoryFile
	for _, f := range markdownIn(dir) {
		if m, ok := a.memoryFile(MemoryAuto, f, []string{dir}); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
