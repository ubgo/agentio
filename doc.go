// Package agentio is the vendor-neutral core of a typed library for
// coding-agent CLIs (Claude Code, Codex, and agents described by data).
//
// It holds three things:
//
//   - L1 core: Agent, Home, Target, Command, Host, Spec / FromSpec and the
//     Spec-driven Core that vendor packages embed for their neutral view.
//   - L2 shared capabilities: one small interface per concept both vendors
//     share (usage, conversations, live sessions, …) and one typed accessor
//     per interface (UsageOf, ConversationsOf, …), so callers never write a
//     type assertion.
//   - Shared value types: redacting secrets (Token, EnvValue, Personal),
//     versions and compatibility, presence-tracked optionals, warnings, cost
//     classes and the neutral error sentinels.
//
// Invariants (SPEC §9): this module imports only the standard library and no
// vendor module; nothing reads the real machine except HostFromOS; tokens
// never appear in argv, errors, String(), JSON or logs.
package agentio
