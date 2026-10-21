#### CUDA and HIP Kernel Review Principles
> Favor precision over recall: report only issues that produce wrong results, read or write memory out of bounds, hide a failed launch, or hang the device — anything else is noise in a review of GPU code. CUDA and HIP are one programming model with two spellings of the same API, so judge each finding against the header the file actually includes (`cuda*` versus `hip*`) rather than translating one into the other, and read the target architecture before applying anything version-specific: the `_sync` shuffle and ballot intrinsics arrived in CUDA 9, `atomicAdd()` on `double` is not available on devices with compute capability lower than 6.0, and a device-side launch — a `__global__` call from device code, or recursion in device code — is only supported by devices of compute capability 3.5 and higher and needs relocatable device code (`-rdc=true`, linked against the CUDA device runtime). Do not report naming, `<<<...>>>` spacing, or host/device code layout preferences.

#### Launch Configuration and Error Checking
- A grid or block dimension that can evaluate to zero (an empty batch, `ceil_div(0, n)`, a `min` clamp), which fails the launch and invalidates every later operation on that stream
- A block size above the 1024-thread limit, or a `dim3` whose components are computed in 32-bit arithmetic and can overflow
- A launch or runtime call whose result is never checked: without `cudaGetLastError()`/`hipGetLastError()`, `cudaDeviceSynchronize()`, or a `HIPCHECK`-style wrapper, an asynchronous failure surfaces later as garbage output or as an unrelated error
- An error check that reads the error state after another call already updated it; `cudaGetLastError()` consumes the error it reports, so the result must be captured before anything else touches that state
- A `cudaMalloc`/`hipMalloc` result used without a `cudaSuccess` check, leaving a null device pointer behind
- Do not report occupancy, launch overhead, or grid-stride-loop shape: those are tuning, not correctness

#### Shared Memory and Block Synchronization
- Shared memory written by one thread and read by another with no barrier between the write and the read
- `__syncthreads()` inside a branch not every thread of the block takes — a thread that never reaches the barrier waits forever, and the barrier's meaning degrades to undefined behaviour even when the branches happen to reach the same count
- A shared-memory read issued before the `__syncthreads()` that guards the write, including the case where a later refactor (hoisting, an early `return`, a `continue`) moved the read across the barrier
- `extern __shared__` declared in the kernel while the launch passes a different size as its third argument, or passes none
- A `__syncthreads()` relied on to carry a value to another block, which it cannot: the barrier covers only its own block, and cross-block ordering needs `__threadfence()` together with a flag or atomic in a release-acquire handshake — the fence establishes order and visibility, it does not wait for another block to arrive

#### Warp-Level Programming and the Memory Model
- `__shfl`, `__ballot`, `__any`, or `__all` used without the `_sync` form on Volta or later, where independent thread scheduling no longer guarantees a converged warp around a divergent branch
- `__activemask()` treated as a barrier, or its mask handed to a later `_sync` call as the member mask without the caller establishing that it still names the lanes that must participate: the intrinsic has no `_sync` form, reports only the lanes active at that instant, and guarantees no convergence
- Warp-synchronous programming: one lane consuming a register another lane wrote, with no `__shfl_sync` or `__syncwarp()` — it works only by accident on pre-Volta hardware
- A flag or spin variable written by one thread and read by another without `volatile`, `__threadfence()`, or an atomic, so the compiler may keep it in a register and the reader never observes the write
- A non-atomic read-modify-write (`x++`, `x += y`) on a location two threads can touch: histogram bins, counters, and accumulators in a strided loop are the usual cases
- `atomicAdd` on `float` where the resulting accumulation order changes the answer beyond the caller's tolerance, or on `double` in code also built for pre-sm_60 targets

#### Bounds, Shapes, and Type Sizes
- A thread index derived from `blockIdx`, `blockDim` and `threadIdx` used to index an array with no range check covering the partial block of the last iteration
- A kernel that assumes the element count is a multiple of the block size, or that reads `n + 1` elements from a buffer sized `n`
- Shared, register, or constant-memory arrays sized from a host-side constant that can differ between the host and device compilation passes
- `int` indices and sizes where a tensor can exceed 2^31 elements, or a `size_t` narrowed to `int` at the launch site
- A grid-stride loop whose `gridDim.x * blockDim.x` stride is computed in 32-bit arithmetic and wraps

#### Host, Device, and Stream Lifetimes
- Device memory touched after `cudaFree`/`hipFree`, or freed twice, including through a copy of the pointer taken before the free
- An asynchronous copy or kernel result read on the host with no synchronization, or a `cudaMemcpyAsync` that is presented as asynchronous while its host side is ordinary pageable memory
- Two streams with no event or stream dependency between them where at least one of them writes a buffer the other touches; two readers of the same buffer need no ordering between them
- Data produced on one device and consumed on another, or copied into unified memory, with no synchronization on the producing side
- A `__global__` function launched from another `__global__` function, or recursion in device code, in a translation unit not built for relocatable device code
- Do not report a missing `cudaDeviceSynchronize()` whose consumer is another call on the same stream: stream order already guarantees that

#### Numerics, Aliasing, and the Compile-Time Contract
- Fast-math intrinsics (`__fdividef`, `__expf`, `rsqrtf`) or a `--use_fast_math` build in code whose results are compared against a reference, where the deviation from the IEEE operations they replace can exceed the tolerance that comparison allows; a comparison with slack absorbs it
- Denormal or NaN behaviour assumed to match the host build under fast math, which flushes denormals and may drop special-value handling
- `__restrict__` on a parameter that does alias another parameter of the same kernel, which licenses the compiler to reorder the overlapping loads and stores
- Device-only code guarded by a host-side macro, or an assumption that `__CUDA_ARCH__` holds the same value in the host and device passes
- Do not report the presence or absence of `__launch_bounds__`; it is a register-pressure choice

#### Sources
Every architecture and version claim above is taken from NVIDIA's own documentation; the behavioural rules follow the same sections.
- Compute capability, thread and block limits, and the required launch configuration: *CUDA C++ Programming Guide*, Thread Hierarchy and Launch Configuration sections.
- `__syncthreads*()` uniformity, `__activemask()`, `__syncwarp()`, the warp vote and shuffle intrinsics, `__threadfence()` and `__restrict__`: *CUDA C++ Programming Guide*, C/C++ Language Extensions.
- Code generated for a device-side launch, and its `-rdc=true` / device-runtime requirement: *CUDA C++ Programming Guide*, CUDA Dynamic Parallelism.
- `cudaGetLastError()` resetting the error variable, and the launch/configuration errors: *CUDA Runtime API*, Error Handling; *CUDA C++ Programming Guide*, Error Checking.
- `--use_fast_math` implying `--ftz=true --prec-div=false --prec-sqrt=false --fmad=true`: *CUDA Compiler Driver NVCC*, `--use_fast_math`.
- Page-locked host memory being required to overlap a transfer with kernel execution: *CUDA C++ Programming Guide*, Page-Locked Host Memory and Asynchronous Concurrent Execution.
