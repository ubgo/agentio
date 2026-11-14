package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"error: agentio: binary not found on PATH: claude\n  -> install the CLI first\n",
		"error: agentio: unsupported: toy: prompt: not described by the agent spec\n  -> not supported by this agent\n",
		"error: claude: credentials: agentio: login expired\n  -> run the vendor CLI once to refresh its login\n",
		`429 retry-after="120": agentio: rate-limited; try again shortly (retry after 2m0s) (rate-limited=true)` + "\n",
		"retry after 1m30s",
		"opt: verbose set=false or(true)=true\n",
		"opt: retries set=false (null reads as unset)\n",
		`opt: json={"theme":"dark","verbose":false}` + "\n",
		"unknown: keys=[fastMode theme]\n",
		`unknown: theme raw={"name":"dark"}` + "\n",
		"warning: type-changed: settings.json: cleanupPeriodDays: expected a number\n",
		`parse "v1.2": ok=true version="1.2.0" major=1 pre=""` + "\n",
		`parse "x86_64 build 7": ok=true version="7.0.0"`,
		`parse "no version here": ok=false`,
		"sorted: [0.162.1 1.0.0-alpha 1.0.0-beta.2 1.0.0-beta.11 1.0.0]\n",
		`json autonomy yolo: agentio: unknown autonomy "yolo"` + "\n",
		"out of range: autonomy(42)\n",
		"install method mise: known=false\n",
		"autonomy values: [default read-only ask accept-edits full]\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
