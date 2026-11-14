package claude

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanakia/agentcli"
)

// streamFixtureRoot holds one directory per Claude Code version, each with a
// redacted real `claude -p` run: stream-partial.jsonl (stream-json with
// --verbose --include-partial-messages) and result.json (--output-format json).
const streamFixtureRoot = "testdata/stream"

// streamFixtureVersions lists the recorded versions; every stream test runs
// over each of them.
func streamFixtureVersions(t *testing.T) []string {
	t.Helper()
	ents, err := os.ReadDir(streamFixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("no stream fixtures")
	}
	return out
}

func readStream(t *testing.T, path string) []StreamMessage {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	d := NewStreamDecoder(f)
	var msgs []StreamMessage
	for {
		m, err := d.Next()
		if errors.Is(err, io.EOF) {
			return msgs
		}
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, m)
	}
}

// TestStreamFixtures decodes every line of every recorded run through its
// typed accessor and checks the values a caller relies on.
func TestStreamFixtures(t *testing.T) {
	for _, v := range streamFixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			msgs := readStream(t, filepath.Join(streamFixtureRoot, v, "stream-partial.jsonl"))
			var text string
			seen := map[APIEventType]bool{}
			var gotInit, gotResult, gotRate bool
			for i, m := range msgs {
				if !m.Type.IsKnown() {
					t.Errorf("line %d: unknown type %q", i, m.Type)
				}
				if m.SessionID == "" {
					t.Errorf("line %d: no session id", i)
				}
				switch m.Type {
				case StreamTypeSystem:
					if !m.Subtype.IsKnown() {
						t.Errorf("line %d: unknown subtype %q", i, m.Subtype)
					}
					switch m.Subtype {
					case SubtypeInit:
						in, err := m.AsInit()
						if err != nil {
							t.Fatal(err)
						}
						gotInit = true
						if in.ClaudeCodeVersion != v || in.Model == "" || in.CWD == "" || len(in.Tools) == 0 || len(in.Capabilities) == 0 || !in.PermissionMode.IsKnown() {
							t.Errorf("init = %+v", in)
						}
						if in.Unknown != nil {
							t.Errorf("init unknown keys %v", in.Unknown.Keys())
						}
						if m.Neutral().Type != agentcli.RunEventStarted {
							t.Error("init is not Started")
						}
					case SubtypeStatus:
						s, err := m.AsStatus()
						if err != nil || s.Status == "" || s.Unknown != nil {
							t.Errorf("status = %+v, %v", s, err)
						}
					case SubtypeCommandsChanged:
						c, err := m.AsCommandsChanged()
						if err != nil || len(c.Commands) == 0 || c.Commands[0].Name == "" || c.Unknown != nil {
							t.Errorf("commands = %+v, %v", c, err)
						}
					}
				case StreamTypeEvent:
					e, err := m.AsEvent()
					if err != nil {
						t.Fatal(err)
					}
					if !e.Event.Type.IsKnown() || e.APIMessageID == "" {
						t.Errorf("event = %+v", e)
					}
					seen[e.Event.Type] = true
					switch e.Event.Type {
					case APIEventMessageStart:
						if e.TTFT == nil || *e.TTFT <= 0 || e.Event.Message == nil || e.Event.Message.Usage == nil || e.Event.Message.Model == "" {
							t.Errorf("message_start = %+v", e)
						}
					case APIEventContentBlockStart:
						if e.Event.Index == nil || e.Event.ContentBlock == nil || e.Event.ContentBlock.Type != BlockText {
							t.Errorf("content_block_start = %+v", e.Event)
						}
					case APIEventContentBlockDelta:
						if e.Event.Delta == nil || e.Event.Delta.Type != DeltaText {
							t.Errorf("delta = %+v", e.Event)
						}
						text += m.Neutral().Text
					case APIEventMessageDelta:
						if e.Event.Delta == nil || e.Event.Delta.StopReason == "" || e.Event.Usage == nil || e.Event.Usage.Output == 0 {
							t.Errorf("message_delta = %+v", e.Event)
						}
					}
					if e.Event.Unknown != nil && e.Event.Type != APIEventMessageDelta {
						t.Errorf("%s unknown keys %v", e.Event.Type, e.Event.Unknown.Keys())
					}
				case StreamTypeAssistant:
					a, err := m.AsAssistant()
					if err != nil {
						t.Fatal(err)
					}
					if a.Message.Content.Text() != "ok" || a.RequestID == "" || a.Timestamp.IsZero() || a.Message.Usage == nil || a.Unknown != nil {
						t.Errorf("assistant = %+v", a)
					}
					if ev := m.Neutral(); ev.Type != agentcli.RunEventMessage || ev.Text != "ok" {
						t.Errorf("assistant neutral = %+v", ev)
					}
				case StreamTypeRateLimit:
					r, err := m.AsRateLimit()
					if err != nil {
						t.Fatal(err)
					}
					gotRate = true
					w, ok := r.Info.UnifiedWindows[r.Info.RateLimitType]
					if r.Info.Status == "" || r.Info.ResetsAt.IsZero() || !ok || w.Utilization <= 0 || w.Utilization > 1 || r.Info.Unknown != nil || r.Unknown != nil {
						t.Errorf("rate limit = %+v", r)
					}
				case StreamTypeResult:
					gotResult = true
					r, err := m.AsResult()
					if err != nil {
						t.Fatal(err)
					}
					checkResult(t, r)
					if len(r.ModelUsageSinceLastResult) == 0 {
						t.Error("stream result has no model_usage_since_last_result")
					}
					ev := m.Neutral()
					if ev.Type != agentcli.RunEventDone || ev.Text != "ok" || ev.Tokens == nil || ev.Tokens.Output == 0 {
						t.Errorf("result neutral = %+v", ev)
					}
				}
			}
			if !gotInit || !gotResult || !gotRate {
				t.Errorf("init %v result %v rate %v", gotInit, gotResult, gotRate)
			}
			for _, et := range []APIEventType{APIEventMessageStart, APIEventContentBlockStart, APIEventContentBlockDelta, APIEventContentBlockStop, APIEventMessageDelta, APIEventMessageStop} {
				if !seen[et] {
					t.Errorf("no %s event", et)
				}
			}
			if text != "ok" {
				t.Errorf("partial text = %q", text)
			}

			f, err := os.Open(filepath.Join(streamFixtureRoot, v, "result.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			r, err := DecodeJSONResult(f)
			if err != nil {
				t.Fatal(err)
			}
			checkResult(t, r)
		})
	}
}

// checkResult asserts a recorded success result decoded every typed key.
func checkResult(t *testing.T, r StreamResult) {
	t.Helper()
	if r.Subtype != SubtypeSuccess || r.IsError || r.Result != "ok" || r.NumTurns != 1 || r.StopReason == "" || r.TerminalReason == "" {
		t.Errorf("result = %+v", r)
	}
	if r.Duration <= 0 || r.APIDuration <= 0 || r.TTFT <= 0 || r.Duration < time.Millisecond {
		t.Errorf("durations = %v %v %v", r.Duration, r.APIDuration, r.TTFT)
	}
	if r.TotalCostUSD <= 0 || r.Usage.Output == 0 || r.Usage.CacheRead == 0 || len(r.ModelUsage) != 1 || r.SubagentStats == nil || r.CredentialExpiresAt == nil || r.APIErrorStatus != nil {
		t.Errorf("result totals = %+v", r)
	}
	for model, mu := range r.ModelUsage {
		if mu.CostUSD <= 0 || mu.ContextWindow == 0 || mu.CanonicalModel != model {
			t.Errorf("model usage %s = %+v", model, mu)
		}
	}
	if r.Unknown != nil {
		t.Errorf("result unknown keys %v", r.Unknown.Keys())
	}
	// usage carries keys MessageUsage does not model; they are kept.
	if _, ok := r.Usage.Unknown["server_tool_use"]; !ok {
		t.Errorf("usage unknown keys = %v", r.Usage.Unknown.Keys())
	}
}

// TestStreamUnknownTypeIsRaw: a line type this version does not know is not
// an error; it keeps its type and Raw and maps to Other.
func TestStreamUnknownTypeIsRaw(t *testing.T) {
	line := `{"type":"brand_new","session_id":"s","x":1}`
	msgs := readStreamString(t, line)
	m := msgs[0]
	if m.Type != "brand_new" || m.Type.IsKnown() || string(m.Raw) != line {
		t.Fatalf("msg = %+v", m)
	}
	if m.Neutral().Type != agentcli.RunEventOther {
		t.Fatal("unknown type is not Other")
	}
	if _, err := m.AsResult(); !errors.Is(err, ErrWrongStreamMessage) {
		t.Fatalf("AsResult on unknown: %v", err)
	}
	// An error subtype maps to Error even when is_error is absent.
	r := readStreamString(t, `{"type":"result","subtype":"error_max_turns","session_id":"s"}`)[0]
	if !r.Subtype.IsResultError() || r.Neutral().Type != agentcli.RunEventError {
		t.Fatalf("error result = %+v", r.Neutral())
	}
	// A new key on a typed line lands in Unknown.
	in, err := readStreamString(t, `{"type":"system","subtype":"init","session_id":"s","new_key":true}`)[0].AsInit()
	if err != nil || len(in.Unknown) != 1 || in.Unknown["new_key"] == nil {
		t.Fatalf("init unknown = %v, %v", in.Unknown, err)
	}
}

func readStreamString(t *testing.T, s string) []StreamMessage {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(s+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return readStream(t, p)
}
