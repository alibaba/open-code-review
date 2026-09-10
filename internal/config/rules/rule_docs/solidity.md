#### Solidity Review Principles
> Favor precision over recall: report only issues likely to cause loss of funds, incorrect accounting, permanent lockup, or a security vulnerability on a reachable path. Read the `pragma` before raising any arithmetic finding — Solidity `>=0.8` reverts on overflow and underflow by default, so reporting unchecked wraparound outside an `unchecked` block is a false positive. The `.sol` extension is also used for Gerber PCB solder-mask layers; if the file is not Solidity source, report nothing.

Do not duplicate what `solc`, Slither, or solhint already flag unless the diff creates concrete correctness impact. Before reporting non-local behavior, verify the caller, the modifier chain, the inherited implementation, and the storage layout rather than inferring them from names.

#### Obvious Typos or Spelling Errors
- Misspellings in declaration sites that become part of the contract's surface: function, event, error, state variable, parameter, struct field, or `constant` names
- Do not report typos inside comments, NatSpec prose, or revert strings unless the string is matched on elsewhere

#### External Calls and Reentrancy
- State written after an external call rather than before it, where the callee can re-enter and observe stale state (Checks-Effects-Interactions)
- Cross-function reentrancy: the entry point carries `nonReentrant` but a sibling function sharing the same storage does not
- Cross-contract reentrancy where two contracts share accounting state and only one guards the path
- Read-only reentrancy: a `view` getter consulted by a third party while a callback is in flight returns a value derived from half-updated state
- ERC777/ERC1155/ERC721 transfer hooks and `receive`/`fallback` treated as inert; any token transfer to an arbitrary address is an external call
- A guard added to the wrapper while the `internal` function it delegates to is also reachable from an unguarded path

#### Unchecked Call Results
- Return value of `.call`, `.send`, or `.delegatecall` discarded, so a failed call proceeds as success
- A low-level call to an address with no deployed code returns `true`; missing an `extcodesize` or equivalent existence check before trusting it
- ERC20 `transfer`, `transferFrom`, or `approve` used directly on tokens that return nothing or return `false` instead of reverting, where `SafeERC20` is required
- Success flag captured into a variable that is never asserted on

#### Delegatecall and Proxy Upgradeability
- `delegatecall` to a target that a caller can influence, or to an address read from mutable storage without an allowlist
- Storage-slot collision between proxy and implementation, or between an old and a new implementation: variables reordered, inserted, or removed, and consumed `__gap` slots not reduced by the same count
- A variable that became `constant` or `immutable`, or stopped being one, across an upgrade — every subsequent slot shifts
- Implementation contract left initializable: `initializer` modifier missing, `_disableInitializers()` absent from the constructor, an inherited `__X_init` never called, or a `reinitializer` version reused
- `selfdestruct` or `delegatecall` reachable in an implementation, which can brick every proxy pointing at it
- Function-selector collision or shadowing between the proxy and the implementation, silently routing a call to the wrong body
- Upgrade authorization (`_authorizeUpgrade`, `onlyProxy`) missing or guarded by a role that is not the intended one

#### Access Control
- Privileged function missing its `onlyOwner`/`onlyRole` modifier, or carrying a modifier that does not gate the value it is meant to gate
- `tx.origin` used to authenticate a caller
- Unprotected initializer, setter, or minting entry point
- Single-step ownership transfer to an address that is never verified, where a two-step handshake is required
- Role admin left as the role itself, letting members grant the role to anyone
- A `public`/`external` visibility change on a function that was previously `internal`

#### Arithmetic and Conversions
- An `unchecked` block whose bound is not locally provable — a loop counter compared against `.length` is fine, a subtraction on caller-supplied input is not
- Downcast (`uint256` to a narrower type, or signed/unsigned conversion) that truncates or flips sign on a reachable value
- Division before multiplication, compounding precision loss in a fee, share, or interest calculation
- Token decimals assumed to be 18, or two tokens' decimals assumed equal
- Rounding that favors the caller over the protocol on both deposit and withdrawal paths

#### Storage and Initialization
- Uninitialized local `storage` pointer, which writes to slot 0
- A `memory` copy of a struct or array mutated where `storage` was intended, so the write is discarded
- `delete` applied to a struct or array containing a mapping, which leaves the mapping populated
- `immutable` or `constant` expected but the value is set in a function that can run more than once

#### Ether and Token Handling
- `transfer` or `send` relying on the 2300-gas stipend against a recipient that may be a contract
- Push payments in a loop where one reverting or gas-hungry recipient blocks every other recipient; a pull pattern is required
- Fee-on-transfer or rebasing tokens credited with the amount argument rather than the measured balance delta
- `msg.value` read inside a loop or a `payable` multicall, letting one deposit be counted many times
- Contract that can receive Ether with no withdrawal path, or a `receive`/`fallback` that reverts on a path the protocol depends on

#### Randomness, Time, and Oracles
- `block.timestamp`, `block.number`, `blockhash`, or `block.prevrandao` used as a randomness source for anything of value
- A spot pool reserve, `getReserves`, or a single-block TWAP read as a price, which is flash-loan manipulable
- Chainlink `latestRoundData` consumed without checking staleness (`updatedAt`), a positive answer, or round completeness
- Timestamp comparisons that assume a fixed block interval

#### Front-Running and MEV
- Swap, mint, or redeem without both a deadline and a minimum-output (or maximum-input) bound
- The ERC20 approve race, where a non-zero allowance is overwritten with another non-zero value
- A value derived from state an attacker can move within the same block or transaction bundle
- Commit-reveal or auction logic whose commitment can be front-run because it omits the sender or a salt

#### Gas and Denial of Service
- A loop over an array or mapping-backed list that any caller can grow without bound
- An external call, a `SLOAD`-heavy read, or a token transfer executed inside a loop
- Unbounded array growth in storage with no removal or pagination path

#### Events and Signatures
- A state change with no event emitted, or an event whose `indexed` fields do not identify the affected party
- `abi.encodePacked` over two or more dynamic arguments feeding a hash, allowing collisions; use `abi.encode`
- A signature accepted without a nonce, a chain id, or an EIP-712 domain separator, permitting replay across accounts, chains, or contracts
- `ecrecover` used without rejecting the zero address and without rejecting the malleable high-`s` form
- Signature verification that does not bind the signed payload to the caller or the specific action

#### Inline Assembly
- Assembly that writes past the free memory pointer, or fails to update it after allocating
- `mload`/`mstore`/`calldatacopy` with an offset or length derived from calldata without a bounds check
- A block annotated `memory-safe` that does not satisfy the memory-safety rules
- Storage slots computed by hand that overlap a compiler-assigned slot

#### Testing Correctness
- A fuzz test whose `vm.assume` or bound filters out exactly the domain the property is about
- A test asserting nothing about the behavior named in the test, or asserting only that a call did not revert
- A fork test pinned to no block number, making it non-reproducible
- Mocks that always return success, hiding the failure path the change introduced
