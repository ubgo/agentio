package codex

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
)

// untypedConfigKeys are decoder keys that are sections, raw sub-trees,
// secret-bearing keys reported by presence only, or nested shapes the
// table does not offer as -c targets. Every other key the decoder
// switches on must be in ConfigKeys.
func untypedConfigKeys() []string {
	return []string{
		"approval_policy", "granular", keyModelProviders, keyProfiles, keyMCPServers, keyProjects, keyPlugins, keyMarketplaces,
		keyFeatures, "hooks", keyPermissions, "sandbox_workspace_write", "shell_environment_policy", "set", "skills", "history",
		"agents", "goals", "analytics", "feedback", "memories", "tui", "desktop",
		"query_params", "http_headers", "env_http_headers", "auth", "aws", "gateway_oauth", "capabilities", "experimental_bearer_token",
		"env", "env_vars", "oauth", "tools", "bearer_token", "startup_timeout_sec", "tool_timeout_sec",
		"name", "source", "last_updated", "last_revision", "source_type", "trusted_hash", "path", keyEnabled,
		// hooks.state.<event> tables and skills.config array-of-tables entries.
		"state", "config", "bundled",
	}
}

// TestConfigKeysCoverDecoder walks the decoder's switch cases and string
// comparisons (go/ast over config.go): a key the decoder types must be in
// ConfigKeys, so the table cannot fall behind the decoder.
func TestConfigKeysCoverDecoder(t *testing.T) {
	t.Parallel()
	f, err := parser.ParseFile(token.NewFileSet(), "config.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		if vs, ok := n.(*ast.ValueSpec); ok {
			for i, nm := range vs.Names {
				if i < len(vs.Values) {
					if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
						consts[nm.Name], _ = strconv.Unquote(bl.Value)
					}
				}
			}
		}
		return true
	})
	lit := func(e ast.Expr) (string, bool) {
		switch v := e.(type) {
		case *ast.BasicLit:
			s, err := strconv.Unquote(v.Value)
			return s, err == nil
		case *ast.Ident:
			s, ok := consts[v.Name]
			return s, ok
		}
		return "", false
	}
	var keys []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || !strings.HasPrefix(fd.Name.Name, "decode") {
			continue
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CaseClause:
				for _, e := range v.List {
					if s, ok := lit(e); ok {
						keys = append(keys, s)
					}
				}
			case *ast.BinaryExpr:
				if v.Op == token.EQL {
					if s, ok := lit(v.Y); ok {
						keys = append(keys, s)
					}
				}
			}
			return true
		})
	}
	last := map[string]bool{}
	for _, k := range ConfigKeys() {
		segs := strings.Split(k.Path, ".")
		last[segs[len(segs)-1]] = true
	}
	for _, k := range keys {
		if k != "" && !last[k] && !slices.Contains(untypedConfigKeys(), k) {
			t.Errorf("decoder key %q is not in ConfigKeys", k)
		}
	}
}

// sampleFor is a TOML value of kind k (the first enum value for enums)
// and one of the wrong kind.
func sampleFor(k ConfigKey) (right, wrong TOMLRaw) {
	switch k.Kind {
	case kBool:
		return "true", `"x"`
	case kInt:
		return "1", `"x"`
	case kList:
		return `["x"]`, "true"
	case kEnum:
		return TOMLRaw(strconv.Quote(k.Values[0])), "1"
	}
	return `"x"`, "true"
}

// TestConfigKeysProbe decodes every entry through ParseConfig: its kind's
// sample is typed, a wrong-kind sample is a type error.
func TestConfigKeysProbe(t *testing.T) {
	t.Parallel()
	for _, k := range ConfigKeys() {
		path := strings.ReplaceAll(k.Path, "*", "n1")
		right, wrong := sampleFor(k)
		if c, err := CheckOverride(Override{Path: path, Value: right}); err != nil || c.Status != OverrideTyped {
			t.Errorf("%s = %s: %+v %v", path, right, c, err)
		}
		if k.Kind == kList && strings.HasSuffix(k.Path, "forced_chatgpt_workspace_id") {
			continue // a scalar string is accepted there too
		}
		if c, err := CheckOverride(Override{Path: path, Value: wrong}); err != nil || c.Status != OverrideTypeError {
			t.Errorf("%s = %s (wrong kind): %+v %v", path, wrong, c, err)
		}
	}
}

// TestCheckOverride covers the other statuses and the constructors.
func TestCheckOverride(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		o    Override
		want OverrideStatus
	}{
		{OverrideString("model_reasoning_effort", "high"), OverrideTyped},
		{OverrideString("model_reasoning_effort", "turbo"), OverrideUnlistedValue},
		{OverrideBool("sandbox_workspace_write.network_access", true), OverrideTyped},
		{OverrideInt("model_context_window", 272000), OverrideTyped},
		{OverrideStrings("mcp_servers.github.args", []string{"-y", "srv"}), OverrideTyped},
		{OverrideString("tui.theme", "dark"), OverrideRaw},
		{OverrideString("brand_new_key", "x"), OverrideUnknown},
		{OverrideBool("model", true), OverrideTypeError},
	} {
		c, err := CheckOverride(tc.o)
		if err != nil || c.Status != tc.want {
			t.Errorf("%s: %+v %v, want %s", tc.o, c, err, tc.want)
		}
	}
	if _, err := CheckOverride(Override{Path: "model", Value: "[unclosed"}); err == nil {
		t.Error("non-TOML value must fail")
	}
	if k, ok := LookupConfigKey("profiles.deep.model"); !ok || k.Kind != agentio.OptionString {
		t.Errorf("lookup = %+v %v", k, ok)
	}
}
