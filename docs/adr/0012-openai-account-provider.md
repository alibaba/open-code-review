---
status: accepted
---

# Native OpenAI account provider

OCR needs to support users who authenticate to an OpenAI/Codex account instead
of supplying an OpenAI API key. The existing `openai` provider is API-key based
and must remain compatible with current configurations. A proxy-owned provider
would add another runtime dependency and would hide account authentication from
OCR's own configuration model.

## Decision

Add a separate `openai-account` provider owned by OCR.

The provider uses the official OpenAI OAuth authorization-code flow with PKCE,
stores OCR-owned credentials, and reads the account's model catalog. Requests
use the account's Responses endpoint and carry the account identity headers
required by that service. Access tokens are refreshed before expiry and once
after an authorization failure.

The model catalog is the source for account model discovery. OCR keeps a local
cache so configuration can remain usable when discovery is temporarily
unavailable. The user can choose the model, reasoning effort, and service tier;
the fast-mode label maps to the provider's priority service tier.

The existing `openai` API-key provider, custom providers, and legacy `llm`
configuration keep their current resolution paths.

## Consequences

- OCR can authenticate and call an OpenAI account without CLIProxyAPI.
- Account credentials are secret material and are written with owner-only file
  permissions. OCR never writes to the official Codex CLI credential file.
- Account model availability can change independently of the static API-key
  provider list, so catalog refresh and stale-cache behavior are explicit.
- Account Responses calls use streaming completion events while the shared OCR
  client interface remains unchanged.
- The account provider has more configuration states than API-key providers,
  including token expiry, catalog freshness, and service-tier support.
