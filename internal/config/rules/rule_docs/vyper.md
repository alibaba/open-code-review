> Favor precision over recall: report only issues that are likely to cause incorrect behavior, loss of user funds, or security vulnerabilities. Vyper makes external calls explicit and provides built-in reentrancy protection, but that protection is opt-in through `@nonreentrant` unless the contract uses `#pragma nonreentrancy`. Reentrancy remains possible around unprotected external calls, and `raw_call(..., is_delegate_call=True)` supports delegate calls. Focus on access control, reentrancy, `raw_call` misuse, delegate calls, and economic safety while accounting for the Vyper version.

#### Access Control and Authorization
- Privileged functions (mint, ownership transfer, withdrawal, upgrade) missing a role or ownership check, or asserting the wrong role
- Authorization based on `tx.origin` rather than `msg.sender`, which is spoofable through a chain of calls

#### Raw External Calls
- `raw_call(..., revert_on_failure=False)` whose returned success flag is ignored, allowing a failed call to go unnoticed; `max_outsize=0` alone is not an error because failures still revert by default
- `raw_call` that forwards unbounded gas or targets a user-controlled address without validating the result
- Interface or external call declarations whose ABI, argument types, or return types are inconsistent with the target contract

#### Integer and Value Safety
- Use of explicit `unsafe_add`, `unsafe_sub`, `unsafe_mul`, or `unsafe_div` where unchecked arithmetic can corrupt accounting, or a narrowing conversion whose bounds are not validated
- Division or modulo by a value that can be zero at runtime
- A payable function that lets the contract receive ether with no accounting or withdrawal path

#### Economic Safety
- A hardcoded price or single-source spot price used as an oracle, which is manipulable via flash loans or sandwich attacks
- Mint, burn, or transfer logic that does not update the corresponding balance or supply, or updates it in the wrong order
- Missing slippage protection on swaps, or a recipient/beneficiary that an arbitrary caller can set

#### Assertions and Control Flow
- Use of `assert ..., UNREACHABLE` for a condition that can fail during normal execution, because this form emits `INVALID` and consumes the remaining gas; ordinary `assert` and `raise` both revert and return unused gas
- Unbounded loops over user-controlled arrays that can exceed the block gas limit

#### Block and Randomness
- Use of `block.timestamp` or `blockhash` for randomness, or for any value that must be unpredictable
- Signature verification that does not protect against replay or malleability
