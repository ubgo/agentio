# Claude status line

Claude Code can show a custom status line under the prompt: it runs a command, writes the session state as JSON on the command's stdin, and shows the first line the command prints. The `claude` module reads the `statusLine` setting and decodes that input, so a status-line program in Go is a dozen lines.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- Reading the setting needs an agent and a home (see [The Claude agent](agent.md)); the status-line program itself needs neither.

The snippets come from [`examples/claudehooks`](../../examples/claudehooks/). Its `statusline` mode is a complete status-line program.

## Step 1 — Read the status line setting

The setting is `Settings.StatusLine` (`*claude.StatusLine`: `Type`, `Command`, `Padding`, unknown keys kept), nil when no layer sets it. The example writes it with the [Claude writer](writer.md), reads it back from the user layer, then runs the program on a sample input:

<!-- snippet: examples/claudehooks/main.go#showstatus -->
```go
// showStatusLine sets the statusLine setting through the Writer, reads it
// back, and runs statusLineMain on the sample input.
func showStatusLine(p printer, a *claude.Agent) error {
	p.section("status line")
	h := a.DefaultHome()
	setting, err := json.Marshal(claude.StatusLine{Type: "command", Command: statusLineCmd})
	if err != nil {
		return err
	}
	if _, err := a.Writer().SetUserSetting(h, claude.SettingsPatch{Set: map[string]agentio.Raw{"statusLine": setting}}); err != nil {
		return err
	}
	user, err := a.UserSettings(h)
	if err != nil {
		return err
	}
	if sl := user.Settings.StatusLine; sl != nil {
		p.line(fmt.Sprintf("setting: type=%s command=%s", sl.Type, sl.Command))
	}
	in, err := claude.DecodeStatusLineInput(strings.NewReader(statusLineInput))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("input: model=%s project=%s style=%s unknown=%v", in.Model.ID, in.Workspace.ProjectDir, in.OutputStyle.Name, in.Unknown.Keys()))
	var line bytes.Buffer
	if err := statusLineMain(strings.NewReader(statusLineInput), &line); err != nil {
		return err
	}
	p.line(fmt.Sprintf("prints: %s", strings.TrimSpace(line.String())))
	return nil
}
```

## Step 2 — Decode status line input

`claude.DecodeStatusLineInput(os.Stdin)` returns a `StatusLineInput`: `SessionID`, `TranscriptPath`, `Cwd`, `Version`, `Model` (`StatusLineModel`: `ID`, `DisplayName`), `Workspace` (`StatusLineWorkspace`: `CurrentDir`, `ProjectDir`), `OutputStyle` (`StatusLineOutputStyle`: `Name`), `Cost` (`*StatusLineCost`: total cost, durations, lines added and removed) and `Unknown`. This is the whole status-line program:

<!-- snippet: examples/claudehooks/main.go#statusline -->
```go
// statusLineMain is a complete status-line command: Claude Code runs it,
// writes the session state on stdin and shows the first line it prints.
func statusLineMain(stdin io.Reader, stdout io.Writer) error {
	in, err := claude.DecodeStatusLineInput(stdin)
	if err != nil {
		return err
	}
	dir := filepath.Base(in.Workspace.CurrentDir)
	line := fmt.Sprintf("[%s] %s", in.Model.DisplayName, dir)
	if in.Cost != nil { // absent before the first request
		line += fmt.Sprintf(" $%.2f +%d -%d", in.Cost.TotalCostUSD, in.Cost.TotalLinesAdded, in.Cost.TotalLinesRemoved)
	}
	_, err = fmt.Fprintln(stdout, line)
	return err
}
```

## Try it

```sh
cd examples && go run ./claudehooks
```

The section this page covers:

```text
== status line
setting: type=command command=/usr/local/bin/my-statusline
input: model=claude-opus-5-5 project=/work/project style=default unknown=[exceeds_200k_tokens]
prints: [Opus] src $0.42 +12 -3
```

Run the program directly on the sample input:

```sh
cd examples && go run ./claudehooks statusline < claudehooks/testdata/statusline.json
```

```text
[Opus] src $0.42 +12 -3
```

## Pitfalls

- **The input shape is not verified against a recording.** The fields come from the Claude Code docs; no status-line input was captured on the tested versions. Keys you need that are not modelled are in `Unknown`, and a field may be empty on a real session.
- `Cost` is a pointer and is nil before the first request of a session; guard it, as the example does.
- Claude Code shows only the first line of output and runs the command often; keep it fast and never call the network from it.
- `StatusLine.Padding` is a pointer: nil means the key is absent, not zero.

## Related

- [Claude hooks](hooks.md), [Claude writer](writer.md), [Claude settings](settings.md)
- Example: [`examples/claudehooks`](../../examples/claudehooks/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
