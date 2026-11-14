package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// GitInfo is the git snapshot Codex records per conversation (rollout
// session_meta.git, threads.git_* columns). Upstream protocol.rs GitInfo.
type GitInfo struct {
	CommitHash    string `json:"commit_hash,omitempty"`
	Branch        string `json:"branch,omitempty"`
	RepositoryURL string `json:"repository_url,omitempty"`
}

// SandboxPolicy is the decoded sandbox_policy JSON of a turn_context record
// and of threads.sandbox_policy. Upstream it is a tagged enum on "type";
// fields that do not apply to a variant stay zero.
type SandboxPolicy struct {
	Type                SandboxMode `json:"type"`
	WritableRoots       []string    `json:"writable_roots,omitempty"`
	NetworkAccess       bool        `json:"network_access,omitempty"`
	ExcludeTmpdirEnvVar bool        `json:"exclude_tmpdir_env_var,omitempty"`
	ExcludeSlashTmp     bool        `json:"exclude_slash_tmp,omitempty"`
}

// sandboxPolicyWire tolerates network_access as a bool (read-only,
// workspace-write) or as the ExternalSandbox NetworkAccess enum string.
type sandboxPolicyWire struct {
	Type                SandboxMode     `json:"type"`
	WritableRoots       []string        `json:"writable_roots"`
	NetworkAccess       json.RawMessage `json:"network_access"`
	ExcludeTmpdirEnvVar bool            `json:"exclude_tmpdir_env_var"`
	ExcludeSlashTmp     bool            `json:"exclude_slash_tmp"`
}

// networkEnabled is the ExternalSandbox NetworkAccess value that allows traffic.
const networkEnabled = "enabled"

// UnmarshalJSON accepts a bare string ("danger-full-access", older
// versions) or the tagged object. Pure.
func (p *SandboxPolicy) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("codex: sandbox_policy: %w", err)
		}
		*p = SandboxPolicy{Type: SandboxMode(s)}
		return nil
	}
	var w sandboxPolicyWire
	if err := json.Unmarshal(trimmed, &w); err != nil {
		return fmt.Errorf("codex: sandbox_policy: %w", err)
	}
	out := SandboxPolicy{Type: w.Type, WritableRoots: w.WritableRoots, ExcludeTmpdirEnvVar: w.ExcludeTmpdirEnvVar, ExcludeSlashTmp: w.ExcludeSlashTmp}
	if len(w.NetworkAccess) > 0 {
		var on bool
		if json.Unmarshal(w.NetworkAccess, &on) == nil {
			out.NetworkAccess = on
		} else {
			var s string
			if json.Unmarshal(w.NetworkAccess, &s) == nil {
				out.NetworkAccess = s == networkEnabled
			}
		}
	}
	*p = out
	return nil
}

// UnmarshalJSON decodes a SessionSource from a plain string ("cli") or an
// upstream object variant ({"subagent": {...}}, {"custom": "x"}), keeping
// the variant name. Pure.
func (s *SessionSource) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = ""
		return nil
	}
	if trimmed[0] == '"' {
		var v string
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return fmt.Errorf("codex: source: %w", err)
		}
		*s = SessionSource(v)
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return fmt.Errorf("codex: source: %w", err)
	}
	for k := range obj {
		*s = SessionSource(k)
		return nil
	}
	*s = SourceUnknown
	return nil
}

// ParseSessionSource converts a stored threads.source text (a plain string
// or the JSON of an object variant) to a SessionSource. Pure.
func ParseSessionSource(text string) SessionSource {
	var s SessionSource
	if len(text) > 0 && (text[0] == '{' || text[0] == '"') {
		if json.Unmarshal([]byte(text), &s) == nil {
			return s
		}
	}
	return SessionSource(text)
}
