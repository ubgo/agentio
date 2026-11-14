package claude

import (
	"errors"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestMCPAddCommandNeverPutsSecretsOnArgv guards invariant 8: env values and
// headers are refused, and a plain server's argv carries no secret.
func TestMCPAddCommandNeverPutsSecretsOnArgv(t *testing.T) {
	a := New(agenttest.NewHost(t))
	h := a.DefaultHome()
	secret := "s3cret-value"
	for _, s := range []MCPServer{
		{Name: "e", Transport: "stdio", Command: "srv", Env: map[string]agentio.EnvValue{"K": agentio.NewEnvValue(secret)}},
		{Name: "h", Transport: "http", URL: "https://x", Headers: map[string]agentio.EnvValue{"Authorization": agentio.NewEnvValue(secret)}},
	} {
		c, err := a.MCPAddCommand(h, "", s)
		if !errors.Is(err, agentio.ErrUnsupported) {
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
