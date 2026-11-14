package docscheck

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// update rewrites markdown snippet blocks from their Go source instead of
// failing on a difference (task docs:snippets).
var update = flag.Bool("update", false, "rewrite markdown snippet blocks from source")

// snippetRef matches the markdown marker line: <!-- snippet: path#NAME -->.
var snippetRef = regexp.MustCompile(`^\s*<!--\s*snippet:\s*([^\s#]+)(?:#(\S+))?\s*-->\s*$`)

// regionMarker matches a source marker line: // snippet:start NAME.
var regionMarker = regexp.MustCompile(`^\s*//\s*snippet:(start|end)\s+(\S+)\s*$`)

const (
	markerStart = "start"
	markerEnd   = "end"
)

// marker is one region marker found in a source file.
type marker struct {
	kind string
	name string
	line int // 1-based
}

// markerProblems reports unmatched, duplicated or misordered markers.
func markerProblems(lines []string) []string {
	var probs []string
	open := map[string]int{}
	seen := map[string]bool{}
	for i, l := range lines {
		m := parseMarker(l, i+1)
		if m == nil {
			continue
		}
		switch m.kind {
		case markerStart:
			if _, ok := open[m.name]; ok {
				probs = append(probs, fmt.Sprintf("%d: snippet %q started again before its end", m.line, m.name))
			} else if seen[m.name] {
				probs = append(probs, fmt.Sprintf("%d: snippet %q defined twice", m.line, m.name))
			}
			open[m.name] = m.line
			seen[m.name] = true
		case markerEnd:
			if _, ok := open[m.name]; !ok {
				probs = append(probs, fmt.Sprintf("%d: snippet:end %q without a start", m.line, m.name))
			}
			delete(open, m.name)
		}
	}
	for name, line := range open {
		probs = append(probs, fmt.Sprintf("%d: snippet:start %q without an end", line, name))
	}
	return probs
}

func parseMarker(line string, n int) *marker {
	sm := regionMarker.FindStringSubmatch(line)
	if sm == nil {
		return nil
	}
	return &marker{kind: sm[1], name: sm[2], line: n}
}

// extractRegion returns the lines of region name (marker lines of any region
// removed), or the whole file without marker lines when name is "".
func extractRegion(lines []string, name string) ([]string, error) {
	var out []string
	in, found := name == "", name == ""
	for i, l := range lines {
		m := parseMarker(l, i+1)
		if m != nil {
			if m.name == name && m.kind == markerStart {
				in, found = true, true
			}
			if m.name == name && m.kind == markerEnd {
				in = false
			}
			continue
		}
		if in {
			out = append(out, l)
		}
	}
	if !found {
		return nil, fmt.Errorf("region %q not found", name)
	}
	return out, nil
}

// normalize trims leading/trailing blank lines and trailing spaces, then
// removes the leading whitespace common to every non-blank line.
func normalize(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.TrimRight(l, " \t"))
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	prefix := ""
	first := true
	for _, l := range out {
		if l == "" {
			continue
		}
		ws := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if first {
			prefix, first = ws, false
			continue
		}
		for !strings.HasPrefix(ws, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	for i, l := range out {
		out[i] = strings.TrimPrefix(l, prefix)
	}
	return out
}

// snippetBlock is one marker + fenced block found in a markdown file.
type snippetBlock struct {
	refLine    int // 0-based index of the marker line
	path, name string
	open, end  int // 0-based indexes of the opening and closing fence lines
	indent     string
}

func TestSnippets(t *testing.T) {
	root := repoRoot(t)
	var goFiles []string
	for _, f := range candidateFiles(t, root) {
		if isCheckedGo(f) {
			goFiles = append(goFiles, f)
		}
	}
	probs, rewritten, err := syncSnippets(root, goFiles, publicMarkdown(t, root), *update)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range probs {
		t.Error(p)
	}
	for _, md := range rewritten {
		t.Logf("%s: snippet blocks rewritten from source", md)
	}
}

// syncSnippets checks marker balance in goFiles and in every referenced
// source, then compares each markdown snippet block with its source region.
// With write set, differing blocks are rewritten from source instead of
// reported. Problems read "file:line: reason"; rewritten lists changed files.
func syncSnippets(root string, goFiles, mdFiles []string, write bool) (probs, rewritten []string, err error) {
	sources := map[string][]string{} // nil value: file missing
	source := func(rel string) []string {
		if lines, ok := sources[rel]; ok {
			return lines
		}
		b, rerr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if rerr != nil {
			sources[rel] = nil
			return nil
		}
		lines := splitLines(string(b))
		if lines == nil {
			lines = []string{}
		}
		for _, p := range markerProblems(lines) {
			probs = append(probs, rel+":"+p)
		}
		sources[rel] = lines
		return lines
	}
	for _, f := range goFiles {
		source(f)
	}
	for _, md := range mdFiles {
		b, rerr := os.ReadFile(filepath.Join(root, filepath.FromSlash(md)))
		if rerr != nil {
			return nil, nil, rerr
		}
		lines := splitLines(string(b))
		blocks, bprobs := findSnippetBlocks(lines)
		for _, p := range bprobs {
			probs = append(probs, md+":"+p)
		}
		changed := false
		// Rewrite from the last block back so earlier indexes stay valid.
		for i := len(blocks) - 1; i >= 0; i-- {
			blk := blocks[i]
			where := fmt.Sprintf("%s:%d", md, blk.refLine+1)
			src := path.Clean(strings.TrimPrefix(blk.path, "/"))
			var srcLines []string
			if src != ".." && !strings.HasPrefix(src, "../") {
				srcLines = source(src)
			}
			if srcLines == nil {
				probs = append(probs, fmt.Sprintf("%s: snippet source %s not found in the repo", where, blk.path))
				continue
			}
			region, xerr := extractRegion(srcLines, blk.name)
			if xerr != nil {
				probs = append(probs, fmt.Sprintf("%s: snippet %s: %v", where, blk.path, xerr))
				continue
			}
			want := normalize(region)
			got := normalize(lines[blk.open+1 : blk.end])
			if strings.Join(want, "\n") == strings.Join(got, "\n") {
				continue
			}
			if !write {
				probs = append(probs, fmt.Sprintf("%s: block differs from %s%s (first difference at block line %d); run task docs:snippets", where, blk.path, hashName(blk.name), firstDiff(want, got)))
				continue
			}
			body := make([]string, len(want))
			for j, l := range want {
				if l != "" {
					l = blk.indent + l
				}
				body[j] = l
			}
			lines = append(lines[:blk.open+1], append(body, lines[blk.end:]...)...)
			changed = true
		}
		if changed {
			p := filepath.Join(root, filepath.FromSlash(md))
			if werr := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), mdFileMode); werr != nil {
				return nil, nil, werr
			}
			rewritten = append(rewritten, md)
		}
	}
	return probs, rewritten, nil
}

// mdFileMode is the permission for rewritten markdown (the usual 0644).
const mdFileMode = 0o644

func hashName(name string) string {
	if name == "" {
		return ""
	}
	return "#" + name
}

// firstDiff returns the 1-based line of the first difference between a and b.
func firstDiff(a, b []string) int {
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return i + 1
		}
	}
	return len(a) + 1
}

// findSnippetBlocks pairs every snippet marker with the fenced block on the
// next line. A marker not directly followed by a fence, or an unclosed fence,
// is reported as a problem ("line: reason").
func findSnippetBlocks(lines []string) ([]snippetBlock, []string) {
	var blocks []snippetBlock
	var probs []string
	fenced := fencedLines(lines)
	for i, l := range lines {
		if fenced[i] {
			continue
		}
		sm := snippetRef.FindStringSubmatch(l)
		if sm == nil {
			continue
		}
		if i+1 >= len(lines) {
			probs = append(probs, fmt.Sprintf("%d: snippet marker not followed by a fenced code block", i+1))
			continue
		}
		f, ok := openFence(lines[i+1])
		if !ok {
			probs = append(probs, fmt.Sprintf("%d: snippet marker not followed by a fenced code block", i+1))
			continue
		}
		end := -1
		for j := i + 2; j < len(lines); j++ {
			if f.closes(lines[j]) {
				end = j
				break
			}
		}
		if end < 0 {
			probs = append(probs, fmt.Sprintf("%d: snippet code block is never closed", i+2))
			continue
		}
		blocks = append(blocks, snippetBlock{
			refLine: i, path: sm[1], name: sm[2], open: i + 1, end: end,
			indent: strings.Repeat(" ", f.indent),
		})
	}
	return blocks, probs
}
