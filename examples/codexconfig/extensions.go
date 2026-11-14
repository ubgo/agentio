package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// argv prints a Command as `codex <args>`: the resolved binary path is a
// temp dir here, and the env (which may carry CODEX_HOME) is left out.
func argv(c agentio.Command) string {
	return strings.TrimSpace(filepath.Base(c.Path) + " " + strings.Join(c.Args, " "))
}

// showMCP covers docs/codex/mcp.md.
func showMCP(w io.Writer, m machine) error {
	if err := listMCP(w, m); err != nil {
		return err
	}
	return mcpCommands(w, m.host)
}

// snippet:start list-mcp
// listMCP lists the effective [mcp_servers] for a folder (user config
// plus trusted project layers). Env values are never in the struct: only
// their names, and one value on request as a redacting EnvValue.
func listMCP(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	servers, err := a.MCPServers(h, m.project)
	if err != nil {
		return err
	}
	for _, s := range servers {
		fmt.Fprintf(w, "mcp %s transport=%s layer=%s enabled-set=%v env-keys=%v\n",
			s.Name, s.Transport(), s.Layer, s.Enabled.Set, s.EnvKeys)
	}
	v, err := a.MCPServerEnvValue(h, "node_repl", "SECRET_TOKEN")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "SECRET_TOKEN prints as %v\n", v)
	if _, err := a.MCPServerEnvValue(h, "node_repl", "MISSING"); errors.Is(err, codex.ErrNoEnvValue) {
		fmt.Fprintln(w, "MISSING: not in config.toml")
	}
	return nil
}

// snippet:end list-mcp

// snippet:start mcp-commands
// mcpCommands builds `codex mcp add|remove`: the vendor edits its own
// config.toml when the caller runs them. Env values are refused because
// they would ride on argv.
func mcpCommands(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	h := a.DefaultHome()
	add, err := a.MCPAddCommand(h, "docs", codex.MCPAddSpec{URL: "https://mcp.example.test", BearerTokenEnvVar: "DOCS_TOKEN"})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, argv(add))
	stdio, err := a.MCPAddCommand(h, "files", codex.MCPAddSpec{Command: "/opt/mcp/files", Args: []string{"--root", "/work"}})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, argv(stdio))
	fmt.Fprintln(w, argv(a.MCPRemoveCommand(h, "docs")))

	_, err = a.MCPAddCommand(h, "x", codex.MCPAddSpec{Command: "srv", Env: map[string]agentio.EnvValue{"K": agentio.NewEnvValue("v")}})
	fmt.Fprintf(w, "env values refused: %v\n", errors.Is(err, agentio.ErrUnsupported))
	return nil
}

// snippet:end mcp-commands

// showInstructionsSkills covers docs/codex/instructions-skills.md.
func showInstructionsSkills(w io.Writer, m machine) error {
	if err := listInstructions(w, m); err != nil {
		return err
	}
	if err := listSkills(w, m); err != nil {
		return err
	}
	return listPlugins(w, m)
}

// snippet:start instructions
// listInstructions lists the AGENTS.md files Codex would load for a
// folder, in load order, and the project root it walks from.
func listInstructions(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	files, err := a.InstructionFiles(h, m.project)
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Fprintf(w, "instructions %s scope=%s kind=%s bytes=%d truncated=%v\n",
			m.rel(f.Path), f.Scope, f.Kind, f.Bytes, f.Truncated)
	}
	root, found, err := a.ProjectRoot(h, m.project)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "project root=%s found=%v\n", m.rel(root), found)
	return nil
}

// snippet:end instructions

// snippet:start skills
// listSkills reads SKILL.md front matter from user, system, project,
// vendor and plugin skill folders.
func listSkills(w io.Writer, m machine) error {
	a := codex.New(m.host)
	skills, err := a.Skills(a.DefaultHome(), m.project)
	if err != nil {
		return err
	}
	for _, s := range skills {
		fmt.Fprintf(w, "skill %s scope=%s enabled=%v plugin=%q\n", s.Name, s.Scope, s.Enabled, s.Plugin)
	}
	return nil
}

// snippet:end skills

// snippet:start plugins
// listPlugins joins installed plugin folders with config enablement, and
// lists configured marketplaces.
func listPlugins(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	plugins, err := a.Plugins(h)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		fmt.Fprintf(w, "plugin %s@%s version=%s enabled=%v display=%q\n",
			p.Name, p.Marketplace, p.Version, p.Enabled, p.Interface.DisplayName)
	}
	cfg, err := a.ConfigFile(h)
	if err != nil {
		return err
	}
	for _, mp := range a.Marketplaces(cfg) {
		fmt.Fprintf(w, "marketplace %s type=%s source=%s\n", mp.Name, mp.SourceType, mp.Source)
	}
	// Running this command reads remote catalogs; it is only built here.
	fmt.Fprintln(w, argv(a.PluginListCommand(h, true)))
	return nil
}

// snippet:end plugins
