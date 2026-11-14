package claude

import (
	"context"
	"errors"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestNilHTTPIsUnsupported guards "no hidden state": every Claude network
// path on a Host without an HTTP client returns ErrUnsupported instead of
// building a private client that would reach the real network.
func TestNilHTTPIsUnsupported(t *testing.T) {
	t.Parallel()
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithHTTP(nil))
	ctx := context.Background()
	calls := map[string]func() error{
		"usage": func() error { _, err := a.Usage(ctx, a.DefaultHome()); return err },
		"subscription": func() error {
			_, err := a.Subscription(ctx, a.DefaultHome())
			return err
		},
		"probe": func() error {
			_, err := a.Prober().TokenUsage(ctx, agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("sk-ant-oat01-x")})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, agentio.ErrUnsupported) {
			t.Errorf("%s with nil Host.HTTP: %v, want ErrUnsupported", name, err)
		}
	}
}
