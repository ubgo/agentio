package agentio

import (
	"context"
	"slices"
)

// specAgent is a config-only agent: a Core plus the capabilities a Spec can
// describe, each gated on its data (CapabilityGate).
type specAgent struct{ *Core }

// specProviderAgent is a specAgent whose token_env names a model provider
// (pi-like: the variable depends on the provider). It adds
// TokenProviderLister, which a caller reads as "this agent wants a
// provider". A separate type, not a method on every specAgent, because
// that interface's presence is the signal: a spec naming no provider must
// not ask callers for one.
type specProviderAgent struct{ specAgent }

var (
	_ Agent               = specAgent{}
	_ ProcessClassifier   = specAgent{}
	_ LoginCommander      = specAgent{}
	_ HomeLinker          = specAgent{}
	_ BinaryVersion       = specAgent{}
	_ Installer           = specAgent{}
	_ Doctor              = specAgent{}
	_ CapabilityGate      = specAgent{}
	_ UnsupportedReasoner = specAgent{}
	_ TokenLogin          = specAgent{}
	_ TokenProviderLister = specProviderAgent{}
)

// configOnlyReason is why a config-only agent lacks code-backed capabilities.
const configOnlyReason = "config-only agent: a spec describes L1 only; this needs code"

// specUndescribedReason is why a config-only agent refuses a request
// field, login flow or token login its spec has no template or entry for.
const specUndescribedReason = "not described by the agent spec"

// specTokenLoginReason is why a config-only agent has no token login: the
// spec's token_env is what names the variable a token goes into.
const specTokenLoginReason = "spec has no token_env"

// specLaunchConfigReason is why a config-only agent takes no launch
// config: a spec describes flags as templates for the neutral request
// only; it names no typed options and no settings flag.
const specLaunchConfigReason = "config-only agent: a spec has no typed launch options and no settings flag; pass vendor flags as raw args"

// FromSpec builds a config-only agent from s (SPEC §5.6). Validation:
// Spec.Validate. A spec whose ID equals a built-in's is refused by
// builtin.New, not here. Pure.
func FromSpec(s Spec, h Host) (Agent, error) {
	c, err := NewCore(s, h)
	if err != nil {
		return nil, err
	}
	a := specAgent{Core: c}
	if slices.ContainsFunc(s.TokenEnv, func(e TokenEnv) bool { return e.Provider != "" }) {
		return specProviderAgent{specAgent: a}, nil
	}
	return a, nil
}

// HasCapability gates each data-driven capability on its Spec field. Pure.
func (a specAgent) HasCapability(c Capability) bool {
	s := a.spec
	switch c {
	case CapProcessClassifier:
		return !s.Process.IsZero()
	case CapLoginCommander:
		return len(s.LoginArgs) > 0
	case CapHomeLinker:
		return len(s.SharedHomeFiles) > 0
	case CapBinaryVersion:
		return len(s.VersionArgs) > 0
	case CapTokenLogin:
		return len(s.TokenEnv) > 0
	case CapDoctor, CapInstaller:
		return true
	}
	return false
}

// UnsupportedReason declares why a capability is missing. Pure.
func (a specAgent) UnsupportedReason(c Capability) (string, bool) {
	if a.HasCapability(c) {
		return "", false
	}
	switch c {
	case CapProcessClassifier:
		return "spec has no process match", true
	case CapLoginCommander:
		return "spec has no login args", true
	case CapHomeLinker:
		return "spec has no shared home files", true
	case CapBinaryVersion:
		return "spec has no version args", true
	case CapTokenLogin:
		return specTokenLoginReason, true
	case CapLaunchConfig:
		return specLaunchConfigReason, true
	}
	return configOnlyReason, true
}

// BinaryVersion runs the version args through Host.Exec (the accessor is
// the door). Process · Read.
func (a specAgent) BinaryVersion(ctx context.Context) (Version, error) {
	return a.Runner().Version(ctx)
}

// TokenKinds lists the kinds token_env names, in spec order, each once;
// the first is the default a caller offers. Pure.
func (a specAgent) TokenKinds() []TokenKind {
	var out []TokenKind
	for _, e := range a.spec.TokenEnv {
		if !slices.Contains(out, e.Kind) {
			out = append(out, e.Kind)
		}
	}
	return out
}

// DetectTokenKind never recognises a kind: a spec describes no token
// shapes, and guessing from a prefix could pick the wrong variable. The
// caller names the kind. Pure.
func (specAgent) DetectTokenKind(Token) (TokenKind, bool) { return TokenKindUnknown, false }

// TokenEnv sets the token_env variable for c: the exact (kind, provider)
// entry, else the only entry of c's kind when c names no provider. A login
// token_env does not describe is ErrUnsupported, worded as Launch words
// it. The scrub (token_env scrub, or credential_env minus the variable) is
// applied by Launch, not returned here. Pure.
func (a specAgent) TokenEnv(c TokenCredential) ([]EnvChange, error) {
	e, err := a.spec.tokenLogin(c)
	if err != nil {
		return nil, err
	}
	return []EnvChange{EnvSetSecret(e.Env, c.Token)}, nil
}

// TokenSetupCommand: a spec describes no token setup command (tokens are
// made in the vendor's own UI). Pure.
func (specAgent) TokenSetupCommand(Home) (Command, bool) { return Command{}, false }

// TokenProviders lists the providers token_env names for kind k, in spec
// order, "" for an entry that names none (that entry takes a credential
// without a provider). nil = token_env has no entry of kind k. Pure.
func (a specProviderAgent) TokenProviders(k TokenKind) []string {
	var out []string
	for _, e := range a.spec.TokenEnv {
		if e.Kind == k {
			out = append(out, e.Provider)
		}
	}
	return out
}
