> Favor precision over recall: report only issues likely to cause incorrect behavior, asset loss, unauthorized access, or a reachable abort in production code. Move has several dialects that differ in ways that invert the correct advice, so read `Move.toml` and the module's imports to establish whether the package targets Sui, Aptos, or core Move before raising any finding marked below as dialect-specific. Do not report formatting handled by `move fmt`, and do not restate a diagnostic the compiler already emits.

#### Ability Grants and Resource Discipline
- Abilities granted more widely than the type needs: `copy` on a value whose uniqueness is the invariant, `drop` on a value with a required settle, repay, or destroy path, `store` letting third-party modules nest and move the value out of this module's control
- A struct that gains `drop` where a previously enforced hot-potato or receipt pattern silently becomes a no-op when the value is discarded
- `phantom` type parameters dropped or added where the change alters which instantiations are legal for callers
- Do not report ability grants that match the surrounding module's established pattern without evidence that the wider grant is reachable and harmful

#### Global Storage and Aborts (Aptos and core Move)
- `move_to` on an address that may already hold the resource, which aborts, without an `exists<T>` guard or a documented caller contract
- `move_from`, `borrow_global`, or `borrow_global_mut` on an address that may not hold the resource, without an `exists<T>` check or a typed `error::not_found` abort
- Overlapping mutable global borrows, or a mutable borrow held across a call that re-enters the same resource
- `acquires` annotations that no longer reflect the resources a function reaches after the change
- Aborts raised with a bare integer where the module otherwise uses categorized `error::` constructors, losing the client-visible error category

#### Object Model and Sharing (Sui)
- `transfer::share_object` or `public_share_object` reachable with an object passed in by the caller rather than created in the same transaction: sharing an already-owned object aborts with `ESharedNonNewObject`, so the path is a reachable abort (`share_owned`)
- `transfer::transfer` versus `transfer::public_transfer` chosen inconsistently with the type's `store` ability and the custody the caller expects
- A value sent to `ctx.sender()` where returning it to the caller would compose better and keeps the PTB in control of its destination (`self_transfer`)
- Wrapping a shared or frozen object inside another object, which strands it (`freeze_wrapped`)
- `&TxContext` on a public function that will need mutation later, forcing a breaking signature change (`prefer_mut_tx_context`)
- A parent object's `UID` deleted before its dynamic fields, dynamic object fields, or received children are removed: they are not deleted with the parent and become permanently inaccessible, with no storage rebate

#### Visibility and Authorization
- `entry fun` widened to `public entry fun` on a privileged path (Aptos): other modules can then call it directly, and the compatible upgrade policy freezes it as public. On Sui, `public` is already transaction-callable and `entry` adds nothing to it, so do not report `public entry` by itself
- Visibility widened from `public(package)` (Sui, Move 2024) or `public(friend)` (Aptos and legacy editions) to `public` without a stated reason
- Authorization decided from an `address` argument rather than proven authority: require `&signer` and compare `signer::address_of` on Aptos, or `ctx.sender()` or a capability object on Sui
- Capability objects or `&signer` values passed to functions that do not need them, widening the blast radius of a compromised call path
- Do not report a missing authorization check on a function whose only caller is an already-authorized internal path, unless the function is `public` or `entry`

#### Arithmetic, Vectors, and Gas
- Unchecked subtraction on unsigned integers: Move aborts rather than wrapping, so an underflow is a reachable denial of service on that code path, not a silent wrap
- `as` downcasts that abort when the value exceeds the target width, especially `u256`/`u128` narrowed to `u64` in fee, price, or share math
- Division performed before multiplication in fee, ratio, or share calculations, losing precision that accrues against users
- `vector::borrow`, `vector::remove`, or `vector::swap_remove` with an index not proven within bounds
- Loops over caller-supplied vectors or unbounded tables with no length cap, letting a single transaction exceed the gas limit and making the path unusable

#### Generics and Type Constraints
- Ability constraints wider than the function body requires, which needlessly restricts the types callers may instantiate
- Publicly instantiable `T: store` or `T: key + store` parameters where a caller can substitute an arbitrary type into a privileged path
- One-time-witness types on Sui not verified as genuine before granting the authority they represent
- Type parameters used to key storage or events where two distinct instantiations can collide

#### Errors, Events, and Upgrade Compatibility
- Named `const E*` abort code values changed or renumbered: clients and indexers match on them, so a renumbering is a breaking change even though the code still compiles
- State changes that indexers depend on made without emitting the corresponding event, or an event's fields changed in place
- Struct fields or public function signatures changed in ways that violate Sui upgrade compatibility or Aptos `upgrade_policy = "compatible"`
- Mint, admin, or fixture helpers written for tests (`mint_for_testing`, `create_for_testing`) declared without `#[test_only]`, which ships them in published bytecode
