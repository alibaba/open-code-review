> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking. Review only what is observable in the HCL under review; do not infer runtime provider behavior, cloud account configuration, or state stored outside this file.

#### Obvious Typos or Spelling Errors
- Spelling errors in resource/module/variable/output names at their declaration sites; do not report spelling errors at reference sites
- Typos in `description` fields that affect readability of the module's public interface

#### Hardcoded Secrets and Credentials
- A literal password, API key, access key/secret pair, private key, or connection string assigned directly to a resource argument or a `variable`/`locals` default instead of coming from a secret manager, `sensitive` input, or environment-backed data source
- A `.tfvars` file (this file type is the conventional home for real input values, and is frequently committed by accident with production secrets in it) assigning a real-looking secret value rather than a placeholder
- A `variable` block that clearly holds a credential (name/description implies password, token, key, or secret) missing `sensitive = true`

#### Overly Permissive Access
- A security group / firewall / network ACL rule with an unrestricted source (`0.0.0.0/0`, `::/0`, or `"*"`) on a sensitive port (SSH/22, RDP/3389, database ports) or on all ports
- An IAM policy, role, or resource policy granting a wildcard action (`"Action": "*"`) or wildcard resource (`"Resource": "*"`) instead of a scoped permission set
- Public read/write ACLs or public access settings enabled on a storage resource (bucket, blob container) that has no clear public-content purpose stated in the diff

#### State and Lifecycle
- A `terraform.tfstate` or `*.tfstate.backup` file included in the diff — state files can contain resource attributes and secrets in plaintext and should never be committed
- Removing or weakening a `lifecycle { prevent_destroy = true }` block on a resource that looks stateful/critical (database, persistent volume, KMS key) without an explanation in the diff
- A stateful resource (database, storage bucket, KMS key) newly created without any `lifecycle` protection, when sibling resources of the same kind in the diff do have one — an inconsistency worth flagging, not an absolute rule

#### Versioning and Reproducibility
- A `required_providers`/module `source` version constraint left fully unbounded (e.g. no version argument at all, or `>= 0.0.0`) where sibling entries in the same file pin a version — inconsistent, not universally wrong, since some root modules intentionally float
- Do not flag a deliberately wide constraint (e.g. `~>`, a documented range) that is clearly intentional from the surrounding code

#### Style and Structure
- Duplicate resource/data-source labels within the same module (would fail `terraform validate`, if not already caught by other tooling)
- Variables declared but never referenced anywhere in the diff's module, or referenced variables never declared in the diff's scope
- Do not flag formatting/whitespace that `terraform fmt` would silently fix — focus on structural and semantic issues
