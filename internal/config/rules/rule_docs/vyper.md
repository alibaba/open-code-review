#### Vyper Review Principles
> Favor precision over recall: report only issues likely to cause loss of funds, incorrect accounting, or a security vulnerability on a reachable path. Vyper syntax and semantics changed hard at 0.4 — 0.3.x writes `@nonreentrant("key")` and calls externals with plain method syntax, 0.4.x writes bare `@nonreentrant` and requires the `extcall`/`staticcall` keywords — so read the `# pragma version` before applying any version-specific rule. Do not import Solidity idioms: Vyper has no inheritance, no inline assembly, no recursion, no unbounded loops, and arithmetic is checked by default.

#### Obvious Typos or Spelling Errors
- Misspellings in declaration sites that become part of the contract's surface: function, event, struct field, interface method, storage variable, parameter, or `constant` names
- Do not report typos inside comments, docstrings, or `assert` messages unless the message is matched on elsewhere

#### Reentrancy and `@nonreentrant`
- A function that makes an external call before writing state and carries no `@nonreentrant` decorator
- A decorator removed to satisfy the compiler's "cannot call `X` since it is `@nonreentrant` and reachable from `Y`" error: that error means the lock is genuinely nested, and dropping the decorator removes real protection instead of fixing the call graph
- `@nonreentrant` counts as state access, so adding it to a function used from a `@view` context or a pure module changes what the module is allowed to do
- A pinned compiler in the 0.2.15–0.3.0 range, where the reentrancy lock was miscompiled; the version pin is itself the finding, not a style note
- `raw_call` or a token transfer to an arbitrary address treated as inert; any such call can re-enter

#### Language Restrictions
- `for i in range(N)` whose bound `N` is smaller than the collection being iterated: the loop silently truncates rather than reverting, so the tail is never processed
- `raw_call` used as a hand-rolled dispatch table to reconstruct inheritance, losing the compiler's type and mutability checks
- `create_from_blueprint` or `create_copy_of` factories where the blueprint address is mutable or unverified
- Logic that assumes recursion or a dynamically sized loop is available and works around its absence incorrectly

#### External Calls and `raw_call`
- `raw_call` with `max_outsize=0` where the callee's success or return data actually matters
- `revert_on_failure=False` whose returned success flag is dropped instead of asserted
- `delegate_call=True` to a target that is not a compile-time constant
- A state-mutating call made where `is_static_call=True` was intended, or vice versa
- Return data decoded to a fixed `Bytes[N]` smaller than what the callee can return

#### Access Control and Module Initialization
- Vyper has no modifiers, so the `assert msg.sender == self.owner` line is written per function and is easy to omit on one of several privileged entry points
- A `payable` `__default__` that accepts Ether the contract has no way to withdraw
- `@external` where `@internal` was meant, exposing an internal helper
- 0.4 `initializes:` / `exports:` re-exporting more of a module's surface than intended, or an `initializes:` module whose `__init__` is never called
- `__init__` logic reachable a second time through a factory deployment path

#### Arithmetic and Bounds
- Arithmetic is checked by default — do not report overflow as though this were Solidity's `unchecked`. The real findings are the explicit escapes: `unsafe_add`, `unsafe_sub`, `unsafe_mul`, `unsafe_div` used where the bound is not locally provable
- `DynArray[T, N]` appended to past `N`, or a length assumed rather than checked
- `slice`, `concat`, or `extract32` silently bounded by a `Bytes[N]` / `String[N]` capacity smaller than the real input
- A narrowing `convert()` that truncates or flips sign on a reachable value
- Division before multiplication in a fee, share, or interest calculation, and rounding that favors the caller on both deposit and withdrawal

#### Storage Layout and Deployment
- Vyper has no proxy standard, so an upgrade path implies a blueprint or copy deployment; state migration logic that assumes slot compatibility is wrong
- `--storage-layout-file` overrides that no longer match the declared variables after a reorder or removal
- Transient storage reused across calls where the value is expected to be cleared

#### Ether, Tokens, and Oracles
- `send()` relying on the 2300-gas stipend against a recipient that may be a contract; `raw_call` with an explicit gas budget is required
- `self.balance` used as accounting truth, which any forced transfer can move
- Non-bool-returning ERC20s called through an interface without `default_return_value=True`, or through `raw_call` with the result unchecked
- A spot pool reserve or single-block price read as an oracle
- `block.timestamp`, `block.number`, or `blockhash` used as a randomness source

#### Front-Running and MEV
- Swap, mint, or redeem without both a deadline and a minimum-output (or maximum-input) bound
- A value derived from state an attacker can move within the same block

#### Events
- A state change with no corresponding `log`, or an event whose `indexed` fields do not identify the affected party

#### Testing Correctness
- A `boa.env.prank` or `boa.reverts` block that asserts nothing about the behavior it names
- A fuzz test whose bounds exclude the boundary value the property is about
- A test asserting only that a call did not revert
