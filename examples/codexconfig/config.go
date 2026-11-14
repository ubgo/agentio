package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ubgo/agentio/codex"
)

// showPaths covers docs/codex/paths.md.
func showPaths(w io.Writer, m machine) error {
	if err := homePaths(w, m); err != nil {
		return err
	}
	return homeInventory(w, m)
}

// snippet:start paths
// homePaths names the files of a home without touching disk. PathsWith
// also honours sqlite_home, log_dir and CODEX_SQLITE_HOME.
func homePaths(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	p := a.Paths(h)
	fmt.Fprintf(w, "config=%s auth=%s\n", m.rel(p.Config), m.rel(p.Auth))
	fmt.Fprintf(w, "sessions=%s state db=%s\n", m.rel(p.Sessions), m.rel(p.StateDB))
	fmt.Fprintf(w, "v2 profile work=%s\n", m.rel(p.ProfileConfig("work")))

	cfg, err := a.ConfigFile(h)
	if err != nil {
		return err
	}
	pw := a.PathsWith(h, cfg)
	fmt.Fprintf(w, "sqlite home with config=%s\n", m.rel(pw.SQLiteHome))
	return nil
}

// snippet:end paths

// snippet:start inventory
// homeInventory stats every known path once and names unknown entries.
func homeInventory(w io.Writer, m machine) error {
	a := codex.New(m.host)
	inv, err := a.Inventory(a.DefaultHome())
	if err != nil {
		return err
	}
	var present []string
	for _, e := range inv.Entries {
		if e.Exists {
			present = append(present, e.Name)
		}
	}
	fmt.Fprintf(w, "present: %s\n", strings.Join(present, " "))
	fmt.Fprintf(w, "unknown top-level entries: %d\n", len(inv.Unknown))
	return nil
}

// snippet:end inventory

// showConfig covers docs/codex/config.md.
func showConfig(w io.Writer, m machine) error {
	if err := readConfigFile(w, m); err != nil {
		return err
	}
	if err := parseConfigBytes(w); err != nil {
		return err
	}
	if err := effectiveConfig(w, m); err != nil {
		return err
	}
	if err := readProfiles(w, m); err != nil {
		return err
	}
	return buildOverrides(w)
}

// snippet:start config-file
// readConfigFile decodes <home>/config.toml alone (no layers). Every
// scalar is an Opt: Set tells an absent key from its zero value.
func readConfigFile(w io.Writer, m machine) error {
	a := codex.New(m.host)
	cfg, warns, err := a.ConfigFileWarnings(a.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "model=%s effort=%s (known=%v) warnings=%d\n",
		cfg.Model.V, cfg.ModelReasoningEffort.V, cfg.ModelReasoningEffort.V.IsKnown(), len(warns))
	fmt.Fprintf(w, "sandbox_mode set=%v approval_policy set=%v\n", cfg.SandboxMode.Set, cfg.ApprovalPolicy.Set)

	kc := cfg.KeyCount()
	fmt.Fprintf(w, "keys total=%d typed=%d raw=%d unknown=%d\n", kc.Total, kc.Typed, kc.Raw, kc.Unknown)

	// Unknown keeps every unmodelled key with its TOML source text.
	keys := make([]string, 0, len(cfg.Unknown))
	for k := range cfg.Unknown {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "unknown %s = %s\n", k, cfg.Unknown[k])
	}
	return nil
}

// snippet:end config-file

// snippet:start parse-config
// parseConfigBytes decodes config.toml text you already hold. A key whose
// type changed degrades to Unknown with a warning; the rest still reads.
func parseConfigBytes(w io.Writer) error {
	src := []byte("approval_policy = \"on-request\"\nsandbox_mode = \"brand-new-mode\"\nmodel_context_window = \"wide\"\n")
	cfg, warns, err := codex.ParseConfig(src, "inline.toml")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "approval=%s known=%v\n", cfg.ApprovalPolicy.V, cfg.ApprovalPolicy.V.IsKnown())
	fmt.Fprintf(w, "sandbox=%s known=%v\n", cfg.SandboxMode.V, cfg.SandboxMode.V.IsKnown())
	for _, wn := range warns {
		fmt.Fprintf(w, "warning: %s\n", wn)
	}
	return nil
}

// snippet:end parse-config

// snippet:start effective
// effectiveConfig merges the layers Codex applies for a folder and says
// which layer won each key. The project's .codex/config.toml applies only
// once the folder is trusted in [projects].
func effectiveConfig(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()

	layers, err := a.ConfigLayers(h, m.project)
	if err != nil {
		return err
	}
	for _, l := range layers {
		state := "applied"
		if l.Disabled != "" {
			state = "disabled: " + l.Disabled
		}
		fmt.Fprintf(w, "layer %-12s exists=%-5v %s\n", l.Kind, l.Exists, state)
	}

	eff, err := a.Effective(h, codex.EffectiveQuery{
		Dir:       m.project,
		Profile:   "work",
		Overrides: []codex.Override{codex.OverrideString("model_verbosity", "low")},
	})
	if err != nil {
		return err
	}
	for _, key := range []string{"model", "model_reasoning_effort", "model_verbosity"} {
		fmt.Fprintf(w, "effective %s from %s\n", key, eff.Origins[key])
	}
	fmt.Fprintf(w, "model=%s not evaluated=%v\n", eff.Config.Model.V, eff.NotEvaluated)

	cfg := eff.Config
	if lvl, ok := cfg.TrustOf("/work/proj/sub"); ok {
		fmt.Fprintf(w, "trust of /work/proj/sub: %s\n", lvl)
	}
	fmt.Fprintf(w, "feature overrides: %v\n", cfg.FeatureOverrides())
	return nil
}

// snippet:end effective

// snippet:start profiles
// readProfiles lists legacy [profiles.*] tables and v2 <name>.config.toml
// files, and checks a name before passing it to `codex -p`.
func readProfiles(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	ps, err := a.Profiles(h)
	if err != nil {
		return err
	}
	for _, p := range ps {
		fmt.Fprintf(w, "profile %s kind=%s table=%v file=%s\n", p.Name, p.Kind, p.HasTable, m.rel(p.Path))
	}
	files, err := a.ProfileFiles(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "v2 profile files: %v\n", files)

	cfg, err := a.ConfigFile(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "legacy names: %v\n", cfg.LegacyProfileNames())
	if err := cfg.ProfileConflict("ollama-launch"); errors.Is(err, codex.ErrProfileConflict) {
		fmt.Fprintf(w, "conflict: %v\n", err)
	}
	return nil
}

// snippet:end profiles

// snippet:start overrides
// buildOverrides makes `-c path=value` arguments. The typed constructors
// quote TOML for you; ParseOverride reads text the way Codex does.
func buildOverrides(w io.Writer) error {
	ovs := []codex.Override{
		codex.OverrideString("model", "gpt-5.5"),
		codex.OverrideBool("features.hooks", true),
		codex.OverrideInt("model_context_window", 200000),
		codex.OverrideStrings("notify", []string{"/bin/notify", "--quiet"}),
	}
	for _, text := range []string{"sandbox_mode=read-only", "model_auto_compact_token_limit=1000"} {
		o, err := codex.ParseOverride(text)
		if err != nil {
			return err
		}
		ovs = append(ovs, o)
	}
	for _, o := range ovs {
		fmt.Fprintf(w, "-c %s\n", o)
	}
	if _, err := codex.ParseOverride("no-equals-sign"); errors.Is(err, codex.ErrBadOverride) {
		fmt.Fprintln(w, "rejected: no '='")
	}
	return nil
}

// snippet:end overrides
