#### Verilog and SystemVerilog Review Principles
> Favor precision over recall: report only defects likely to change synthesized hardware behavior, cause simulation/synthesis mismatch, or introduce timing hazards in the changed RTL. Account for whether the file is synthesizable design code or a simulation-only model before raising findings, and do not report style that a linter or formatter already handles. This rule covers Verilog (`.v`), Verilog headers (`.vh`), and SystemVerilog (`.sv`). The `.v` extension is also used by Coq and by the V language; if the file is not Verilog or SystemVerilog, report nothing HDL-specific and fall back to general review principles.

#### Blocking and Non-Blocking Assignments
- Non-blocking assignments (`<=`) used for combinational logic, or blocking assignments (`=`) used for sequential logic inside a clocked `always`/`always_ff` block, where the mixed style changes simulated ordering or infers unintended hardware
- Mixed blocking and non-blocking assignments to the same variable when later statements observe a value different from the intended value because non-blocking updates are deferred
- Do not claim that statement ordering within one procedural block is inherently nondeterministic
- A register or variable intended to have a single procedural driver being written by multiple processes, particularly violations of `always_ff`, `always_comb`, or `always_latch` single-writer rules
- Do not flag intentional multiple drivers on an appropriate net type unless an actual conflict is demonstrated
- Misuse of `always_ff`, `always_comb`, or `always_latch` that violates their event-control, assignment, or single-writer semantics
- Do not report a correctly formed `always @(*)` solely as a style issue

#### Inferred Latches
- A combinational `always`/`always_comb` block where a signal is not assigned on every path (missing `else`, an incomplete `case`, or a `case` without `default`), inferring an unintended transparent latch
- Missing default assignments at the top of a combinational block, so a newly added branch silently reintroduces a latch
- Outputs left unassigned for some input combinations of a decoder, mux, or FSM next-state logic

#### Signal Width and Signedness
- Assignments or comparisons between operands of different bit widths that truncate or zero-extend silently, dropping significant bits or changing a comparison result
- Arithmetic that overflows the declared width of the result, or an intermediate expression narrowed before it is widened
- Mixed signed/unsigned operands where Verilog's context-determined signedness makes a comparison or shift behave unexpectedly; be explicit with `$signed`/`$unsigned`
- Part-selects, concatenations, or replication counts whose width does not match the target, and reliance on implicit `reg`/`wire` width from an undeclared net

#### Clock and Reset Handling
- Reset that is not correctly synchronous or asynchronous as intended, a reset polarity mismatch, or a reset released asynchronously without a synchronizing deassertion (reset-recovery hazard)
- Sequential logic missing the reset in the sensitivity list for an asynchronous reset, or a value that must survive reset being placed on the reset branch
- Gated, derived, or combinationally generated clocks used where a clock enable is intended, and multiple clocks driving the same register
- Registers with no reset where the design assumes a known power-on state

#### Clock-Domain Crossings and Races
- A signal sampled in one clock domain that is driven from another without a synchronizer (two-flop for single-bit control, handshake or asynchronous FIFO for buses), risking metastability
- Multi-bit buses synchronized bit-by-bit, so bits arrive skewed and produce transient invalid values; use gray coding or a handshake
- Combinational feedback loops, or read-during-write races on inferred memory without a defined collision policy

#### Simulation vs. Synthesis and Unsafe Constructs
- Simulation-oriented constructs such as `#delay`, `fork`/`join`, `force`/`release`, or an `initial` block whose behavior is required by the design but is unsupported by the declared target synthesis flow. Do not flag initialization solely by syntax when the target FPGA or synthesis tool documents support for it
- Incomplete or overlapping sensitivity lists in a bare `always @(...)` that make simulation differ from the synthesized combinational function; prefer `@(*)` or `always_comb`
- `casex`/`casez` whose don't-care matching hides priority bugs, or a `case` relying on `x`/`z` matching; prefer `case` with `unique`/`priority` where the intent is exclusive or prioritized
- `$display`/`$finish`/assertions guarding real behavior, and non-synthesizable system tasks left in the design path
- Full-case/parallel-case pragmas that assert properties the logic does not actually guarantee
