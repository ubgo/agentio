package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testUserHome is the fixture user home relative to this package dir.
const testUserHome = "../../claude/testdata/homes/2.1.325/userhome"

func TestRun(t *testing.T) {
	exampletest.RequirePOSIXLayout(t)
	var out bytes.Buffer
	if err := run(&out, testUserHome); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"host: user=user PATH set=true\n",
		`host: MODE=b EMPTY="" set=true MISSING set=false` + "\n",
		"install: path=$TMP/bin/claude\n",
		"install: real=$TMP/lib/node_modules/@anthropic-ai/claude-code/cli.js\n",
		"install: method=npm-global version=2.1.325\n",
		"install: shadowed=$TMP/old/bin/claude\n",
		"home: name=personal default=true dir=~/.claude\n",
		"home: name=work default=false dir=~/.claude-work\n",
		`neutral: id=claude name="Claude Code" binary=claude` + "\n",
		"lookpath: hit[1]=$TMP/old/bin/claude\n",
		"empty PATH: agentio: binary not found on PATH: claude\n",
		"agent: 2 homes on disk\n",
		"spec: markers=[settings.json projects .credentials.json .claude.json]\n",
		`spec: process exe="/Applications/Claude.app/Contents/MacOS/claude" argv=[] matches=false` + "\n",
		"core: launch toy --resume abc --model fast hello\n",
		"core: env TOY_HOME=~/.toy-work\n",
		"core: headless unsupported=true\n",
		"core: login toy login\n",
		"core: check binary-found fail: toy not found on PATH\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
