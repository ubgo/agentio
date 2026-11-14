package claude

import (
	"encoding/json"
	"io"
	"reflect"

	"github.com/ubgo/agentio"
)

// StatusLineModel is the model block of the status-line input.
type StatusLineModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// StatusLineWorkspace is the workspace block of the status-line input.
type StatusLineWorkspace struct {
	CurrentDir string `json:"current_dir"`
	ProjectDir string `json:"project_dir"`
}

// StatusLineOutputStyle is the output_style block of the status-line input.
type StatusLineOutputStyle struct {
	Name string `json:"name"`
}

// StatusLineCost is the cost block of the status-line input.
type StatusLineCost struct {
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalDurationMS    int64   `json:"total_duration_ms"`
	TotalAPIDurationMS int64   `json:"total_api_duration_ms"`
	TotalLinesAdded    int     `json:"total_lines_added"`
	TotalLinesRemoved  int     `json:"total_lines_removed"`
}

// StatusLineInput is the JSON a status-line command receives on stdin.
// Fields are from docs knowledge and NOT verified on 2.1.323 (no sample
// recorded); every other key is kept in Unknown.
type StatusLineInput struct {
	SessionID      string                `json:"session_id"`
	TranscriptPath string                `json:"transcript_path"`
	Cwd            string                `json:"cwd"`
	Version        string                `json:"version"`
	Model          StatusLineModel       `json:"model"`
	Workspace      StatusLineWorkspace   `json:"workspace"`
	OutputStyle    StatusLineOutputStyle `json:"output_style"`
	Cost           *StatusLineCost       `json:"cost,omitempty"`
	Unknown        agentio.Unknown       `json:"-"`
}

// DecodeStatusLineInput decodes a status-line command's stdin. cost: Pure.
func DecodeStatusLineInput(r io.Reader) (StatusLineInput, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxLineBytes))
	if err != nil {
		return StatusLineInput{}, err
	}
	var in StatusLineInput
	unknown, _, err := decodeLenient("status line input", b, reflect.TypeFor[StatusLineInput](), func(d []byte) error { in = StatusLineInput{}; return json.Unmarshal(d, &in) })
	if err != nil {
		return StatusLineInput{}, err
	}
	in.Unknown = unknown
	return in, nil
}
