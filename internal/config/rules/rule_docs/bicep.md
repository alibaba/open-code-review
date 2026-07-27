> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Review only what is observable in the Bicep under review; do not infer Azure subscription/tenant configuration, deployed resource state, or policy assignments that live outside this file.

#### Obvious Typos or Spelling Errors
- Spelling errors in resource/module/parameter/variable/output names at their declaration sites; do not report spelling errors at reference sites
- Typos in `@description()` text that affect readability of the module's public interface

#### Hardcoded Secrets and Credentials
- A literal password, connection string, API key, or access token assigned directly to a resource property, parameter default, or variable instead of coming from a Key Vault reference (`getSecret()` / `Microsoft.KeyVault/vaults/secrets` resource) or a secure parameter supplied at deployment time
- A parameter whose name or description clearly indicates a credential (password, secret, token, connectionString, apiKey) declared without the `@secure()` decorator, which is what prevents the value from being logged or shown in deployment history

#### Overly Permissive Access
- A `Microsoft.Authorization/roleAssignments` resource granting a broad built-in role (`Owner`, `Contributor`) at subscription or resource-group scope where a narrower, resource-scoped or custom role would suffice, especially when sibling assignments in the same file use narrower scopes
- A network security group rule (`Microsoft.Network/networkSecurityGroups/securityRules`) with `sourceAddressPrefix` set to `*`/`Internet`/`0.0.0.0/0` on a sensitive port (SSH/22, RDP/3389, database ports) or on all ports (`destinationPortRange: '*'`)
- A storage account, key vault, or SQL server resource with `publicNetworkAccess` explicitly set to `'Enabled'` (or left at a default that resolves to public) alongside no compensating `networkAcls`/private-endpoint configuration elsewhere in the same file

#### Insecure Resource Defaults
- A storage account without `minimumTlsVersion` set to a current version, or with `supportsHttpsTrafficOnly` explicitly set to `false`
- A resource property that disables encryption-at-rest or transparent data encryption where the resource type supports enabling it
- Do not flag a resource for merely omitting an optional hardening property when the diff gives no indication either way — only flag an explicit insecure value or an explicit disabling of a secure default

#### Versioning and Reproducibility
- An `api-version` in a resource's type string that is unusually old relative to sibling resources of the same provider in the same diff — inconsistency worth flagging, not an absolute "must be latest" rule
- A module reference (`module ... 'path/to/module.bicep'` or a registry reference) with no version/tag pinning where the surrounding file otherwise pins versions

#### Style and Structure
- Parameters declared but never referenced anywhere in the diff's scope, or referenced parameters/variables never declared in the diff's scope
- Duplicate resource symbolic names within the same file (would fail compilation, if not already caught by other tooling)
- Do not flag formatting/whitespace that the Bicep formatter would silently fix — focus on structural and semantic issues
