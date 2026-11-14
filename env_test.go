package agentio

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// testHomeVar stands in for CLAUDE_CONFIG_DIR / CODEX_HOME.
const testHomeVar = "TOOL_HOME"

// envVals returns every value env holds for key.
func envVals(env []string, key string) []string {
	var out []string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			out = append(out, v)
		}
	}
	return out
}

// TestHomeEnvChange_Matrix pins invariant 2 over every inherited env shape
// (ported from agx TestHomeEnv_Matrix): the default home runs with the
// variable ABSENT, every other home with exactly one entry.
func TestHomeEnvChange_Matrix(t *testing.T) {
	t.Parallel()
	const def = "/u/.tool"
	inherited := [][]string{
		nil, {}, {"PATH=/bin"},
		{"PATH=/bin", testHomeVar + "=/stale"},
		{testHomeVar + "=" + def},
		{testHomeVar + "=/a", "X=1", testHomeVar + "=/b"},
		{testHomeVar + "="},
		{testHomeVar + "X=keep", "X" + testHomeVar + "=keep"},
	}
	for _, tc := range []struct {
		name string
		home Home
	}{
		{"default", NewHome("tool", def, true, DefaultHomeName)},
		{"sibling", NewHome("tool", "/u/.tool-work", false, "work")},
		{"unicode", NewHome("tool", "/u/.tool-ünï", false, "x")},
		{"spaces", NewHome("tool", "/u/my tool home", false, "x")},
		{"default as prefix", NewHome("tool", "/u/.tool/sub", false, "x")},
	} {
		for i, env := range inherited {
			before := slices.Clone(env)
			c, ok := HomeEnvChange(testHomeVar, tc.home)
			if !ok {
				t.Fatal("HomeEnvChange: no change for a named variable")
			}
			got := ApplyEnv(env, c)
			vals := envVals(got, testHomeVar)
			if tc.home.IsDefault() && len(vals) != 0 {
				t.Errorf("%s/env%d: default home must run with %s absent, got %v", tc.name, i, testHomeVar, vals)
			}
			if !tc.home.IsDefault() && !slices.Equal(vals, []string{tc.home.Dir()}) {
				t.Errorf("%s/env%d: want exactly %s=%s, got %v", tc.name, i, testHomeVar, tc.home.Dir(), vals)
			}
			if !slices.Equal(env, before) {
				t.Errorf("%s/env%d: input mutated", tc.name, i)
			}
			for _, kv := range env {
				if !strings.HasPrefix(kv, testHomeVar+"=") && !slices.Contains(got, kv) {
					t.Errorf("%s/env%d: dropped unrelated %q", tc.name, i, kv)
				}
			}
		}
	}
	if _, ok := HomeEnvChange("", NewHome("tool", def, true, "")); ok {
		t.Error("an agent without a home variable must yield no change")
	}
}

func TestApplyEnv_Matrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		env     []string
		changes []EnvChange
		want    []string
	}{
		{"nil env set", nil, []EnvChange{EnvSet("K", "v")}, []string{"K=v"}},
		{"replace", []string{"K=old"}, []EnvChange{EnvSet("K", "v")}, []string{"K=v"}},
		{"duplicates collapse", []string{"K=1", "A=2", "K=3"}, []EnvChange{EnvSet("K", "v")}, []string{"A=2", "K=v"}},
		{"value with equals", []string{"A=1"}, []EnvChange{EnvSet("K", "a=b=c")}, []string{"A=1", "K=a=b=c"}},
		{"empty value", []string{"K=x"}, []EnvChange{EnvSet("K", "")}, []string{"K="}},
		{"longer name kept", []string{"KK=1", "K_X=2"}, []EnvChange{EnvUnset("K")}, []string{"KK=1", "K_X=2"}},
		{"bare name kept", []string{"K"}, []EnvChange{EnvSet("K", "v")}, []string{"K", "K=v"}},
		{"unset then set", []string{"T=old"}, []EnvChange{EnvUnset("T"), EnvSetSecret("T", NewToken("tok"))}, []string{"T=tok"}},
		{"order matters", nil, []EnvChange{EnvSet("K", "1"), EnvUnset("K")}, []string{}},
	} {
		before := slices.Clone(tc.env)
		got := ApplyEnv(tc.env, tc.changes...)
		if !slices.Equal(got, tc.want) && (len(got) != 0 || len(tc.want) != 0) {
			t.Errorf("%s: ApplyEnv = %q, want %q", tc.name, got, tc.want)
		}
		if !slices.Equal(tc.env, before) {
			t.Errorf("%s: input mutated", tc.name)
		}
	}
}

// TestRedaction proves invariant 8 for every redacting type and Command.
func TestRedaction(t *testing.T) {
	t.Parallel()
	const secret = "sk-ant-oat01.SECRETVALUE"
	tok := NewToken(secret)
	var logBuf strings.Builder
	slog.New(slog.NewTextHandler(&logBuf, nil)).Info("x", "tok", tok, "p", NewPersonal(secret), "e", NewEnvValue(secret))
	j, err := json.Marshal(struct {
		T Token
		P Personal
		E EnvValue
		C TokenCredential
	}{tok, NewPersonal(secret), NewEnvValue(secret), TokenCredential{Kind: TokenAPIKey, Token: tok}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := NewCommand("tool", []string{"--x"}, []string{"PATH=/bin", "MY_API_KEY=" + secret}, "", EnvSetSecret("INNOCENT", tok))
	cj, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	outs := []string{
		tok.String(), fmt.Sprintf("%v %+v %#v %s %q %x", tok, tok, tok, tok, tok, tok),
		fmt.Sprintf("%v %#v", NewPersonal(secret), NewEnvValue(secret)),
		logBuf.String(), string(j), string(cj), cmd.String(),
		fmt.Sprintf("%+v %#v", cmd.Redacted(), cmd.Redacted()),
		fmt.Sprintf("%v %+v", EnvSetSecret("K", tok), EnvSetSecret("K", tok)), EnvSetSecret("K", tok).String(),
	}
	for _, o := range outs {
		if strings.Contains(o, secret) {
			t.Errorf("leak: %s", o)
		}
	}
	if !slices.Contains(cmd.Env, "INNOCENT="+secret) {
		t.Error("Command.Env must hold the token for the child")
	}
	if NewToken("").String() != "" || tok.String() != redactedText {
		t.Error("empty token prints empty, a set one prints [redacted]")
	}
	var back struct{ P Personal }
	if err := json.Unmarshal([]byte(`{"P":"a@b.c"}`), &back); err != nil || back.P.Reveal() != "a@b.c" {
		t.Errorf("Personal UnmarshalJSON = %v, %v", back.P.Reveal(), err)
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		v    string
		want time.Duration
		ok   bool
	}{
		{"", 0, false}, {"42", 42 * time.Second, true}, {"0", 0, true}, {"-5", 0, false},
		{"1.5", 0, false}, {"soon", 0, false},
		{now.Add(2 * time.Hour).Format(http.TimeFormat), 2 * time.Hour, true},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0, false},
	} {
		got, ok := RetryAfter(tc.v, now)
		if got != tc.want || ok != tc.ok {
			t.Errorf("RetryAfter(%q) = %v,%v want %v,%v", tc.v, got, ok, tc.want, tc.ok)
		}
	}
	h := http.Header{}
	h.Set(retryAfterHeader, "7")
	if err := RateLimitError(h, now); !errors.Is(err, ErrRateLimited) || !strings.Contains(err.Error(), "7s") {
		t.Errorf("RateLimitError = %v", err)
	}
}
