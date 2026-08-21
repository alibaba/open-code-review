> Favor precision over recall: report only issues that are likely to cause incorrect behavior, loss of user funds, or security vulnerabilities. Vyper deliberately omits several Solidity footguns: state is written before external calls by default and there is no raw delegatecall, so reentrancy and delegatecall attacks are largely mitigated. Focus on the remaining risks: access control, raw_call misuse, and economic safety. Account for the Vyper version, since built-in arithmetic overflow checks and assert/raise semantics changed across releases.

#### Access Control and Authorization
- Privileged functions (mint, ownership transfer, withdrawal, upgrade) missing a role or ownership check, or asserting the wrong role
- Authorization based on tx.origin rather than msg.sender, which is spoofable through a chain of calls

#### Raw External Calls
- raw_call with max_outsize=0, so the return value is not checked and a silently failing call goes unnoticed
- raw_call that forwards unbounded gas or targets a user-controlled address without validating the result
- Interface or external call declarations whose ABI, argument types, or return types are inconsistent with the target contract

#### Integer and Value Safety
- Arithmetic in a version before built-in overflow checks (0.3.4) that can wrap, or unchecked conversion between uint256, int128, and other types that truncates a value used for accounting
- Division or modulo by a value that can be zero at runtime
- A payable function that lets the contract receive ether with no accounting or withdrawal path

#### Economic Safety
- A hardcoded price or single-source spot price used as an oracle, which is manipulable via flash loans or sandwich attacks
- Mint, burn, or transfer logic that does not update the corresponding balance or supply, or updates it in the wrong order
- Missing slippage protection on swaps, or a recipient/beneficiary that an arbitrary caller can set

#### Assertions and Control Flow
- Use of assert for input validation or conditions that can fail on ordinary user input, since assert consumes all remaining gas, where raise would refund unused gas and revert cleanly
- Unbounded loops over user-controlled arrays that can exceed the block gas limit

#### Block and Randomness
- Use of block.timestamp or blockhash for randomness, or for any value that must be unpredictable
- Signature verification that does not protect against replay or malleability
