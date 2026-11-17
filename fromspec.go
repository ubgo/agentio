package agentio

import "context"

// specAgent is a config-only agent: a Core plus the capabilities a Spec can
// describe, each gated on its data (CapabilityGate).
type specAgent struct{ *Core }

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
)

// configOnlyReason is why a config-only agent lacks code-backed capabilities.
const configOnlyReason = "config-only agent: a spec describes L1 only; this needs code"

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
	return specAgent{Core: c}, nil
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
