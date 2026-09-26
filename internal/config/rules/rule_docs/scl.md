#### Siemens SCL Review Principles
> Focus on defects that can change PLC behavior, safety, determinism, or runtime reliability. SCL (Structured Control Language) is a Pascal-based textual PLC language aligned with IEC 61131-3 Structured Text. Do not flag ordinary Pascal-like syntax or vendor-specific conventions unless the changed code demonstrates a concrete correctness problem.

#### Control Flow and State
- Conditions that accidentally omit a required branch, especially when the omitted state leaves an output, command, or state variable holding a stale value
- `CASE` statements that do not cover an expected value when the uncovered state can produce unsafe or incorrect machine behavior
- State-machine transitions that make a state unreachable, create an unintended transition loop, or leave the machine with no valid successor
- Repeated writes to the same control variable in one scan where later assignments can silently override an earlier decision

#### Loops and Scan-Time Safety
- Loops whose termination depends on mutable PLC state without a clear progress condition
- Unbounded or unexpectedly large loops in cyclic program execution when they can exceed the expected scan-time budget or starve time-critical logic
- Busy-wait loops used where a state transition, timer, or other PLC-supported sequencing mechanism is needed

#### Data Types and Numeric Correctness
- Implicit conversions or narrowing assignments that can truncate a value, change signedness, or lose precision in a way that affects control logic
- Arithmetic whose range can exceed the target data type, especially counters, timers, indexes, and accumulated values
- Array indexing that can reach outside the declared bounds because the index is derived from runtime input or an unchecked calculation
- Comparisons between values with incompatible or unintended types where the result can differ from the programmer's apparent intent

#### I/O and Persistent Data
- Writing outputs or persistent state before required validity, mode, or interlock checks have completed
- Reading an input once and then using a stale snapshot across logic that can change the relevant machine state within the same scan
- Resetting, overwriting, or reinitializing retained/process data unexpectedly, causing loss of state across cycles or restarts

#### Error Handling and External Blocks
- Ignoring a status/error result from a called block or communication operation when the caller continues as though the operation succeeded
- Using data produced by a block, timer, counter, or communication interface before establishing that the result is valid for the current state
- Error paths that leave an actuator command, mode bit, or state variable in an unsafe or contradictory condition

#### Maintainability with Behavioral Impact
- Duplicated control conditions that can diverge after a change and cause two parts of the sequence to make conflicting decisions
- Magic constants used for operational limits where the value is part of a safety, timing, or process invariant and can be changed independently of the corresponding logic
- Dead branches or unreachable code when their presence can hide a missing transition or an incomplete safety condition
