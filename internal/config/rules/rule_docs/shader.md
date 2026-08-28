> Favor precision over recall: only raise an issue when you are confident it is a real defect on the target shading language and pipeline stage; stay silent when the surrounding pipeline setup (bindings, render pass, vertex layout) is defined outside this file and cannot be verified. Treat correctness and security findings as blocking, and style or naming suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in uniform/binding names, semantic names (HLSL `SV_*`), varying/interpolant names, or shader entry-point names at their declaration sites
- Typos in preprocessor macros or `#include` paths that would fail to compile or silently pull in the wrong header

#### Precision, Range, and Numeric Correctness
- Low/`half`/`mediump` precision qualifiers used for values that need full range or precision (depth, world-space position, accumulation buffers), risking banding or z-fighting
- Division, `rsqrt`, `normalize`, `log`, or `pow` calls on values that can be zero or negative without a guard, producing `NaN`/`Inf` that propagates through the pipeline
- Implicit or explicit type conversions between `float`/`half`/`int`/`uint` that truncate or wrap in a way that changes shading results, especially in loop counters and texture indices
- Non-uniform control flow around `pow`, `log`, or matrix operations that assumes IEEE-754 behavior not guaranteed across all target GPUs/drivers

#### Texture and Buffer Access Safety
- Texture, buffer, or resource array indices computed from vertex/instance/thread IDs or user data without a bounds check, particularly on `RWStructuredBuffer`/`RWBuffer`/`ssbo`/`storage` writes
- Texture sampling (`tex2D`, `sample`, `textureLod`) called inside non-uniform control flow (e.g. inside an `if` that diverges per-lane) without an explicit LOD, which is undefined or produces incorrect derivatives on some hardware
- Mismatched texture format, channel count, or sRGB/linear color space assumptions between the shader and the resource binding declared on the host side
- Out-of-bounds writes to shared/groupshared/threadgroup memory in compute shaders, or missing barriers (`GroupMemoryBarrierWithGroupSync`, `barrier()`, `workgroupBarrier()`) before reading data another invocation wrote

#### Binding, Layout, and Cross-Stage Contracts
- Uniform/constant buffer, descriptor set, or binding-slot indices that do not match the layout the host application (or a companion shader stage) expects, since mismatches fail silently at runtime rather than at compile time
- Struct layout (`std140`/`std430` in GLSL, register/space in HLSL, `@binding`/`@group` in WGSL) whose field alignment or padding does not match the CPU-side struct, causing misread values
- Vertex output / fragment input (varyings, `SV_Position`, `[[stage_in]]`) whose interpolation qualifiers (`flat`, `noperspective`, `centroid`) are inconsistent with how the value is used downstream
- Shader variants driven by preprocessor defines or pipeline permutations where a new code path is not covered by all defined permutations, leaving some variants uncompiled or behaviorally inconsistent

#### Concurrency and Compute Correctness (Compute/Kernel Shaders)
- Race conditions on shared/groupshared/threadgroup memory or storage buffers written by multiple invocations without atomics or synchronization
- Workgroup/threadgroup size assumptions hardcoded in the shader that do not match the dispatch size declared on the host side
- Atomic operations used on types or backends that do not actually support atomics for that format, or atomics used where a simple reduction would be both correct and faster
- Divergent branches or early `return`/`discard` inside a compute kernel placed before a required barrier, causing a deadlock or undefined synchronization on affected lanes

#### Performance Anti-Patterns
- Expensive operations (dynamic branching, texture-dependent reads, transcendental functions) inside tight loops that could be hoisted, precomputed on the CPU, or baked into a lookup texture
- Dynamic (non-uniform) branching on GPUs where the target hardware executes both sides of a branch per-warp/wavefront, negating the intended savings
- Redundant texture fetches or matrix multiplications recomputed per-fragment/per-thread that are invariant across the invocation and could be computed once (e.g. in the vertex stage or as a uniform)
- Overly large local/register usage (long-lived temporaries, unrolled loops) that reduces occupancy without a documented profiling justification

#### Security-Sensitive and Portability Concerns
- Shader code paths that read attacker- or user-controlled buffer sizes/offsets (e.g. from a compute dispatch driven by untrusted input) without validating them before use as an index
- Vendor-specific intrinsics or extensions (`GL_ARB_*`, `SV_Barycentrics`, wave/subgroup intrinsics) used without a fallback or capability check, breaking portability across GPUs that lack the extension
- `discard`/`clip` used to implement alpha testing in a way that defeats early-Z/early-depth-test optimizations without a documented performance tradeoff
