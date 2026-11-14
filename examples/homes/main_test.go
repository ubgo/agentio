package main

import (
	"bytes"
	"strings"
	"testing"
)

// testUserHome is the fixture user home relative to this package dir.
const testUserHome = "../../claude/testdata/homes/2.1.325/userhome"

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testUserHome); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"default: dir=~/.claude\n",
		"default: path=~/.claude/settings.json\n",
		"zero home: IsZero=true\n",
		"homes: claude:~/.claude name=personal default=true\n",
		"homes: claude:~/.claude-work name=work default=false\n",
		"from env: ok=true name=work dir=~/.claude-work\n",
		"is home: project=false\n",
		"is home: missing=false\n",
		"home at /srv/agents/.claude-ci: name=ci default=false dir=/srv/agents/.claude-ci\n",
		"home at .claude: name=personal default=true dir=~/.claude\n",
		"core home at .toy-staging: toy:~/.toy-staging name=staging\n",
		"check owner: foreign=true err=agentio: home belongs to another agent: codex home passed to claude\n",
		"check owner: own home err=<nil>\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
