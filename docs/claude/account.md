# Claude account and identity

Who is logged into a Claude home, on which plan, and through which store: the `claude` module answers from disk (the `oauthAccount` record in `.claude.json` plus the stored credential), decodes the profile endpoint's organization, and can ask the `claude` binary itself through the Runner door. Use it for an account switcher, a "which home is my work account" screen, or a status line. Personal values (email, names, org) come back as [`agentio.Personal`](ds:block?id=personal-yfzy8bms) and print redacted.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A `Host` with `UserHome`, `User` and `Secrets` for the disk reads; `Host.Exec` only for Step 3.
- A home where Claude Code has logged in at least once (`.claude.json` with `oauthAccount`, plus a credential; see [Claude credentials and tokens](credential.md)).

The snippets come from [`examples/claudeaccount`](../../examples/claudeaccount/), which reads the checked-in 2.1.325 fixture and answers `claude auth status --json` from a canned process runner.

## Step 1 — Read the identity

[`a.Identity(h)`](ds:block?id=claude-agent-identity-5vqz5ezp) returns a `claude.Identity`: the neutral `agentio.Identity` (`LoggedIn`, `Email`, `Org`, `PlanLabel`, `Billing`, `Credential` store, `ExpiresAt`) plus `Account` (the full `*claude.OAuthAccount`) and `Credential` (a `*claude.CredentialSummary`: source, location, expiry, scopes, plan; never the token). [`Neutral()`](ds:block?id=claude-identity-neutral-t8ajmes2) gives the shared view that [`agentio.IdentityOf`](../capabilities/install-identity.md#step-3--read-the-signed-in-identity) returns for any agent. Cost: disk plus a keychain lookup; never a network call and never runs `claude`.

[`LoggedIn` needs a credential](ds:block?id=claude-agent-identity-5vqz5ezp): a home that has an account record but no stored login reads as logged out, with `Email` and `PlanLabel` still filled from the record. The plan label prefers the account's organization type and tier and falls back to the credential's.

<!-- snippet: examples/claudeaccount/main.go#identity -->
```go
// readIdentity prints who is logged into h. Email and Org are
// agentio.Personal: they print redacted unless revealed on purpose.
func readIdentity(w io.Writer, a *claude.Agent, h claude.Home) error {
	id, err := a.Identity(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "identity: logged in %t, plan %q, billing %s, credential %s\n", id.LoggedIn, id.PlanLabel, id.Billing, id.Neutral().Credential)
	fmt.Fprintf(w, "  email %v, org %v\n", id.Email, id.Org)
	if s := id.Credential; s != nil {
		fmt.Fprintf(w, "  summary: %s, expires %s\n", s.Source, s.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}
```

## Step 2 — Read the OAuth account

[`a.Account(h)`](ds:block?id=claude-agent-account-fy6cq5vn) returns the `oauthAccount` record of the home's `.claude.json` as a `*claude.OAuthAccount` (nil when Claude Code never logged in there): `OrganizationType` (a `claude.OrgType` such as `claude.OrgMax`), `BillingType`, rate-limit tiers, `PlanDisplayName`, `SeatTier`, roles, `HasExtraUsageEnabled`, account and subscription dates, trial fields, and `Unknown` for keys this version does not model. [UUIDs, email and names are `agentio.Personal`.](ds:block?id=claude-oauthaccount-jqh4rx7y)

[`claude.PlanLabel(subscriptionType, rateLimitTier)`](ds:block?id=claude-planlabel-8fuqam6b) renders the plan the way claude.ai does: `"claude_max"` (the `claude_` prefix is dropped) with `"default_claude_max_20x"` is `Max (20x)`; an empty subscription is `""`. [`claude.ParseProfile(body)`](ds:block?id=claude-parseprofile-eg8cknwb) decodes an `/api/oauth/profile` body into an `agentio.Subscription` (`Plan`, `Status`, `BillingType`, `StartedAt`) without decoding personal fields; a body with no organization is `claude.ErrNoOrganization`. [`a.Subscription(ctx, h)`](ds:block?id=claude-agent-subscription-rtkahzpe) fetches that body for a home's login (Network · Read; see [Claude plan usage](usage.md)).

<!-- snippet: examples/claudeaccount/main.go#account -->
```go
// readAccount reads the oauthAccount record of .claude.json and decodes a
// profile body; a body without an organization is ErrNoOrganization.
func readAccount(w io.Writer, a *claude.Agent, h claude.Home) error {
	acct, err := a.Account(h)
	if err != nil {
		return err
	}
	if acct == nil {
		fmt.Fprintln(w, "account: none recorded")
		return nil
	}
	tier := ""
	if acct.OrganizationRateLimitTier != nil {
		tier = *acct.OrganizationRateLimitTier
	}
	fmt.Fprintf(w, "account: org type %s, billing %s, plan %s\n", acct.OrganizationType, acct.BillingType, claude.PlanLabel(string(acct.OrganizationType), tier))
	sub, err := claude.ParseProfile([]byte(profileJSON))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "profile: plan %s, status %s, billing %s, since %s\n", sub.Plan, sub.Status, sub.BillingType, sub.StartedAt.Format(time.DateOnly))
	if _, err := claude.ParseProfile([]byte(`{}`)); errors.Is(err, claude.ErrNoOrganization) {
		fmt.Fprintln(w, "profile without organization:", err)
	}
	return nil
}
```

## Step 3 — Read auth status through the Runner

[`a.Runner().AuthStatus(ctx, h)`](ds:block?id=claude-runner-authstatus-ktzmqmsf) runs `claude auth status --json` for the home through `Host.Exec` and decodes it into a `claude.AuthStatus`: `LoggedIn`, `AuthMethod`, `APIProvider`, `ConfigDirectory`, `ProjectsDirectory`, `LoginExpiresAt`, `SubscriptionType`, `PlanTier`, `StoredClaudeAILogin`, the personal `Email`, `OrgID`, `OrgName`, and `Unknown`. [`claude.ParseAuthStatus(b)`](ds:block?id=claude-parseauthstatus-3kuerg58) decodes output you captured yourself. This is the binary's own view (it also sees environment logins the disk reads cannot), but it runs a process that may touch the login, so the library never calls it implicitly: see [Claude Runner](runner.md).

<!-- snippet: examples/claudeaccount/main.go#authstatus -->
```go
// readAuthStatus asks the claude binary through the Runner door. The door
// runs `claude auth status --json` via Host.Exec, here a canned fake.
func readAuthStatus(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) error {
	st, err := a.Runner().AuthStatus(ctx, h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "auth status: logged in %t via %s (%s), plan tier %s, email %v\n", st.LoggedIn, st.AuthMethod, st.APIProvider, st.PlanTier, st.Email)
	again, err := claude.ParseAuthStatus([]byte(authStatusJSON))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed offline: subscription %s, unknown keys %d\n", again.SubscriptionType, len(again.Unknown))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeaccount
```

The lines for this page (the first part of the output is on [Claude credentials and tokens](credential.md)):

```text
identity: logged in true, plan "Max (20x)", billing subscription, credential file
  email [redacted], org [redacted]
  summary: file, expires 2026-11-13T20:56:01Z
account: org type claude_max, billing stripe_subscription, plan Max (20x)
profile: plan Max (20x), status active, billing stripe_subscription, since 2025-01-02
profile without organization: claude: profile response has no organization
auth status: logged in true via claude.ai (firstParty), plan tier max_20x, email [redacted]
parsed offline: subscription max, unknown keys 0
```

## Pitfalls

- **[Personal values print redacted on purpose.](ds:block?id=personal-yfzy8bms)** `fmt`, `%v`, `%#v`, JSON and `slog` all show `[redacted]` for `agentio.Personal`; call `Reveal()` only where you mean to show the value to its owner.
- **An account record does not mean logged in.** [`Identity` reports `LoggedIn` false when no credential is found](ds:block?id=claude-agent-identity-5vqz5ezp), even if `.claude.json` still names an account (a logout keeps the record).
- **[`Account` is nil, not an error, for a home that never logged in.](ds:block?id=claude-agent-account-fy6cq5vn)** Check the pointer before reading fields.
- **`AuthStatus` may touch the login.** The vendor documents that it creates `~/.claude.json` when missing; whether it refreshes a token is unverified. Prefer `Identity` in loops and status lines; call `AuthStatus` when the user asks.
- **[The Runner needs `Host.Exec`.](ds:block?id=claude-runner-run-af4j3992)** A host without a process runner returns an `agentio.ErrUnsupported` error; a non-zero exit is an error carrying a short stderr snippet.
- **[`ParseProfile` reads the organization only.](ds:block?id=claude-parseprofile-eg8cknwb)** Personal fields of the profile body are not decoded; `Subscription.Raw` keeps the body when you fetch it with `a.Subscription`.

## Related

- [Claude credentials and tokens](credential.md), [Claude plan usage](usage.md), [Claude Runner](runner.md), [Claude global config](global-config.md)
- Neutral view: [Install and identity](../capabilities/install-identity.md#step-3--read-the-signed-in-identity), [Usage and subscription](../capabilities/usage.md#step-3--read-the-subscription)
- Example: [`examples/claudeaccount`](../../examples/claudeaccount/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
