#### VHDL Review Principles
> Favor precision over recall: report only defects likely to change synthesized hardware behavior, cause simulation/synthesis mismatch, or introduce timing hazards in the changed RTL. Account for whether the file is synthesizable design code or a simulation-only model before raising findings, and do not report style that a linter or formatter already handles.

#### numeric_std and Type Usage
- Arithmetic implemented with non-standard vendor packages such as `std_logic_arith` or `std_logic_unsigned` instead of `numeric_std`, where package-specific overloads make behavior nonportable or ambiguous
- Incorrect explicit conversions between `integer`, `signed`, `unsigned`, and `std_logic_vector`, including incorrect target widths supplied to `to_unsigned`, `to_signed`, or `resize`
- Arithmetic or comparisons whose selected overload or result width does not preserve the intended value
- Width or overload mistakes as distinct from direct incompatible `signed`/`unsigned` mixing, which normally requires an explicit conversion and may be rejected by analysis
- Metavalue-dependent arithmetic or control flow involving `'U'`, `'X'`, or `'Z'` that synthesized hardware cannot reproduce

#### Ranges, Indexing, and Resize
- Vector indexed or sliced outside its declared range, or with the wrong direction (`to` vs. `downto`), causing a runtime error in simulation and wrong bits in hardware
- A `resize` to a smaller width that discards significant information; for `SIGNED`, account for the defined behavior of retaining the original sign bit together with the rightmost part, and for `UNSIGNED`, dropping the leftmost bits
- Off-by-one range arithmetic in `(N-1 downto 0)` declarations and loop bounds, and `others => '0'` aggregates applied to a target of unexpected width

#### Inferred Latches and Process Sensitivity
- A combinational process whose sensitivity list omits signals it reads, so simulation and synthesis disagree; prefer `process(all)` (VHDL-2008) where available
- A combinational process that does not assign every output on every path (missing `else`, incomplete `case`/`when`, no default assignment), inferring an unintended latch
- A branch within a legal, complete `case` statement that fails to assign an output also assigned by the other branches, causing that output to retain its previous value and infer a latch. Do not require `when others` when every value of the selector subtype is explicitly covered; an incomplete VHDL `case` statement is a compile-time error

#### Clock and Reset Handling
- Clocked logic that tests only the clock level (for example, `if clk = '1'`) instead of an edge, or places data-path assignments outside the intended edge condition. Accept both `rising_edge(clk)`/`falling_edge(clk)` and the conventional `clk'event and clk = '1'`/`clk'event and clk = '0'` forms when their semantics are intentional
- Reset that is not synchronous or asynchronous as intended, a reset polarity mismatch, or an asynchronous reset released without synchronized deassertion (recovery/removal hazard)
- Signals that must retain state across reset placed on the reset branch, or registers with no defined reset where a known startup state is assumed
- Gated or derived clocks where a clock enable is intended, and more than one clock driving the same register

#### Clock-Domain Crossings and Resolved Signals
- A signal generated in one clock domain and sampled in another without a synchronizer (two-flop for single-bit, handshake or asynchronous FIFO for buses), risking metastability
- Multi-bit buses crossing domains without gray coding or a handshake, so bits arrive skewed
- Multiple drivers on a resolved signal (`std_logic`) relied on for wired logic, where the resolution function hides an unintended multi-driver conflict; unintended shared drivers on what should be a point-to-point signal

#### Simulation vs. Synthesis and Unsafe Constructs
- Simulation-oriented constructs such as `wait for`, `after` delays, file operations, or reporting side effects whose behavior is required by the design but is unsupported by the declared target synthesis flow. Do not flag initial signal values or assertions solely by syntax when the target tool documents support or safely ignores verification-only statements
- Variables (`:=`) inside a clocked process used where their immediate-update semantics differ from signal (`<=`) semantics and change the inferred register or its ordering
- Reads of a signal in the same process cycle expecting the updated value, ignoring VHDL's postponed signal update (delta-cycle) semantics
- Nonsynthesizable constructs (`access` types, files, unbounded loops, dynamic memory) left in the design path, and metavalue-dependent branches that behave differently in gate-level simulation
