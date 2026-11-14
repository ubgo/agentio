package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testRecording is the recorded run relative to this package dir.
const testRecording = "../" + defaultRecording

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testRecording); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"init: Claude Code 2.1.325, model claude-haiku-5-5, mode bypassPermissions, 5 tools, 2 MCP servers, 3 slash commands",
		"  AsResult on init: claude: wrong stream message type: system/init",
		"stream_event content_block_delta",
		`assistant: model claude-haiku-5-5, text "ok"`,
		"rate limit: allowed, binding seven_day, window seven_day at 52%",
		"result: success, error false, 1 turns, $0.0045, 1.279s wall, stop end_turn",
		"api_retry: attempt 1 of 10 after 500ms, HTTP 529 (overloaded)",
		"  usage: 2 in, 11370 cache read, 17940 cache write, 4 out; API 1.199s, TTFT 1.328s",
		`neutral done: "ok", 4 output tokens`,
		"  message events: 7",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
