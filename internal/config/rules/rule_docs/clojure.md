> Favor precision over recall: report only issues that are likely to cause incorrect behavior, resource leaks, security vulnerabilities, or material performance problems. Do not report formatting or naming handled by `cljfmt` and `clj-kondo`. Before raising a finding, establish whether the file is Clojure, ClojureScript, or a `.cljc` file whose behavior differs per reader conditional, and review each branch against the platform it targets.

#### Laziness, Sequences, and Resource Scope
- A `with-open` or transaction body whose value is an unrealized lazy sequence such as `line-seq`, `map`, or `filter`; the resource closes before the consumer reads it, so force realization inside the scope with `doall`, `into`, or a reducing call
- Holding a reference to the head of a long or unbounded sequence while consuming the tail, retaining every realized element
- `doall` used where only side effects matter, or `doseq`/`run!`/`dorun` replaced by a lazy `map` whose side effects may never run or may run twice
- `count`, `last`, `sort`, or `reduce` applied to a streaming or infinite sequence
- `pmap` over a body too cheap to offset coordination cost, or over a side-effecting body; its chunking realizes ahead of the consumer and its thread pool is unbounded
- Do not report ordinary lazy pipelines whose results are consumed within the lifetime of their inputs

#### State, Concurrency, and Dynamic Bindings
- A side-effecting or non-idempotent function passed to `swap!`, `alter`, or `commute`; these re-run on retry and the effect repeats
- A read-then-write split across two `swap!` calls where the update must be atomic over the composite value
- An atom used where coordinated multi-identity updates require refs, or a ref used where an uncoordinated independent value suffices
- `binding` values not visible inside `future`, `go` blocks, executor tasks, or callbacks; capture the frame with `bound-fn` or pass the value explicitly
- Mutable Java collections or arrays shared across threads without synchronization, and `locking` on an interned string, boxed number, or other shared constant

#### Namespaces, Keywords, and Data Contracts
- Keyword-versus-string key drift where a map crosses a JSON, HTTP, or storage boundary, typically `:key-fn keyword` applied on one path and omitted on another
- Namespaced keywords flattened, dropped, or silently re-namespaced during serialization
- `spec` or `malli` validation applied to one entry point of a contract but not to a sibling path that accepts the same data
- `require` cycles between namespaces, and `:refer :all`, which makes the origin of a symbol unresolvable
- `def` used for state that must survive namespace reload, where `defonce` is required

#### Macros and Compile-Time Code
- Symbols introduced by an expansion without `gensym` or a `sym#` auto-gensym, capturing a binding from the call site
- An argument expression spliced into the expansion more than once, so a side-effecting or expensive argument evaluates repeatedly; bind it in a `let` first
- A macro where an ordinary function would do, which costs composability and first-class use for no gain
- Symbols emitted unqualified instead of through syntax-quote, resolving against the caller's namespace rather than the definition's

#### Interop, Reflection, and Performance
- Interop calls in a hot path without type hints; check against `(set! *warn-on-reflection* true)` in the namespace
- Boxed arithmetic in tight loops where primitive math is intended
- Chained `map`/`filter`/`remove` building intermediate sequences where the surrounding code already uses transducers with `into` or `transduce`
- `concat` accumulated across loop iterations, which builds nested lazy sequences and overflows the stack on realization
- A transient escaping the scope that created it, used from another thread, or treated as a persistent value rather than through its return value

#### ClojureScript and JavaScript Interop
- Property access on an externally supplied JavaScript object under `:advanced` optimizations without an extern or `goog.object/get`; renaming yields `undefined` rather than an error
- `aget` used for property access instead of array indexing, which is unsound under advanced compilation
- `js->clj` or `clj->js` deep conversion applied to a large object or on a hot path, or `:keywordize-keys` applied inconsistently between producer and consumer
- `.-prop` access on a value that may be `nil` or `js/undefined`, where the two are not interchangeable

#### EDN Data, Configuration, and Untrusted Input
- `read-string` instead of `clojure.edn/read-string` on data that is not developer-authored; the former honors `*read-eval*` and `#=(...)` is remote code execution
- An `edn/read-string` given a permissive `:default` or custom `:readers` that reintroduces evaluation, or called on user input without a surrounding `try`
- Dependency coordinates pinned to a branch or moving version instead of an exact `:git/sha` or `:mvn/version`
- Aliases whose `:main-opts` or `:exec-fn` execute code resolved at build time, and `:paths` shipping test or development directories into a production build
- Secrets, tokens, or credentials committed in a configuration `.edn` file
- Untrusted values concatenated into `clojure.java.shell/sh` arguments, SQL built by string concatenation instead of parameterized queries, or paths used without constraining traversal
- Security tokens or identifiers generated with `java.util.Random` or `rand` rather than `SecureRandom`
