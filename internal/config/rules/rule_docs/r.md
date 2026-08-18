#### R Code Review Principles

> Favor correctness, numerical precision, memory efficiency, and vectorization over style: report only defects likely real in the changed code, statistical models, pipelines, and package structure. Treat data corruption, subtle scope bugs, vectorized logic failures, unsafe evaluation, and package build breaks as blocking; style-only suggestions are non-blocking. Do not duplicate errors that `R CMD check`, `lintr`, `styler`, or standard R parser tools identify mechanically unless the diff reveals a concrete production or runtime execution risk.

Before reporting a non-local claim, inspect function definitions, namespace loads (`library()`, `require()`, `pkg::func`), lazy evaluation boundaries, NSE (non-standard evaluation) contexts, formula interfaces, environment chains, and package dependencies. Do not assume a vectorized function, S3/S4 method dispatch, or memory operation is unsafe without evidence of caller context, input data structures, or performance limits.

#### Vectorization, Data Types, and Type Safety

* Unintentional scalar logical operations (`&&`, `||`) used on vectors where element-wise operations (`&`, `|`) are required, or element-wise logicals used inside scalar conditionals like `if (...)`.
* Implicit type coercion caused by mixing types in vectors, matrices, or `c()` calls (e.g., mixing `character` and `numeric`), or relying on implicit `factors`-to-character/numeric conversions without explicit `as.character()` or `as.numeric(as.character())`.
* Missing or incorrect edge-case handling for zero-length inputs (`numeric(0)`, `character(0)`), empty data frames, single-row/column matrices dropping dimensions (`drop = FALSE`), or unexpected `NA`/`NULL`/`NaN`/`Inf` propagation.
* Unsafe recycling of vectors in arithmetic, comparisons, or data frame assignments where vector lengths are not equal or exact multiples, leading to silent standard R recycling or subtle calculation bugs.
* Relying on base equality checks (`==`) with floating-point numbers instead of `isTRUE(all.equal(...))` or setting threshold tolerances (`abs(x - y) < eps`).

#### Non-Standard Evaluation (NSE) and Tidyverse/Data.Table Syntax

* Unquoted column references, dynamic variable names, or programmatic evaluation using NSE (e.g., `dplyr::select()`, `ggplot2::aes()`, `data.table` expressions) without proper quasiquotation (`!!`, `{{{ }}}`, `sym()`, `all_of()`, `any_of()`) when passed as function parameters.
* Ambiguity between data frame column names and environment variables inside `dplyr`, `data.table`, or `subset()` expressions, missing explicit `.data$` or `.env$` pronoun usage in package code.
* Side effects in `data.table` in-place modification (`:=`) leaking into caller environments or modifying passed arguments without explicit deep copying (`copy()`).
* Misuse of standard base evaluation inside tidyverse pipeline functions or vice-versa, causing delayed execution failures or unexpected binding contexts.

#### Scope, Lazy Evaluation, and Environment Boundaries

* Scoping bugs where functions implicitly rely on global environment variables (`.GlobalEnv`) rather than explicitly passed arguments or package options (`getOption()`).
* Unintended variable capture in delayed evaluation contexts, lazy promises, `lapply()` / `purrr::map()` loops, or standard `for` loops where iteration variables are referenced lazily inside closures/lambdas.
* Modifying caller environments using `assign()`, `<<-`, or `parent.frame()` without explicit architectural justification, clear lock boundaries, or documentation of side effects.
* Mismanaging S3, S4, or R6 method dispatch, wrong class inheritance order, or failing to call `UseMethod()` / `callNextMethod()` correctly.

#### Memory Management, Performance, and I/O

* Repeated memory re-allocation inside loops (e.g., appending rows to data frames with `rbind()` or extending vectors dynamically) instead of pre-allocating output vectors or using vector/list accumulation.
* Deep copying of large data objects in memory when passing to functions or executing multi-step transformations where memory-efficient tools (`data.table`, `arrow`, `dbplyr`, or `vroom`) should be used.
* Missing explicit connection closures or resource cleanup (`close()`, `on.exit()`) when opening file handles, database connections, graphics devices (`dev.off()`), or temporary directories.
* Unfiltered large dataset imports using `read.csv()` or generic base I/O instead of chunked, memory-mapped, or fast parallel alternatives (`data.table::fread()`, `arrow::read_parquet()`, `vroom::vroom()`).

#### Statistical Precision and Numerical Stability

* Numerical instability or overflow/underflow in custom mathematical functions, likelihoods, or matrix operations where log-scale computations (`log1p()`, `expm1()`, `log-sum-exp`), specialized solvers, or QR decomposition should be used instead of direct inversion (`solve()`).
* Improper handling of missing data (`NA`) in statistical summaries, aggregates, or model estimation (`na.rm = TRUE`, `na.action` settings), leading to unhandled `NA` results or unexpected row dropped patterns.
* Random number generation (RNG) calls (`rnorm()`, `runif()`, etc.) lacking reproducible `set.seed()` calls in tests or stochastic workflows, or unsafe seed state handling in parallel execution (`L'Ecuyer-CMRG` workers).

#### Package Structure, Dependencies, and Namespace

* Direct use of `library()` or `require()` inside package functions instead of properly declaring imports in `DESCRIPTION` (`Imports`, `Suggests`) and namespace imports via `NAMESPACE` (`importFrom`).
* Unqualified calls to non-base package functions inside package code that depend on global search path order, rather than using `package::function()` prefixing.
* Polluting the global search path or masking core methods through overly broad `import(pkg)` directives in package development.
* Non-portable file paths using hardcoded path separators (`/` or `\`), absolute local paths, or user-specific home directories instead of `file.path()`, `here::here()`, or standard R temporary directory utilities (`tempdir()`, `tempfile()`).

#### Review Scope

* Focus on logical correctness, vectorization bugs, memory safety, data frame integrity, statistical precision, and package export safety.
* Do not report pure code styling choices (e.g., `=` vs `<-` assignment, indentation width, snake_case vs camelCase naming) or documentation missingness unless it breaks package vignettes or `R CMD check`.
* When the code change is intentionally part of a major library overhaul or migration (e.g., converting base R code to `dtplyr` or `rlang`), review the full execution context and dynamic inputs before flagging a compatibility issue.