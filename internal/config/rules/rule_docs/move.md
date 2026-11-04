#### Move Review Principles
> Favor precision over recall: report only issues likely to cause loss of funds, permanent lockup, unauthorized access, or a security vulnerability on a reachable path. Move is a resource-oriented language: resources are linear values that cannot be copied or dropped implicitly, and every `abort` unwinds the whole transaction. The `.move` extension is used by both Sui and Aptos (and other Move-based chains); when a finding depends on chain-specific semantics (object model vs. global storage), verify the framework imports (`sui::` vs. `aptos_std::`) before reporting. Do not report codegen or generated bindings.

Do not duplicate what the Move compiler, `aptos move prove`/Prover, or linters already flag unless the diff creates concrete correctness impact. Before reporting non-local behavior, verify the caller, the abilities on the type, and the storage/object layout rather than inferring them from names.

#### Obvious Typos or Spelling Errors
- Misspellings in declaration sites that become part of the module's public surface: public/entry function, struct, field, or constant names
- Do not report typos inside comments or doc prose unless the string is matched on elsewhere

#### Asset and Balance Handling
- A custom vault/withdrawal entry that takes a caller-specified `amount` out of a shared `Balance`/pool and moves it without checking against the caller's deposited balance, letting them drain more than they own. The framework entry points themselves never allow this: Sui `coin::split`/`coin::take` and `balance::split` abort (`ENotEnough`) when the caller lacks the balance. Object delivery on Sui is `transfer::transfer<T: key>` (restricted to the module that defines `T` — the bytecode verifier rejects transfers from other modules even inside the same package) / `transfer::public_transfer<T: key + store>` (available to other modules once the required abilities are present); coin-to-address delivery goes through the funds accumulator — `coin::send_funds` converts the whole coin to a `Balance` and calls `balance::send_funds`, which pushes it into the recipient's accumulator, and there is no `balance::transfer`. Aptos `coin::withdraw`/`coin::transfer` likewise move only out of the signer's own `CoinStore` and abort on insufficient balance
- A wrapped-coin or escrow that credits the depositor using the transferred amount argument rather than the measured balance delta, so fee-on-transfer or rebasing tokens over-credit
- A withdrawal path that returns the full balance while leaving the withdrawal fee or reserve unaccounted
- `destroy`/`burn` on a coin or object whose value is never reconciled, silently removing supply
- A `Coin`/`Balance` (or Aptos `Coin`) stored in a struct that is not protected from double-spend via `take` + `deposit` mismatches

#### Global Storage and Borrow Safety
- Do not report borrow aliasing or use-after-scope by itself: the Move borrow checker rejects in-function cases at compile time, and a cross-function double mutable borrow aborts the whole transaction atomically (no state corruption, no fund loss — `abort` rolls back all state changes). Distinguish compiler-rejected borrow violations (out of scope) from runtime aborts on a reachable path that have demonstrable application-level consequences, or from a `move_from`/`move_to` (Aptos) sequence that moves a resource out and the module later assumes it still exists on a path that is not a deliberate abort
- A `signer`-gated global-storage write whose signer address is not actually derived from the resource owner being modified
- Sui object ownership: `&mut` object passed where a shared or immutable object is required, letting a transaction mutate state another party depends on without the right constraint

#### Abilities and Resource Safety
- Do not report a struct declared `copy`/`drop` that holds a linear resource (e.g. a Sui `Coin`): the abilities checker requires every field to carry the ability and rejects this at compile time. The reviewer-relevant residual is the next bullet
- `store` added to a struct that now allows arbitrary storage of a capability or authority token
- A key/store mismatch: an object stored under a key that does not match its declared type, or a Sui object used as a global key
- Missing `store` on a struct that is placed inside a Sui object field, producing a compile-time capability that is too narrow or too wide

#### Entry Functions and Signer Validation
- `entry` functions that accept a `signer` but never verify the caller is the intended account (owner, admin, or module signer)
- Public functions callable by anyone that change privileged state without an authorization check
- A two-signer or multi-signer flow where only one signature is validated
- A `&mut` object or `&Coin` argument in an `entry` function that lets callers pass any object they control to a privileged path

#### Integer Arithmetic
- `u64`/`u128` arithmetic that can overflow or underflow on a reachable value; Move aborts on overflow by default, so verify the operands are bounded before reporting wraparound
- Division before multiplication compounding precision loss in a fee, share, or exchange-rate calculation
- Unchecked arithmetic in `spec` blocks or in code that deliberately uses `unchecked` wrappers without a proven bound
- Rounding that favors the caller over the protocol on both deposit and withdrawal paths

#### Access Control and Authority
- A capability (`Cap<...>`) or admin address stored in a struct that can be transferred or copied freely, letting a caller mint authority
- `assert!(signer::address_of(signer) == admin, ...)` patterns where `admin` is read from mutable storage without an upgrade or governance path
- Missing two-step ownership transfer where the new owner is never verified

#### Randomness and Oracles
- A value derived from transaction/epoch time, block number, or a locally generated "random" seed used for anything of value (lottery, raffle, matchmaking)
- A spot price or reserve read from a single pool/order-book without manipulation resistance when it gates liquidation or mint/burn

#### Upgrade and Compatibility
- Upgrade compatibility: Sui and Aptos both enforce structural compatibility at upgrade time, and the verifier rejects incompatible struct layout changes (removed/reordered/renamed fields, changed abilities) outright — adding a default value or a backfill path does not cure an otherwise layout-incompatible change. Public API compatibility is enforced by the verifier; non-public implementation functions are not all immutable and some may be changed or removed under the applicable upgrade policy, so do not report verifier-enforced layout/ability/removal rules inside application diffs. The reviewer-relevant residuals are semantic: an upgrade that changes a public function's observable behavior without a migration, or a legacy already-deployed package version that remains callable and can still reach a capability or shared state the new version treats as revoked
- A test helper compiled into production bytecode (i.e. not annotated `#[test_only]`, which Move already excludes from production builds) that is reachable from a production entry point and exposes privileged or dangerous behavior. Do not flag correctly annotated `#[test_only]` declarations; they never ship in production bytecode
- Upgrades that change the semantics of an existing public function without a version bump or migration

#### Testing Correctness
- A test asserting nothing about the behavior named in the test, or asserting only that a call did not abort
- A test that passes because the function under test is never called (dead test)
- A fuzz/property test whose bounds filter out exactly the domain the property is about
