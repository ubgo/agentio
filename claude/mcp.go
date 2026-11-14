package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/khanakia/agentcli"
)

// MCPScope is where an MCP server is configured.
type MCPScope string

// MCP scopes (help `mcp add -s`, docs).
const (
	MCPScopeLocal    MCPScope = "local"
	MCPScopeUser     MCPScope = "user"
	MCPScopeProject  MCPScope = "project"
	MCPScopeManaged  MCPScope = "managed"
	MCPScopePlugin   MCPScope = "plugin"
	MCPScopeClaudeAI MCPScope = "claudeai"
	MCPScopeSession  MCPScope = "session"
	MCPScopeLegacy   MCPScope = "legacy"
)

// IsKnown reports a documented scope. cost: Pure.
func (s MCPScope) IsKnown() bool {
	switch s {
	case MCPScopeLocal, MCPScopeUser, MCPScopeProject, MCPScopeManaged, MCPScopePlugin, MCPScopeClaudeAI, MCPScopeSession, MCPScopeLegacy:
		return true
	}
	return false
}

// MCPTransport is a server transport; "" in a file means stdio.
type MCPTransport string

// Transports (help `-t`, add-json also ws).
const (
	TransportStdio MCPTransport = "stdio"
	TransportSSE   MCPTransport = "sse"
	TransportHTTP  MCPTransport = "http"
	TransportWS    MCPTransport = "ws"
)

// IsKnown reports a documented transport. cost: Pure.
func (t MCPTransport) IsKnown() bool {
	switch t {
	case TransportStdio, TransportSSE, TransportHTTP, TransportWS:
		return true
	}
	return false
}

// MCPApproval is a project server's approval state.
type MCPApproval string

// Approval states.
const (
	ApprovalApproved MCPApproval = "approved"
	ApprovalRejected MCPApproval = "rejected"
	ApprovalPending  MCPApproval = "pending"
)

// MCPOAuth holds a server's OAuth client options (never a secret).
type MCPOAuth struct {
	ClientID     string `json:"clientId,omitempty"`
	CallbackPort int    `json:"callbackPort,omitempty"`
}

// MCPServer is one configured server. Env and header values are
// agentcli.EnvValue: they often hold keys, so they print redacted.
type MCPServer struct {
	Name      string
	Scope     MCPScope
	Source    Source
	Transport MCPTransport
	Command   string
	Args      []string
	Env       map[string]agentcli.EnvValue
	URL       string
	Headers   map[string]agentcli.EnvValue
	OAuth     *MCPOAuth
	Approval  MCPApproval
	Unknown   agentcli.Unknown
}

type wireMCPServer struct {
	Type    MCPTransport                 `json:"type"`
	Command string                       `json:"command"`
	Args    []string                     `json:"args"`
	Env     map[string]agentcli.EnvValue `json:"env"`
	URL     string                       `json:"url"`
	Headers map[string]agentcli.EnvValue `json:"headers"`
	OAuth   *MCPOAuth                    `json:"oauth"`
}

// parseMCPServer decodes one server entry.
func parseMCPServer(name string, raw json.RawMessage) (MCPServer, error) {
	var w wireMCPServer
	unknown, _, err := decodeLenient("mcpServers", raw, reflect.TypeFor[wireMCPServer](), func(b []byte) error { w = wireMCPServer{}; return json.Unmarshal(b, &w) })
	if err != nil {
		return MCPServer{}, err
	}
	t := w.Type
	if t == "" {
		t = TransportStdio
		if w.URL != "" && w.Command == "" {
			t = TransportHTTP
		}
	}
	return MCPServer{Name: name, Transport: t, Command: w.Command, Args: w.Args, Env: w.Env, URL: w.URL, Headers: w.Headers, OAuth: w.OAuth, Unknown: unknown}, nil
}

// parseServerMap decodes an mcpServers object; bad entries become warnings.
func parseServerMap(in map[string]json.RawMessage, scope MCPScope, file string, warns agentcli.Warnings) (map[string]MCPServer, agentcli.Warnings) {
	if in == nil {
		return nil, warns
	}
	out := make(map[string]MCPServer, len(in))
	for name, raw := range in {
		s, err := parseMCPServer(name, raw)
		if err != nil {
			warns = append(warns, agentcli.Warning{Kind: agentcli.WarnTypeChanged, File: file, Key: "mcpServers", Detail: "one server entry did not decode"})
			continue
		}
		s.Scope = scope
		out[name] = s
	}
	return out, warns
}

// wireMCPFile is a file with a top-level mcpServers object (.mcp.json,
// managed-mcp.json, <home>/mcp.json, --mcp-config files).
type wireMCPFile struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

// ParseMCPConfig decodes an mcpServers file body (for --mcp-config files).
// cost: Pure.
func ParseMCPConfig(b []byte) (map[string]MCPServer, error) {
	var w wireMCPFile
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("claude: decode mcp config: %w", err)
	}
	m, _ := parseServerMap(w.MCPServers, MCPScopeSession, "", nil)
	return m, nil
}

// readMCPFile reads one mcpServers file into the list.
func readMCPFile(path string, scope MCPScope, kind SourceKind) ([]MCPServer, error) {
	data, found, err := readFileBounded(path, maxJSONFileBytes)
	if err != nil || !found {
		return nil, err
	}
	var w wireMCPFile
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("claude: decode %s: %w", path, err)
	}
	m, _ := parseServerMap(w.MCPServers, scope, path, nil)
	return sortedServers(m, Source{Kind: kind, Path: path}), nil
}

func sortedServers(m map[string]MCPServer, src Source) []MCPServer {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]MCPServer, 0, len(m))
	for _, n := range names {
		s := m[n]
		s.Source = src
		out = append(out, s)
	}
	return out
}

// MCPServers lists every file-configured server for h (and p when set):
// local (per-project in .claude.json), project (.mcp.json, with approval),
// user (.claude.json), managed (managed-mcp.json), and the legacy
// <home>/mcp.json. Listing never spawns a server. cost: Disk · Read.
func (a *Agent) MCPServers(h Home, p *Project) ([]MCPServer, error) {
	g, err := a.GlobalConfig(h)
	if err != nil {
		return nil, err
	}
	gsrc := Source{Kind: SourceGlobalConfig, Path: g.Path}
	var out []MCPServer
	if p != nil {
		if ps, ok := g.Projects[filepath.Clean(p.Dir)]; ok {
			out = append(out, sortedServers(ps.MCPServers, gsrc)...)
		}
		proj, err := readMCPFile(filepath.Join(p.Dir, projectMCPFile), MCPScopeProject, SourceProject)
		if err != nil {
			return nil, err
		}
		eff, _ := a.Settings(h, p, nil)
		var ps *ProjectState
		if s, ok := g.Projects[filepath.Clean(p.Dir)]; ok {
			ps = &s
		}
		for i := range proj {
			proj[i].Approval = approval(proj[i].Name, eff.Merged, ps)
		}
		out = append(out, proj...)
	}
	out = append(out, sortedServers(g.MCPServers, gsrc)...)
	managed, err := readMCPFile(a.SystemPaths().ManagedMCP, MCPScopeManaged, SourceManagedFile)
	if err != nil {
		return nil, err
	}
	out = append(out, managed...)
	legacy, err := readMCPFile(filepath.Join(h.Dir(), legacyMCPFile), MCPScopeLegacy, SourceUser)
	if err != nil {
		return nil, err
	}
	return append(out, legacy...), nil
}

// approval derives a project server's state from the settings lists and the
// per-project lists in .claude.json; neither naming it is pending.
func approval(name string, s Settings, ps *ProjectState) MCPApproval {
	var enabled, disabled []string
	enabled = append(enabled, s.EnabledMcpjsonServers...)
	disabled = append(disabled, s.DisabledMcpjsonServers...)
	if ps != nil {
		enabled = append(enabled, ps.EnabledMcpjsonServers...)
		disabled = append(disabled, ps.DisabledMcpjsonServers...)
	}
	switch {
	case slices.Contains(disabled, name):
		return ApprovalRejected
	case slices.Contains(enabled, name), s.EnableAllProjectMcpServers != nil && *s.EnableAllProjectMcpServers:
		return ApprovalApproved
	}
	return ApprovalPending
}

// wireNeedsAuth is one mcp-needs-auth-cache.json entry.
type wireNeedsAuth struct {
	Timestamp millis `json:"timestamp"`
}

// MCPNeedsAuth reads mcp-needs-auth-cache.json: server → when it was seen
// needing auth. cost: Disk · Read.
func (a *Agent) MCPNeedsAuth(h Home) (map[string]time.Time, error) {
	p := filepath.Join(h.Dir(), mcpNeedsAuthFile)
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil || !found {
		return nil, err
	}
	var w map[string]wireNeedsAuth
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("claude: decode %s: %w", p, err)
	}
	out := make(map[string]time.Time, len(w))
	for k, v := range w {
		out[k] = v.Timestamp.Time()
	}
	return out, nil
}
