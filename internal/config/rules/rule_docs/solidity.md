> Favor precision over recall: report only issues that are likely to cause incorrect behavior, loss of user funds, or security vulnerabilities. Smart-contract defects are high-stakes and usually irreversible, so prioritize findings with a concrete exploit path over style suggestions. Account for the Solidity version: arithmetic overflow/underflow checks are built into the compiler from 0.8.0 onward, so only flag unchecked blocks, explicit type casts that truncate, or code pinned below 0.8.0.

#### Reentrancy
- External calls (ETH transfers, calls to untrusted contracts, or token callbacks such as ERC-721 onERC721Received) made before state variables are updated, letting a malicious contract re-enter and drain funds or corrupt state
- Use of low-level call or delegatecall that forwards all remaining gas without a reentrancy guard (checks-effects-interactions ordering, a nonReentrant modifier, or a pull-payment design)
- A token balance or accounting value read after an external call that can be manipulated mid-execution, or state written after the interaction instead of before

#### Integer Overflow and Underflow
- Arithmetic inside an unchecked block, or in a contract pinned below 0.8.0, where an operation can wrap and change a balance, supply, or loop bound
- Explicit casts or conversions between integer types that silently discard high-order bits and change a value used for accounting or access control
- Division or modulo by a value that can be zero at runtime

#### Access Control and Authorization
- Privileged functions (minting, ownership transfer, withdrawal, upgrade, pause) missing an onlyOwner or role check, or checking the wrong role
- Use of tx.origin for authorization, which is spoofable through a chain of intermediate calls
- Missing or incorrect checks that let an arbitrary caller become owner/admin, or an ownership transfer to the zero address that permanently orphans the contract

#### Unchecked External Calls and Error Handling
- A low-level call, staticcall, or delegatecall whose boolean return value is ignored, so a silently failing transfer or call goes unnoticed
- Use of send or transfer for value movement without checking the return value, or assuming it always succeeds (send forwards only 2300 gas and can fail)
- delegatecall into an untrusted or user-controlled address, which executes code in the caller's storage context
- selfdestruct (or the post-0.8.24 replacement) reachable by an unauthorized account

#### Economic and Oracle Safety
- A hardcoded price or a single on-chain spot price used as an oracle, which is manipulable by flash loans or sandwich attacks
- Accounting that mints, burns, or transfers tokens without updating the corresponding balance or supply, or that updates them in the wrong order
- Missing slippage protection on swaps, or a function that lets an arbitrary caller specify the beneficiary or recipient

#### Gas and Denial of Service
- Unbounded loops over user-controlled arrays that can exceed the block gas limit and permanently lock a function
- A function that sends ether in a loop where a single failing recipient reverts the whole transaction, allowing griefing

#### Correctness and Storage
- Uninitialized storage pointers, or storage/memory/calldata location errors that alias two variables to the same slot
- A fallback or receive function with logic that can be triggered unexpectedly, or a payable function that lets the contract receive ether with no recovery path

#### Signature and Randomness
- Signature verification that does not protect against replay or malleability, or that recovers the wrong signer
- Use of block.timestamp or blockhash for randomness, or for any value that must be unpredictable or hard to manipulate
