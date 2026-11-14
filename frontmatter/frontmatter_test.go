package frontmatter

import (
	"slices"
	"testing"
)

// skillSample is a redacted SKILL.md shape (names and text replaced).
const skillSample = "---\n" +
	"name: sample-skill\n" +
	"description: Does a thing when the user asks for it,\n" +
	"  continued on a second line. Use when: \"x\" happens.\n" +
	"allowed-tools: Read, Grep, Bash(git:*)\n" +
	"tags: [one, \"two, three\", 'four']\n" +
	"disable-model-invocation: true\n" +
	"model: 'opus'\n" +
	"# a comment\n" +
	"metadata:\n" +
	"  version: \"1.2\"\n" +
	"  owner: someone # trailing comment\n" +
	"tools:\n" +
	"  - Read\n" +
	"  - \"Write\"\n" +
	"notes: |\n" +
	"  line one\n" +
	"    indented\n" +
	"\n" +
	"  line three\n" +
	"folded: >-\n" +
	"  a\n" +
	"  b\n" +
	"url: https://example.com/x\n" +
	"empty:\n" +
	"---\n" +
	"# Body\n"

func TestParse_SkillSubset(t *testing.T) {
	t.Parallel()
	d, body, err := Parse([]byte(skillSample))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# Body\n" {
		t.Errorf("body = %q", body)
	}
	checks := map[string]string{
		"name":        "sample-skill",
		"description": "Does a thing when the user asks for it, continued on a second line. Use when: \"x\" happens.",
		"model":       "opus",
		"notes":       "line one\n  indented\n\nline three\n",
		"folded":      "a b",
		"url":         "https://example.com/x",
		"empty":       "",
	}
	for k, want := range checks {
		if got := d.String(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got := d.List("allowed-tools"); !slices.Equal(got, []string{"Read", "Grep", "Bash(git:*)"}) {
		t.Errorf("allowed-tools = %q", got)
	}
	if got := d.List("tags"); !slices.Equal(got, []string{"one", "two, three", "four"}) {
		t.Errorf("tags = %q", got)
	}
	if got := d.List("tools"); !slices.Equal(got, []string{"Read", "Write"}) {
		t.Errorf("tools = %q", got)
	}
	if v, ok := d.Bool("disable-model-invocation"); !v || !ok {
		t.Error("bool")
	}
	m := d.Map("metadata")
	if m.String("version") != "1.2" || m.String("owner") != "someone" {
		t.Errorf("metadata = %+v", m)
	}
	if !slices.Equal(d.Keys()[:3], []string{"name", "description", "allowed-tools"}) {
		t.Errorf("keys = %v", d.Keys())
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"# just markdown\n", "---\nname: x\n(no close)", ""} {
		d, body, err := Parse([]byte(in))
		if err != nil || len(d.Fields) != 0 || string(body) != in {
			t.Errorf("%q: %+v %q %v", in, d, body, err)
		}
	}
	d, _, err := Parse([]byte("\ufeff---\r\nname: crlf\r\n---\r\nbody"))
	if err != nil || d.String("name") != "crlf" {
		t.Errorf("bom/crlf: %q %v", d.String("name"), err)
	}
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()
	if _, err := ParseYAML([]byte("just text\n")); err == nil {
		t.Error("a line without key must be an error")
	}
}
