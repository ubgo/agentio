package claude

import (
	"errors"
	"strings"
	"testing"

	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/agenttest"
)

// TestMCPAddCommandNeverPutsSecretsOnArgv guards invariant 8: env values and
// headers are refused, and a plain server's argv carries no secret.
func TestMCPAddCommandNeverPutsSecretsOnArgv(t *testing.T) {
	a := New(agenttest.NewHost(t))
	h := a.DefaultHome()
	secret := "s3cret-value"
	for _, s := range []MCPServer{
		{Name: "e", Transport: "stdio", Command: "srv", Env: map[string]agentcli.EnvValue{"K": agentcli.NewEnvValue(secret)}},
		{Name: "h", Transport: "http", URL: "https://x", Headers: map[string]agentcli.EnvValue{"Authorization": agentcli.NewEnvValue(secret)}},
	} {
		c, err := a.MCPAddCommand(h, "", s)
		if !errors.Is(err, agentcli.ErrUnsupported) {
			t.Fatalf("%s: err = %v, want ErrUnsupported", s.Name, err)
		}
		if strings.Contains(strings.Join(c.Args, " "), secret) || strings.Contains(err.Error(), secret) {
			t.Fatalf("%s: secret leaked", s.Name)
		}
	}
	c, err := a.MCPAddCommand(h, "", MCPServer{Name: "p", Transport: "stdio", Command: "srv", Args: []string{"--x"}})
	if err != nil || !strings.Contains(strings.Join(c.Args, " "), `"command":"srv"`) {
		t.Fatalf("plain server: %v %v", c.Args, err)
	}
}
