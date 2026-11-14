package main

import (
	"fmt"
	"io"

	"github.com/ubgo/agentio"
)

// firstHome is the first home Homes lists (the default when it exists, else
// the CODEX_HOME one); false when the agent has no home on disk. Disk · Read.
func firstHome(a agentio.Agent) (agentio.Home, bool, error) {
	homes, err := a.Homes()
	if err != nil || len(homes) == 0 {
		return agentio.Home{}, false, err
	}
	return homes[0], true, nil
}

// snippet:start config
// printConfig prints the config files that apply in dir (highest precedence first,
// with whether each exists), the project trust and the effective launch
// defaults, for each agent's first home.
func printConfig(w io.Writer, h agentio.Host, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		cr, ok := agentio.ConfigOf(a)
		if !ok {
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		layers, err := cr.ConfigLayers(home, dir)
		if err != nil {
			return err
		}
		for _, l := range layers {
			fmt.Fprintf(w, "%s layer %-8s exists=%-5t %s\n", a.ID(), l.Scope, l.Exists, tidy(h, l.Path))
		}
		trust, err := cr.ProjectTrust(home, dir)
		if err != nil {
			return err
		}
		def, err := cr.Defaults(home, dir)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s trust=%s model=%q autonomy=%s\n", a.ID(), trust, def.Model, def.Autonomy)
	}
	return nil
}

// snippet:end config

// snippet:start mcp
// printMCP lists configured MCP servers without spawning one. Env values
// never appear: MCPServer carries variable names only.
func printMCP(w io.Writer, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		ml, ok := agentio.MCPServersOf(a)
		if !ok {
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		servers, err := ml.MCPServers(home, dir)
		if err != nil {
			return err
		}
		for _, s := range servers {
			target := s.Command
			if s.URL != "" {
				target = s.URL
			}
			fmt.Fprintf(w, "%s %-14s %-5s scope=%-7s %s env=%v\n", a.ID(), s.Name, s.Transport, s.Scope, target, s.EnvNames)
		}
	}
	return nil
}

// snippet:end mcp

// snippet:start instructions
// printInstructions lists the instruction files that apply in dir, in the
// order the agent loads them.
func printInstructions(w io.Writer, h agentio.Host, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		in, ok := agentio.InstructionFilesOf(a)
		if !ok {
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		files, err := in.InstructionFiles(home, dir)
		if err != nil {
			return err
		}
		for _, f := range files {
			fmt.Fprintf(w, "%s %-8s %-10s %4d bytes %s\n", a.ID(), f.Scope, f.Kind, f.Bytes, tidy(h, f.Path))
		}
	}
	return nil
}

// snippet:end instructions

// snippet:start skills
// printSkillsAndPlugins lists skills (frontmatter only) and installed
// plugins for each agent's first home.
func printSkillsAndPlugins(w io.Writer, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		if sl, ok := agentio.SkillsOf(a); ok {
			skills, err := sl.Skills(home, dir)
			if err != nil {
				return err
			}
			for _, s := range skills {
				fmt.Fprintf(w, "%s skill  %-14s scope=%-7s plugin=%q\n", a.ID(), s.Name, s.Scope, s.Plugin)
			}
		}
		if pl, ok := agentio.PluginsOf(a); ok {
			plugins, err := pl.Plugins(home, dir)
			if err != nil {
				return err
			}
			for _, p := range plugins {
				fmt.Fprintf(w, "%s plugin %s@%s version=%s enabled=%t\n", a.ID(), p.Name, p.Marketplace, p.Version, p.Enabled)
			}
		}
	}
	return nil
}

// snippet:end skills

// snippet:start models
// printModels lists the on-disk model catalog. Only Codex keeps one; for
// Claude the accessor reports false and Unsupported says why.
func printModels(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		ml, ok := agentio.ModelsOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapModels)
			fmt.Fprintf(w, "%s: no model catalog (%s)\n", a.ID(), reason)
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		models, err := ml.Models(home)
		if err != nil {
			return err
		}
		for _, m := range models {
			fmt.Fprintf(w, "%s model %-16s visible=%-5t context=%d\n", a.ID(), m.Slug, m.Visible, m.ContextWindow)
		}
	}
	return nil
}

// snippet:end models
