package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/ubgo/agentio"
)

// MCP server types (cli MCPServerConfig.type).
const (
	mcpTypeSSE  = "sse"
	mcpTypeHTTP = "http"
)

// MCPServer is one configured server with where it came from.
type MCPServer struct {
	Name   string
	Config MCPServerConfig
	Scope  agentio.ConfigScope
	Source string
	// Extension names the extension that declares it; "" for settings.
	Extension string
	// Enabled is false when mcp-server-enablement.json disables it or
	// settings mcp.excluded lists it; nil when nothing says.
	Enabled *bool
}

// Transport derives the transport the CLI uses: httpUrl is streamable
// HTTP, url is SSE unless type says http, tcp is a websocket, else stdio.
// cost: Pure.
func (c MCPServerConfig) Transport() agentio.MCPTransport {
	switch {
	case c.HTTPURL != "":
		return agentio.MCPHTTP
	case c.URL != "" && c.Type == mcpTypeHTTP:
		return agentio.MCPHTTP
	case c.URL != "", c.Type == mcpTypeSSE:
		return agentio.MCPSSE
	case c.TCP != "":
		return agentio.MCPWS
	}
	return agentio.MCPStdio
}

// enablementDoc is mcp-server-enablement.json: normalized server name to
// {"enabled": false}; an absent name is enabled (cli
// McpServerEnablementManager; source-derived).
type enablementDoc map[string]struct {
	Enabled *bool `json:"enabled"`
}

// MCPServers lists the servers that apply in dir: settings layers merged by
// name (system > workspace > user > system defaults), then the servers of
// enabled extensions not already named. Env and header values never leave
// MCPServerConfig as plain text. cost: Disk · Read.
func (a *Agent) MCPServers(h Home, dir string) ([]MCPServer, error) {
	e, err := a.EffectiveSettings(h, dir)
	if err != nil {
		return nil, err
	}
	enable, err := readMCPEnablement(h.Path(mcpEnablementFile))
	if err != nil {
		return nil, err
	}
	var out []MCPServer
	for name, cfg := range e.MCPServers {
		out = append(out, MCPServer{Name: name, Config: cfg, Scope: e.MCPScopes[name], Source: e.MCPSources[name]})
	}
	exts, err := a.Extensions(h, dir)
	if err != nil {
		return nil, err
	}
	for _, x := range exts {
		if !x.Enabled {
			continue
		}
		for name, cfg := range x.Manifest.MCPServers {
			if _, ok := e.MCPServers[name]; ok {
				continue // settings win over an extension's server of that name
			}
			out = append(out, MCPServer{Name: name, Config: cfg, Scope: agentio.ConfigPlugin, Source: joinPath(x.Dir, extensionManifest), Extension: x.Manifest.Name})
		}
	}
	excluded := map[string]bool{}
	if mcp := a.mcpFilters(h, dir); mcp != nil {
		for _, n := range mcp.Excluded {
			excluded[normalizeServerID(n)] = true
		}
	}
	off := false
	for i := range out {
		id := normalizeServerID(out[i].Name)
		if v, ok := enable[id]; ok && v.Enabled != nil {
			en := *v.Enabled
			out[i].Enabled = &en
		}
		if excluded[id] {
			out[i].Enabled = &off
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// normalizeServerID is how the CLI keys enablement and filter lists: lower
// case, trimmed (cli normalizeServerId). cost: Pure.
func normalizeServerID(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// mcpFilters is the effective settings.mcp section (highest layer that
// sets it). cost: Disk · Read.
func (a *Agent) mcpFilters(h Home, dir string) *MCPSettings {
	layers, err := a.Settings(h, dir)
	if err != nil {
		return nil
	}
	for _, l := range layers {
		if l.Settings.MCP != nil {
			return l.Settings.MCP
		}
	}
	return nil
}

// readMCPEnablement reads mcp-server-enablement.json; missing = empty.
// cost: Disk · Read.
func readMCPEnablement(p string) (enablementDoc, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return enablementDoc{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gemini: read %s: %w", p, err)
	}
	var doc enablementDoc
	if err := json.Unmarshal(StripJSONComments(agentio.StripBOM(data)), &doc); err != nil {
		return nil, fmt.Errorf("gemini: decode %s: %w", p, err)
	}
	return doc, nil
}

// Neutral projects a server onto agentio.MCPServer: env names only.
// cost: Pure.
func (s MCPServer) Neutral() agentio.MCPServer {
	n := agentio.MCPServer{
		Name: s.Name, Transport: s.Config.Transport(), Scope: s.Scope,
		Command: s.Config.Command, Args: s.Config.Args, Source: s.Source, Enabled: s.Enabled,
	}
	n.URL = s.Config.HTTPURL
	if n.URL == "" {
		n.URL = s.Config.URL
	}
	if n.URL == "" {
		n.URL = s.Config.TCP
	}
	for k := range s.Config.Env {
		n.EnvNames = append(n.EnvNames, k)
	}
	sort.Strings(n.EnvNames)
	return n
}
