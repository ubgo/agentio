package gemini

import (
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/ubgo/agentio"
)

// Instruction file locations (core memoryDiscovery).
const (
	memoryDir      = "memory"
	memoryFile     = "MEMORY.md"
	kindAutoMemory = "MEMORY.md"
)

// InstructionFiles lists the instruction files the CLI loads for dir, in
// its load order (core memoryDiscovery):
//   - global: <home>/<name> for each context file name (GEMINI.md, or
//     settings context.fileName);
//   - project memory: <home>/tmp/<slug>/memory/MEMORY.md;
//   - project: every <name> from dir's git root (the nearest dir with a
//     .git, or dir itself without one) down to dir, root first. A folder
//     the trust file marks DO_NOT_TRUST loads none of them.
//
// Files under subdirectories that the CLI loads just in time, when a tool
// touches them, are not listed. cost: Disk · Read.
func (a *Agent) InstructionFiles(h Home, dir string) ([]agentio.InstructionFile, error) {
	e, err := a.EffectiveSettings(h, dir)
	if err != nil {
		return nil, err
	}
	names := e.ContextFileNames
	var out []agentio.InstructionFile
	for _, n := range names {
		out = appendFile(out, h.Path(n), agentio.InstructionUser, n)
	}
	if dir == "" {
		return out, nil
	}
	if slug, ok, err := a.ProjectSlug(h, dir); err == nil && ok {
		out = appendFile(out, filepath.Join(h.ProjectTmpDir(slug), memoryDir, memoryFile), agentio.InstructionAuto, kindAutoMemory)
	}
	trust, err := a.ProjectTrust(h, dir)
	if err != nil {
		return nil, err
	}
	if trust == agentio.TrustUntrusted {
		return out, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var project []agentio.InstructionFile
	for _, d := range dirsRootToDir(gitRoot(abs), abs) {
		if samePath(d, h.Dir()) {
			continue // the CLI stops at the global .gemini dir
		}
		for _, n := range names {
			project = appendFile(project, filepath.Join(d, n), agentio.InstructionProject, n)
		}
	}
	sort.SliceStable(project, func(i, j int) bool { return project[i].Path < project[j].Path })
	return append(out, project...), nil
}

// appendFile adds path when it is a readable regular file. cost: Disk · Read.
func appendFile(out []agentio.InstructionFile, path string, scope agentio.InstructionScope, kind string) []agentio.InstructionFile {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return out
	}
	if slices.ContainsFunc(out, func(f agentio.InstructionFile) bool { return samePath(f.Path, path) }) {
		return out
	}
	return append(out, agentio.InstructionFile{Path: path, Scope: scope, Kind: kind, Bytes: st.Size()})
}

// gitRoot is the nearest ancestor of dir (dir included) holding .git, or
// dir when there is none (the CLI's default boundary marker). Disk · Read.
func gitRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, gitDir)); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

// dirsRootToDir lists root, then each dir below it down to dir. Pure.
func dirsRootToDir(root, dir string) []string {
	var rev []string
	for d := dir; ; d = filepath.Dir(d) {
		rev = append(rev, d)
		if samePath(d, root) || filepath.Dir(d) == d {
			break
		}
	}
	slices.Reverse(rev)
	return rev
}
