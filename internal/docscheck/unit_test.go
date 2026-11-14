package docscheck

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Step 1 — Open", "step-1--open"},
		{"Before you start", "before-you-start"},
		{"Try it", "try-it"},
		{"`agentio.UsageOf` and friends", "agentiousageof-and-friends"},
		{"What's new?", "whats-new"},
		{"snake_case_name", "snake_case_name"},
		{"A - B", "a---b"},
		{"Host (the machine)", "host-the-machine"},
		{"[Link text](other.md) here", "link-text-here"},
		{"**Bold** and _emphasis_", "bold-and-emphasis"},
		{"Ünïcode Wörds", "ünïcode-wörds"},
		{"v1.2.3: release", "v123-release"},
		{"C++ / Go", "c--go"},
		{"Homes & targets", "homes--targets"},
		{"<code>Spec</code> fields", "spec-fields"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSluggerDuplicates(t *testing.T) {
	var s slugger
	got := []string{s.slug("Pitfalls"), s.slug("Pitfalls"), s.slug("Pitfalls-1"), s.slug("Pitfalls")}
	// The third heading is literally "Pitfalls-1" but that slug is taken, so
	// github-slugger gives it "pitfalls-1-1"; the fourth "Pitfalls" then skips to -2.
	want := []string{"pitfalls", "pitfalls-1", "pitfalls-1-1", "pitfalls-2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("slugs = %v, want %v", got, want)
	}
}

func TestAnchorsOf(t *testing.T) {
	doc := splitLines(strings.Join([]string{
		"---",
		"title: front matter is not a heading",
		"---",
		"# Title #",
		"## Step 1 — Open",
		"```md",
		"# not a heading",
		"```",
		"Setext heading",
		"==============",
		`<a id="custom"></a>`,
		`<a name="legacy">x</a>`,
		"## Title",
	}, "\n"))
	got := anchorsOf(doc)
	for _, want := range []string{"title", "step-1--open", "setext-heading", "custom", "legacy", "title-1"} {
		if !got[want] {
			t.Errorf("anchor %q missing from %v", want, got)
		}
	}
	for _, bad := range []string{"not-a-heading", "title-front-matter-is-not-a-heading"} {
		if got[bad] {
			t.Errorf("anchor %q should not exist", bad)
		}
	}
}

func TestLinkTargets(t *testing.T) {
	doc := splitLines(strings.Join([]string{
		"See [a](a.md) and ![img](assets/x.svg \"title\") and [b](b.md#sec).",
		"Inline `[not](code.md)` is ignored, [c](<with space.md>) is not.",
		"```",
		"[fenced](fenced.md)",
		"```",
		`<img src="assets/y.png" width="10"> <a href="#top">top</a>`,
		"[![badge](badge.svg)](https://example.com)",
		"[ref]: docs/ref.md",
	}, "\n"))
	var got []string
	for _, lt := range linkTargets(doc) {
		got = append(got, lt[0]+" "+lt[1])
	}
	want := []string{
		"1 a.md", "1 assets/x.svg", "1 b.md#sec",
		"2 with space.md",
		"6 assets/y.png", "6 #top",
		"7 badge.svg", "7 https://example.com",
		"8 docs/ref.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("targets =\n%v\nwant\n%v", got, want)
	}
}

func TestHardWraps(t *testing.T) {
	cases := []struct {
		name string
		doc  []string
		want []int // 0-based lines reported
	}{
		{"one line per paragraph", []string{"First paragraph.", "", "Second paragraph."}, nil},
		{"wrapped paragraph", []string{"This sentence was", "wrapped at 80 columns", "twice."}, []int{0, 1}},
		{"wrapped bullet lazy", []string{"- a bullet that", "continues here"}, []int{0}},
		{"wrapped bullet indented", []string{"- a bullet that", "  continues here"}, []int{0}},
		{"bullets", []string{"- one", "- two", "  - nested", "1. ordered", "2) ordered"}, nil},
		{"heading then text", []string{"## Title", "Body text."}, nil},
		{"bold label then text", []string{"**Question?**", "Answer."}, []int{0}},
		{"table", []string{"| a | b |", "|---|---|", "| 1 | 2 |"}, nil},
		{"text then table", []string{"Intro:", "| a | b |"}, nil},
		{"fenced code", []string{"```go", "line one", "line two", "```"}, nil},
		{"tilde fence in list", []string{"- step", "", "  ~~~", "  a", "  b", "  ~~~"}, nil},
		{"indented code", []string{"Para.", "", "    code one", "    code two", "", "    code three"}, nil},
		{"lazy indented continuation", []string{"Para starts", "    and continues"}, []int{0}},
		{"html block", []string{`<p align="center">`, "  some text", "  more text", "</p>"}, nil},
		{"blockquote", []string{"> quoted", "> still quoted"}, nil},
		{"setext heading", []string{"Title", "=====", "Text."}, nil},
		{"thematic break", []string{"Text.", "***", "More."}, nil},
		{"reference definitions", []string{"[a]: a.md", "[b]: b.md"}, nil},
		{"snippet marker then fence", []string{"<!-- snippet: examples/x/main.go#run -->", "```go", "x", "```"}, nil},
		{"front matter", []string{"---", "name: Bug", "about: thing", "---", "", "Body."}, nil},
		{"hard break still a wrap", []string{"Line with break  ", "next"}, []int{0}},
		{"text after fence close", []string{"```", "x", "```", "after", "again"}, []int{3}},
	}
	for _, c := range cases {
		got := hardWraps(c.doc)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: hardWraps = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMarkerProblems(t *testing.T) {
	src := splitLines(strings.Join([]string{
		"// snippet:start a",
		"// snippet:end b",
		"	// snippet:start c",
		"	// snippet:end c",
		"// snippet:start c",
		"// snippet:end c",
		"// snippet:start d",
		"// snippet:start d",
	}, "\n"))
	got := markerProblems(src)
	for _, want := range []string{`2: snippet:end "b" without a start`, `5: snippet "c" defined twice`, `8: snippet "d" started again`, `1: snippet:start "a" without an end`} {
		found := false
		for _, g := range got {
			if strings.HasPrefix(g, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing problem %q in %q", want, got)
		}
	}
}

func TestExtractRegionAndNormalize(t *testing.T) {
	src := splitLines(strings.Join([]string{
		"package main",
		"",
		"func main() {",
		"	// snippet:start body",
		"	x := 1",
		"	// snippet:start inner",
		"	if x > 0 {",
		"		println(x)",
		"	}",
		"	// snippet:end inner",
		"	// snippet:end body",
		"}",
	}, "\n"))
	body, err := extractRegion(src, "body")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"x := 1", "if x > 0 {", "\tprintln(x)", "}"}
	if got := normalize(body); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %q, want %q", got, want)
	}
	whole, _ := extractRegion(src, "")
	if strings.Contains(strings.Join(whole, "\n"), "snippet:") || len(whole) != len(src)-4 {
		t.Errorf("whole file keeps marker lines: %q", whole)
	}
	if _, err := extractRegion(src, "missing"); err == nil {
		t.Error("missing region: want error")
	}
	// A markdown block indented with spaces matches tab-indented source.
	md := []string{"", "    x := 1", "    if x > 0 {", "    \tprintln(x)", "    }", ""}
	if got := normalize(md); !reflect.DeepEqual(got, want) {
		t.Errorf("md block = %q, want %q", got, want)
	}
}

func TestFindSnippetBlocks(t *testing.T) {
	doc := []string{
		"<!-- snippet: examples/a/main.go#run -->",
		"```go",
		"code",
		"```",
		"<!-- snippet: examples/b/main.go -->",
		"text, not a fence",
		"```",
		"<!-- snippet: inside/fence.go -->",
		"```",
	}
	blocks, probs := findSnippetBlocks(doc)
	if len(blocks) != 1 || blocks[0].path != "examples/a/main.go" || blocks[0].name != "run" || blocks[0].open != 1 || blocks[0].end != 3 {
		t.Errorf("blocks = %+v", blocks)
	}
	if len(probs) != 1 || !strings.HasPrefix(probs[0], "5: snippet marker not followed") {
		t.Errorf("probs = %q", probs)
	}
}

func TestStripInlineCode(t *testing.T) {
	cases := map[string]string{
		"a `b` c":           "a  c",
		"a ``b ` c`` d":     "a  d",
		"unclosed `tick":    "unclosed `tick",
		"[x](y) `[z](w)` v": "[x](y)  v",
	}
	for in, want := range cases {
		if got := stripInlineCode(in); got != want {
			t.Errorf("stripInlineCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPublicSelection(t *testing.T) {
	cases := map[string]bool{
		"README.md": true, "CHANGELOG.md": true, "CODE_OF_CONDUCT.md": true,
		".github/ISSUE_TEMPLATE/bug_report.md": true, "docs/core/host.md": true,
		"examples/drift/README.md": true, "examples/drift/NOTES.md": false,
		"claude/README.md": false,
	}
	for in, want := range cases {
		if got := isPublicMarkdown(in); got != want {
			t.Errorf("isPublicMarkdown(%q) = %v, want %v", in, got, want)
		}
	}
	if !hasSkippedSegment("docs/testdata/x.md") || !hasSkippedSegment("docsi/SPEC.md") || hasSkippedSegment("docs/x.md") {
		t.Error("hasSkippedSegment")
	}
}

func TestPhrases(t *testing.T) {
	if p := firstPhrase("Co-Authored-By: someone", attributionPhrases, true); p != "co-authored-by" {
		t.Errorf("got %q", p)
	}
	for _, ok := range []string{"agentio reads Claude Code and Codex homes", "Anthropic's usage endpoint returns 429", "the generated command line"} {
		if p := firstPhrase(ok, attributionPhrases, true); p != "" {
			t.Errorf("%q flagged as %q", ok, p)
		}
	}
	if p := firstPhrase("cd /home/user/work", localPathMarkers, false); p != "" {
		t.Errorf("placeholder home flagged as %q", p)
	}
}

func TestSyncSnippetsUpdate(t *testing.T) {
	root := t.TempDir()
	src := "package main\n\nfunc main() {\n\t// snippet:start body\n\tprintln(\"new\")\n\t// snippet:end body\n}\n"
	md := "# Doc\n\n- Step:\n\n  <!-- snippet: examples/x/main.go#body -->\n  ```go\n  println(\"old\")\n  ```\n\n<!-- snippet: examples/x/main.go -->\n```go\n```\n\n<!-- snippet: examples/x/gone.go#body -->\n```go\n```\n"
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("examples/x/main.go", src)
	write("docs/a.md", md)
	goFiles, mdFiles := []string{"examples/x/main.go"}, []string{"docs/a.md"}

	probs, rewritten, err := syncSnippets(root, goFiles, mdFiles, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(probs) != 3 || len(rewritten) != 0 {
		t.Fatalf("check: probs %q rewritten %q", probs, rewritten)
	}
	if !strings.HasPrefix(probs[0], "docs/a.md:14: snippet source examples/x/gone.go not found") {
		t.Errorf("probs[0] = %q", probs[0])
	}

	probs, rewritten, err = syncSnippets(root, goFiles, mdFiles, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(probs) != 1 || !reflect.DeepEqual(rewritten, []string{"docs/a.md"}) {
		t.Fatalf("update: probs %q rewritten %q", probs, rewritten)
	}
	b, err := os.ReadFile(filepath.Join(root, "docs", "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		"  ```go\n  println(\"new\")\n  ```\n",
		"```go\npackage main\n\nfunc main() {\n\tprintln(\"new\")\n}\n```\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rewritten doc lacks %q:\n%s", want, got)
		}
	}
	// A second check is clean apart from the missing source.
	probs, _, _ = syncSnippets(root, goFiles, mdFiles, false)
	if len(probs) != 1 {
		t.Errorf("after update: %q", probs)
	}
}
