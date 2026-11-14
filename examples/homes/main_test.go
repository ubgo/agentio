package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testUserHome is the fixture user home relative to this package dir.
const testUserHome = "../../claude/testdata/homes/2.1.325/userhome"

func TestRun(t *testing.T) {
	// HomeAt makes a rooted dir absolute the way the OS does: unchanged on
	// POSIX; on Windows on the user home's drive (D:\srv\agents\...).
	abs, err := filepath.Abs(testUserHome)
	if err != nil {
		t.Fatal(err)
	}
	ciDir := filepath.VolumeName(abs) + filepath.FromSlash(ciHomeDir)
	var out bytes.Buffer
	if err := run(&out, testUserHome); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"default: dir=~/.claude\n",
		"default: path=~/.claude/settings.json\n",
		"zero home: IsZero=true\n",
		"homes: claude:~/.claude name=personal default=true\n",
		"homes: claude:~/.claude-work name=work default=false\n",
		"from env: ok=true name=work dir=~/.claude-work\n",
		"is home: project=false\n",
		"is home: missing=false\n",
		"home at " + ciHomeDir + ": name=ci default=false dir=" + ciDir + "\n",
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
