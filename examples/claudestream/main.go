// Command claudestream decodes the output of a headless Claude Code run:
// the line-by-line `--output-format stream-json` stream (with
// --include-partial-messages), the single document of `--output-format
// json`, and the same stream as neutral agentio run events.
//
// It reads the run recorded from Claude Code 2.1.325 in
// claude/testdata/stream/2.1.325. Nothing is launched.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultRecording is the recorded run, relative to the examples module.
const defaultRecording = "../claude/testdata/stream/2.1.325"

// Recorded file names.
const (
	streamFile = "stream-partial.jsonl"
	resultFile = "result.json"
)

// apiRetryLine is a system/api_retry message in the documented shape (no
// retry happened in the recorded run).
const apiRetryLine = `{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"retry_delay_ms":500,"error_status":529,"error":"overloaded","uuid":"u","session_id":"s"}`

// textPreview caps printed text.
const textPreview = 30

func main() {
	dir := flag.String("recording", defaultRecording, "directory with the recorded stream-json run")
	flag.Parse()
	if err := run(os.Stdout, *dir); err != nil {
		fmt.Fprintln(os.Stderr, "claudestream:", err)
		os.Exit(1)
	}
}

// run decodes the recorded stream and result files in dir three ways:
// typed stream events, the single json result, and neutral run events.
// Disk · Read only.
func run(w io.Writer, dir string) error {
	f, err := os.Open(filepath.Join(dir, streamFile))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := decodeStream(w, f); err != nil {
		return err
	}
	if err := decodeAPIRetry(w); err != nil {
		return err
	}
	r, err := os.Open(filepath.Join(dir, resultFile))
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := decodeJSON(w, r); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return neutralEvents(w, f)
}

// preview shortens text for one output line.
func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > textPreview {
		return string(r[:textPreview]) + "…"
	}
	return s
}

// snippet:start stream
// decodeStream reads stream-json lines one at a time. The envelope
// (Type, Subtype, SessionID) is always decoded; the typed body comes from
// the matching AsX accessor. Unknown types are not errors: Raw keeps them.
func decodeStream(w io.Writer, r io.Reader) error {
	d := claude.NewStreamDecoder(r)
	var partial strings.Builder
	for {
		m, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch m.Type {
		case claude.StreamTypeSystem:
			if m.Subtype != claude.SubtypeInit {
				fmt.Fprintf(w, "system/%s\n", m.Subtype)
				continue
			}
			in, err := m.AsInit()
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "init: Claude Code %s, model %s, mode %s, %d tools, %d MCP servers, %d slash commands\n",
				in.ClaudeCodeVersion, in.Model, in.PermissionMode, len(in.Tools), len(in.MCPServers), len(in.SlashCommands))
			if _, err := m.AsResult(); errors.Is(err, claude.ErrWrongStreamMessage) {
				fmt.Fprintln(w, "  AsResult on init:", err)
			}
		case claude.StreamTypeEvent:
			ev, err := m.AsEvent()
			if err != nil {
				return err
			}
			if ev.Event.Delta != nil && ev.Event.Delta.Type == claude.DeltaText {
				partial.WriteString(ev.Event.Delta.Text)
			}
			fmt.Fprintf(w, "stream_event %s\n", ev.Event.Type)
		case claude.StreamTypeAssistant:
			a, err := m.AsAssistant()
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "assistant: model %s, text %q\n", a.Message.Model, preview(a.Message.Content.Text()))
		case claude.StreamTypeRateLimit:
			rl, err := m.AsRateLimit()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(rl.Info.UnifiedWindows))
			for k := range rl.Info.UnifiedWindows {
				names = append(names, k)
			}
			slices.Sort(names)
			for _, k := range names {
				win := rl.Info.UnifiedWindows[k]
				fmt.Fprintf(w, "rate limit: %s, binding %s, window %s at %.0f%%\n", rl.Info.Status, rl.Info.RateLimitType, k, win.Utilization*100)
			}
		case claude.StreamTypeResult:
			res, err := m.AsResult()
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "result: %s, error %t, %d turns, $%.4f, %s wall, stop %s\n", res.Subtype, res.IsError, res.NumTurns, res.TotalCostUSD, res.Duration, res.StopReason)
		default:
			fmt.Fprintf(w, "unknown line type %q (%d raw bytes)\n", m.Type, len(m.Raw))
		}
	}
	fmt.Fprintf(w, "partial text from deltas: %q\n", preview(partial.String()))
	return nil
}

// snippet:end stream

// snippet:start retry
// decodeAPIRetry decodes a system/api_retry line: Claude Code retrying a
// failed API call (documented shape; a healthy run has none).
func decodeAPIRetry(w io.Writer) error {
	m, err := claude.NewStreamDecoder(strings.NewReader(apiRetryLine)).Next()
	if err != nil {
		return err
	}
	retry, err := m.AsAPIRetry()
	if err != nil {
		return err
	}
	status := 0
	if retry.ErrorStatus != nil {
		status = *retry.ErrorStatus
	}
	fmt.Fprintf(w, "api_retry: attempt %d of %d after %s, HTTP %d (%s)\n", retry.Attempt, retry.MaxRetries,
		time.Duration(retry.RetryDelayMS)*time.Millisecond, status, retry.Error)
	return nil
}

// snippet:end retry

// snippet:start json
// decodeJSON reads the one document `claude -p --output-format json`
// prints: the same object as the stream's result line.
func decodeJSON(w io.Writer, r io.Reader) error {
	res, err := claude.DecodeJSONResult(r)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "json result: %s, text %q, session %s\n", res.Subtype, preview(res.Result), res.SessionID)
	fmt.Fprintf(w, "  usage: %d in, %d cache read, %d cache write, %d out; API %s, TTFT %s\n",
		res.Usage.Input, res.Usage.CacheRead, res.Usage.CacheCreation, res.Usage.Output, res.APIDuration, res.TTFT)
	models := make([]string, 0, len(res.ModelUsage))
	for m := range res.ModelUsage {
		models = append(models, m)
	}
	slices.Sort(models)
	for _, m := range models {
		fmt.Fprintf(w, "  model %s: $%.4f\n", m, res.ModelUsage[m].CostUSD)
	}
	return nil
}

// snippet:end json

// snippet:start neutral
// neutralEvents maps each line onto agentio.RunEvent: Started, Message,
// Tool, Done or Error, Other for the rest. Code that loops over agents
// reads every vendor's run this way.
func neutralEvents(w io.Writer, r io.Reader) error {
	d := claude.NewStreamDecoder(r)
	counts := map[agentio.RunEventType]int{}
	for {
		m, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		ev := m.Neutral()
		counts[ev.Type]++
		if ev.Type == agentio.RunEventDone && ev.Tokens != nil {
			fmt.Fprintf(w, "neutral done: %q, %d output tokens\n", preview(ev.Text), ev.Tokens.Output)
		}
	}
	for _, t := range []agentio.RunEventType{agentio.RunEventStarted, agentio.RunEventMessage, agentio.RunEventTool, agentio.RunEventDone, agentio.RunEventOther} {
		fmt.Fprintf(w, "  %s events: %d\n", t, counts[t])
	}
	return nil
}

// snippet:end neutral
