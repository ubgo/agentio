// Package archtest guards the module and code rules of SPEC §10.1 by parsing
// every Go file of the repository with go/parser. It reads source only; it
// never builds or runs the code it checks.
package archtest

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// modulePrefix is the import path of the root module; vendor modules extend it.
const modulePrefix = "github.com/khanakia/agentcli"

// Vendor module import paths (SPEC §3).
const (
	modClaude  = modulePrefix + "/claude"
	modCodex   = modulePrefix + "/codex"
	modState   = modulePrefix + "/codex/state"
	modBuiltin = modulePrefix + "/builtin"
	modExample = modulePrefix + "/examples"
)

// hostFileGlob names the root files allowed to touch the real machine
// (HostFromOS and the OS-backed implementations it returns).
const hostFilePrefix = "hostos"

// srcFile is one parsed Go file with the module it belongs to.
type srcFile struct {
	rel    string // path relative to the repo root, slash separated
	module string // import path of the owning module
	test   bool
	gen    bool // carries a "Code generated" header
	fset   *token.FileSet
	ast    *ast.File
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// moduleOf maps a repo-relative path to its module by the longest go.mod dir.
func moduleOf(rel string, mods map[string]string) string {
	best, bestLen := mods["."], -1
	for dir, mod := range mods {
		if dir == "." {
			continue
		}
		if (rel == dir || strings.HasPrefix(rel, dir+"/")) && len(dir) > bestLen {
			best, bestLen = mod, len(dir)
		}
	}
	return best
}

func loadModules(t *testing.T, root string) map[string]string {
	t.Helper()
	mods := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipDir(d.Name()) && p != root {
			return filepath.SkipDir
		}
		if d.Name() != "go.mod" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		mods[filepath.ToSlash(rel)] = modulePath(string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return mods
}

func modulePath(gomod string) string {
	for _, line := range strings.Split(gomod, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
			return f[1]
		}
	}
	return ""
}

func skipDir(name string) bool {
	switch name {
	case "testdata", "docsi", "dist", ".git", "vendor", "node_modules":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func loadFiles(t *testing.T) []srcFile {
	t.Helper()
	root := repoRoot(t)
	mods := loadModules(t, root)
	var out []srcFile
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", rel, err)
			return nil
		}
		out = append(out, srcFile{
			rel:    rel,
			module: moduleOf(filepath.ToSlash(filepath.Dir(rel)), mods),
			test:   strings.HasSuffix(p, "_test.go"),
			gen:    ast.IsGenerated(f),
			fset:   fset,
			ast:    f,
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no Go files found")
	}
	return out
}

func imports(f srcFile) []string {
	var out []string
	for _, im := range f.ast.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		out = append(out, p)
	}
	return out
}

func isStdlib(path string) bool {
	first := strings.SplitN(path, "/", 2)[0]
	return !strings.Contains(first, ".")
}

// vendorOf reports which vendor module an import path belongs to ("" = none).
func vendorOf(path string) string {
	for _, m := range []string{modState, modClaude, modCodex, modBuiltin, modExample} {
		if path == m || strings.HasPrefix(path, m+"/") {
			return m
		}
	}
	return ""
}

func pos(f srcFile, n ast.Node) string {
	p := f.fset.Position(n.Pos())
	return f.rel + ":" + strconv.Itoa(p.Line)
}

// Rule 1: the root module imports only stdlib and its own packages.
func TestRootImportsStdlibOnly(t *testing.T) {
	for _, f := range loadFiles(t) {
		if f.module != modulePrefix {
			continue
		}
		for _, im := range imports(f) {
			if isStdlib(im) {
				continue
			}
			if strings.HasPrefix(im, modulePrefix) && vendorOf(im) == "" {
				continue
			}
			t.Errorf("%s: root module imports %s", f.rel, im)
		}
	}
}

// Rules 2 and 3: vendor modules never import each other; only builtin (and
// the examples module that consumes it) imports more than one vendor;
// builtin never imports codex/state.
func TestVendorIsolation(t *testing.T) {
	forbidden := map[string][]string{
		modClaude:  {modCodex, modState, modBuiltin},
		modCodex:   {modClaude, modState, modBuiltin},
		modState:   {modClaude, modBuiltin},
		modBuiltin: {modState},
	}
	for _, f := range loadFiles(t) {
		for _, im := range imports(f) {
			v := vendorOf(im)
			for _, bad := range forbidden[f.module] {
				if v == bad {
					t.Errorf("%s: module %s imports %s", f.rel, f.module, im)
				}
			}
		}
	}
}

// Rule 4: each go.mod requires exactly the third-party modules of SPEC §3
// (direct requirements; indirect ones are what those pull).
func TestGoModRequires(t *testing.T) {
	allowed := map[string][]string{
		".":           {},
		"claude":      {},
		"codex":       {"github.com/pelletier/go-toml/v2"},
		"codex/state": {"modernc.org/sqlite"},
		"builtin":     {},
		"examples":    {},
	}
	root := repoRoot(t)
	for dir, want := range allowed {
		got := directRequires(t, filepath.Join(root, dir, "go.mod"))
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s/go.mod: third-party requires %v, want %v", dir, got, want)
		}
	}
}

func directRequires(t *testing.T, path string) []string {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	var out []string
	inBlock := false
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "require (":
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		}
		var mod string
		if inBlock {
			mod = line
		} else if strings.HasPrefix(line, "require ") {
			mod = strings.TrimPrefix(line, "require ")
		} else {
			continue
		}
		if strings.Contains(mod, "// indirect") || mod == "" {
			continue
		}
		name := strings.Fields(mod)[0]
		if !strings.HasPrefix(name, modulePrefix) {
			out = append(out, name)
		}
	}
	return out
}

// Rule 5: nothing reads the real machine outside HostFromOS's files.
func TestNoHiddenState(t *testing.T) {
	banned := map[string]map[string]bool{
		"os":   {"Getenv": true, "Environ": true, "UserHomeDir": true, "LookupEnv": true},
		"exec": {"Command": true, "CommandContext": true, "LookPath": true},
		"time": {"Now": true},
	}
	for _, f := range loadFiles(t) {
		if f.test || (f.module == modulePrefix && strings.HasPrefix(filepath.Base(f.rel), hostFilePrefix)) {
			continue
		}
		for _, im := range imports(f) {
			if im == "os/user" {
				t.Errorf("%s: imports os/user", f.rel)
			}
		}
		ast.Inspect(f.ast, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if ok && banned[x.Name][sel.Sel.Name] {
				t.Errorf("%s: %s.%s outside HostFromOS", pos(f, sel), x.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

// receiverName returns the base type name of a method receiver ("" for funcs).
func receiverName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	tx := fd.Recv.List[0].Type
	for {
		switch v := tx.(type) {
		case *ast.StarExpr:
			tx = v.X
		case *ast.IndexExpr:
			tx = v.X
		case *ast.IndexListExpr:
			tx = v.X
		case *ast.Ident:
			return v.Name
		default:
			return ""
		}
	}
}

var (
	runnerRecv = regexp.MustCompile(`(?i)runner|binaryversion`)
	proberRecv = regexp.MustCompile(`(?i)prober`)
	writerRecv = regexp.MustCompile(`(?i)writer|linker|mover`)
)

// writeFuncs are root/vendor functions that are the Write door themselves
// (HomeLinker, movers, the shared backup helper).
var writeFuncs = map[string]bool{
	"BackupFile": true, "ApplyHomeLinks": true, "ApplyMove": true, "MoveHistory": true,
	"ApplyMovePlan": true, "rollbackMove": true, "CopyTree": true,
}

// fsWrites are the os calls that change the file system.
var fsWrites = map[string]bool{
	"WriteFile": true, "Create": true, "Rename": true, "Symlink": true, "Link": true,
	"Remove": true, "RemoveAll": true, "Mkdir": true, "MkdirAll": true, "Chmod": true,
	"Chtimes": true, "Truncate": true, "CreateTemp": true, "MkdirTemp": true,
}

// Rule 6: Host.Exec only from Runner receivers; HTTP POST only from Prober;
// file writes only from Writer doors.
func TestDoors(t *testing.T) {
	for _, f := range loadFiles(t) {
		// agenttest builds fixture homes in temp dirs; it is test support, not a door.
		// go:generate tools (package main under internal/) write generated source, not vendor files.
		generator := f.ast.Name.Name == "main" && strings.Contains(f.rel, "/internal/")
		if f.test || generator || f.module == modExample || strings.HasPrefix(f.rel, "agenttest/") {
			continue
		}
		hostOS := f.module == modulePrefix && strings.HasPrefix(filepath.Base(f.rel), hostFilePrefix)
		for _, decl := range f.ast.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			recv := receiverName(fd)
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.SelectorExpr:
					if inner, ok := v.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Exec" && v.Sel.Name == "Run" {
						if !runnerRecv.MatchString(recv) && !runnerRecv.MatchString(fd.Name.Name) {
							t.Errorf("%s: Host.Exec.Run in %s.%s (not a Runner door)", pos(f, v), recv, fd.Name.Name)
						}
					}
					if x, ok := v.X.(*ast.Ident); ok && x.Name == "http" && v.Sel.Name == "MethodPost" && !proberRecv.MatchString(recv) {
						t.Errorf("%s: HTTP POST in %s.%s (not a Prober door)", pos(f, v), recv, fd.Name.Name)
					}
					if x, ok := v.X.(*ast.Ident); ok && x.Name == "os" && fsWrites[v.Sel.Name] && !hostOS {
						if !writerRecv.MatchString(recv) && !writeFuncs[fd.Name.Name] && !strings.HasPrefix(fd.Name.Name, "write") {
							t.Errorf("%s: os.%s in %s.%s (not a Writer door)", pos(f, v), v.Sel.Name, recv, fd.Name.Name)
						}
					}
				case *ast.BasicLit:
					if v.Kind == token.STRING && v.Value == `"POST"` && !proberRecv.MatchString(recv) {
						t.Errorf("%s: \"POST\" in %s.%s (not a Prober door)", pos(f, v), recv, fd.Name.Name)
					}
				}
				return true
			})
		}
	}
}

// Rule 7: no any / interface{} anywhere, tests included.
func TestNoAny(t *testing.T) {
	for _, f := range loadFiles(t) {
		ast.Inspect(f.ast, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.Ident:
				if v.Name == "any" && v.Obj == nil {
					t.Errorf("%s: uses any", pos(f, v))
				}
			case *ast.InterfaceType:
				if v.Methods == nil || len(v.Methods.List) == 0 {
					t.Errorf("%s: empty interface{}", pos(f, v))
				}
			}
			return true
		})
	}
}

// Rule 8: no package-level var except Err* sentinels and `var _ I = …`
// assertions; no exported Register-style function. Test files are exempt.
func TestNoPackageVars(t *testing.T) {
	for _, f := range loadFiles(t) {
		if f.test {
			continue
		}
		for _, decl := range f.ast.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						if name.Name == "_" || strings.HasPrefix(name.Name, "Err") || strings.HasPrefix(name.Name, "err") {
							continue
						}
						t.Errorf("%s: package-level var %s", pos(f, name), name.Name)
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil && strings.HasPrefix(d.Name.Name, "Register") {
					t.Errorf("%s: exported registry function %s", pos(f, d), d.Name.Name)
				}
			}
		}
	}
}

// Rule 9: every Capability constant appears in the Supported table and has an
// accessor (the constant is referenced at least once outside its declaration
// in a non-test root file).
func TestCapabilityTable(t *testing.T) {
	files := loadFiles(t)
	var caps []*ast.Ident
	uses := map[string]int{}
	for _, f := range files {
		if f.module != modulePrefix || f.test || f.ast.Name.Name != "agentcli" {
			continue
		}
		for _, decl := range f.ast.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			inCap := false
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				if id, ok := vs.Type.(*ast.Ident); ok {
					inCap = id.Name == "Capability"
				} else if vs.Type != nil {
					inCap = false
				}
				if inCap {
					caps = append(caps, vs.Names...)
				}
			}
		}
		ast.Inspect(f.ast, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				uses[id.Name]++
			}
			return true
		})
	}
	if len(caps) == 0 {
		t.Fatal("no Capability constants found in the root package")
	}
	for _, c := range caps {
		if c.Name == "_" || !c.IsExported() {
			continue
		}
		if uses[c.Name] < 2 {
			t.Errorf("Capability %s has no Supported-table row or accessor", c.Name)
		}
	}
}

// revealFiles are the only non-test files that may read a secret's value
// (SPEC §8): the root env builder, credential-blob parsing and the HTTP
// Authorization header of usage/probe clients.
var revealFiles = regexp.MustCompile(`^(command\.go|(claude|codex)/(credential|usage|auth)\.go)$`)

// TestRevealConfined keeps Token/EnvValue/Personal.Reveal() out of every
// other library file, so a secret cannot reach argv, errors or output.
func TestRevealConfined(t *testing.T) {
	for _, f := range loadFiles(t) {
		if f.test || f.module == modExample || revealFiles.MatchString(f.rel) {
			continue
		}
		ast.Inspect(f.ast, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Reveal" {
				t.Errorf("%s: Reveal() outside the allowed secret readers", pos(f, sel))
			}
			return true
		})
	}
}
