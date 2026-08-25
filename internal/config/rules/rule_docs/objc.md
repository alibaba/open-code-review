> Favor precision over recall: report only defects demonstrable within the supplied review scope and relevant repository context. Treat reachable crashes, use-after-free, persistent leaks, data corruption, security failures, races, and deadlocks as blocking; treat naming, modernization, style, and speculative performance advice as non-blocking. In diff-based review, report findings on changed lines; in full-file scan mode, review the supplied file. Before reporting non-local behavior, use `file_read` and `code_search` to verify ownership, ARC versus MRC build mode, API contracts, protocol and category declarations, deployment targets, queue use, and object lifecycle. Do not infer retain cycles, nullability, thread use, or availability from names alone, and do not restate compiler or static-analyzer diagnostics unless the reviewed code creates a concrete runtime consequence.

#### ARC and Object Ownership

- A new strong ownership edge completes a cycle that keeps an object graph alive after its intended lifecycle; verify both directions of the cycle rather than flagging every strong reference
- An object-valued `assign` or `unsafe_unretained` reference is dereferenced after the referenced object can deallocate, creating a dangling pointer; do not recommend `weak` unless the target and build mode support zeroing weak references
- A `weak` reference is the only reference to an object that must remain alive to complete required work, so the work can silently disappear before it runs
- In a file proven to use manual reference counting: a result owned through `alloc`, `new`, `copy`, `mutableCopy`, or `retain` is not released on every path, an autoreleased object is stored without retain/copy, or an owned object is over-released. Do not apply MRC rules to ARC-managed files
- A custom accessor or instance-variable assignment violates the declared `strong`, `weak`, `copy`, or MRC `retain` ownership contract; property attributes do not enforce semantics inside a hand-written setter
- `dealloc` leaves owned non-Objective-C resources, observation registrations, timers, or callbacks active after the object dies. Under ARC, do not request `[super dealloc]` or manual releases of Objective-C objects

#### Blocks, Callbacks, and Timers

- An escaping block stored by an object strongly captures that same object, directly or through another captured owner, completing a retain cycle
- `__block` is used as though it made an Objective-C object non-retaining under ARC; ARC retains object-valued `__block` captures unless they are explicitly weak
- A weak reference is converted to a strong reference outside the escaping block, so the block still captures and retains the object it was intended not to own
- Several reads of a weak capture must refer to the same live object for the operation to be valid, but the block does not promote it once to a strong local before use
- An escaping block is stored by raw assignment in MRC, or by a custom setter that fails to honor a `copy` contract, allowing a stack block to outlive its scope
- An `NSTimer`, `CADisplayLink`, block-based observer, operation, or subscription retains its target/block while its owner retains the registration object, and no invalidation or ownership break occurs on every lifecycle exit
- An asynchronous completion applies stale state or updates an owner after that operation has been cancelled, replaced, or made irrelevant by lifecycle teardown
- Do not report a strong capture when the captured object does not own the block or other context proves there is no cycle

#### Properties and Encapsulation

- A delegate, data source, child-to-parent link, or callback owner is strong when the surrounding ownership graph proves that the other side already owns it
- An immutable value property such as `NSString`, `NSArray`, or `NSDictionary` is retained instead of copied when callers can pass a mutable subclass and later mutation would violate the property's snapshot contract
- A custom setter releases the old MRC value before safely retaining/copying an aliased new value, or invokes callbacks/KVO while the property is in a partially updated state
- Code treats an `atomic` property as protection for a compound invariant, a mutable pointee, or a read-modify-write sequence; atomic accessors do not make the owning object thread-safe
- Direct instance-variable mutation bypasses a custom setter whose validation, cache invalidation, observation, or ownership side effect is required for correctness
- A readonly API returns its internal mutable collection directly, allowing callers to mutate state that the class assumes it controls
- Do not report `nonatomic` by itself; require evidence that the property is accessed concurrently without other synchronization

#### Initialization and Object Construction

- An initializer uses the original receiver after `[super init...]` instead of assigning and checking the object returned by the superclass initializer
- A designated initializer skips the superclass's designated initializer, or a convenience initializer bypasses the class's designated initializer, leaving required inherited or local state unset
- A newly added initializer, `initWithCoder:`, or factory path returns an object without establishing invariants that other construction paths establish
- An initializer invokes an overridable method before the instance is fully initialized, and an existing or permitted subclass override can observe or act on partial state
- Initialization failure returns a partially usable object instead of `nil`, or a factory method silently substitutes a fallback that violates its documented failure contract
- A subclass inherits an initializer that cannot establish the subclass's mandatory state and does not override or mark that initializer unavailable
- Do not require an `init` override when the superclass initializer already establishes every invariant and the subclass adds no construction requirement

#### Nil, NSNull, and Object Semantics

- Messaging `nil` silently yields `nil`, zero, or `NO` in a path where that value is interpreted as successful or valid, masking a missing required object
- An `NSNumber *` is tested as a Boolean pointer, so `@NO` is treated as true; use `boolValue` when the contained value controls the branch
- `NSNull` from JSON, collections, or KVO is treated as `nil` or sent a domain-object selector, causing incorrect branching or an unrecognized-selector exception
- `==` or `!=` is used where semantic equality of strings, numbers, dates, or collections is required; do not flag identity comparisons that are intentional
- `NSNotFound` is narrowed, used in arithmetic, or passed into a range/index operation before it is checked
- A nullable result flows into a Foundation initializer, collection, attributed-string, or other API that requires a nonnull argument and raises an exception
- Do not report intentional nil messaging when the zero/nil result is explicitly the desired optional behavior

#### Foundation Collections, Strings, and Ranges

- A possibly nil object or key is inserted into an array, set, dictionary, or collection literal, causing an `NSInvalidArgumentException`
- A mutable collection is changed during fast enumeration, including mutation through an alias or callback invoked by the loop
- Code casts an immutable collection to a mutable type and mutates it instead of obtaining a mutable copy
- An index or `NSRange` can exceed the current collection/string bounds after filtering, asynchronous mutation, failed search, or unchecked external input
- `NSString.length` or raw `NSRange` offsets split a surrogate pair or composed character sequence in user-visible text where grapheme boundaries matter
- A C string returned by `UTF8String` is used after the temporary conversion buffer's lifetime, including after the surrounding autorelease pool drains; retaining the `NSString` alone is insufficient, so copy the bytes before the pointer escapes that context
- A pointer returned by `bytes` or another borrowed-buffer accessor is stored or used asynchronously after its owning object can deallocate or its mutable backing store can change
- A runtime-derived or user-controlled string is passed as the format argument to `NSLog`, `stringWithFormat:`, or another variadic formatter instead of as a `%@` value
- A nonliteral format string and its arguments have incompatible types or widths, such as using fixed-width integer specifiers for `NSInteger`, `NSUInteger`, or `size_t`, causing undefined varargs reads

#### Protocols, Delegates, and Selectors

- An `@optional` protocol method is sent to a nonnil object without first verifying `respondsToSelector:`, making an unimplemented selector reachable
- A weak or concurrently replaceable delegate is read once for `respondsToSelector:` and again for invocation, allowing a different object to receive the unchecked selector; hold one strong local across the check and call
- A class newly claims protocol conformance while a required method is a stub, returns a placeholder, or violates a required behavior visible in the protocol or its callers
- A target-action, notification, timer, callback, or `performSelector:` use supplies a selector with the wrong arity or an incompatible parameter/return ABI
- A method declaration and implementation use incompatible parameter or return types for the same selector, and runtime dispatch can therefore pass or interpret values incorrectly
- A framework class, selector, constant, or enum case introduced after the verified minimum deployment target is reachable without an availability guard or supported fallback; name the exact platform version when reporting it
- `conformsToProtocol:` is used in place of `respondsToSelector:` before an optional method call; protocol conformance does not guarantee that optional methods are implemented
- Do not report optional protocol methods that are guarded by a wrapper or forwarding implementation verified in the repository

#### Categories and Runtime Modification

- A category adds a selector already implemented by the class or another category, as confirmed by `code_search`; load order then decides which implementation wins
- A category declares a property but provides neither accessors nor associated storage, making the property compile as a declaration but fail when messaged
- Associated-object storage uses a key already used for a different value, or an association policy that conflicts with the value's required ownership or thread behavior
- Method swizzling can execute more than once, so repeated exchanges toggle or corrupt the installed behavior; one-time installation must be explicit
- Swizzling an inherited method exchanges an implementation on the superclass rather than isolating the change to the intended subclass
- A `+load` hook waits for work that cannot run until image loading completes, acquires a lock that initialization can re-enter, or depends on category/class load order
- A cast of `IMP` or `objc_msgSend` does not exactly match the method's calling convention, parameter types, and return type
- Do not report categories or swizzling merely because they are used; identify the concrete selector collision, global side effect, or ABI/lifecycle failure

#### Core Foundation and C Interoperability

- A Core Foundation result obtained through a Create or Copy rule is neither released nor transferred on every exit path
- A non-owned Get-rule Core Foundation result is released or transferred as though owned, or is used after the owner that guarantees its lifetime can disappear
- `__bridge_retained`/`CFBridgingRetain` is not balanced by a release, or `__bridge_transfer`/`CFBridgingRelease` is followed by another release of the same ownership
- A plain `__bridge` pointer escapes beyond the Objective-C object's lifetime even though no ownership was transferred
- A C callback context, function pointer, or stored `void *` uses the wrong bridge/retain convention, leaving either a dangling Objective-C object or a leaked retained context
- A C API receives a stack address, temporary buffer, or borrowed Foundation bytes that can outlive the backing storage during asynchronous use
- A buffer length, element count, or struct layout passed across the C boundary is computed in the wrong unit or with a narrowing conversion, allowing out-of-bounds access
- Do not request ownership changes without first applying the called API's documented Create/Get and callback-context contract

#### Error Handling and Exceptions

- A caller reads an `NSError *` without first checking the method's primary `BOOL`, object, or sentinel return value, so a stale or unspecified error controls behavior
- An implementation writes through an `NSError **` without checking whether the caller passed `NULL`
- A failure return is ignored and the code continues with nil, partial, or stale output as though the operation succeeded
- A method reports success while also setting an error, reports failure without the error required by its contract, or invokes a completion with contradictory result/error values
- An `@catch` block swallows an exception and continues with potentially corrupted state, or catches programmer exceptions as though they were ordinary recoverable errors
- Recoverable I/O, validation, or service failures are newly converted into exceptions across an API that otherwise uses `NSError` or explicit result values
- Wrapping an error discards the domain/code or underlying error that existing callers use to choose recovery behavior
- Do not report an unused optional `NSError` detail when the primary failure is handled completely and no caller needs the additional distinction

#### Concurrency, Queues, and UI Isolation

- Mutable state or a mutable Foundation collection is read and written from concurrently reachable queues without a single proven synchronization strategy
- UIKit or AppKit state is read or mutated from a background queue where the framework requires main-thread access
- `dispatch_sync` targets the current serial queue, including synchronously dispatching to the main queue from the main thread, causing a deterministic deadlock
- The main thread waits on a semaphore, operation, or condition whose completion is scheduled onto the main queue
- `@synchronized(lock)` can receive `nil`, in which case the critical section is not synchronized at all, or the lock object can be replaced so callers protect the same state with different locks
- A lock is held while sending an externally implemented Objective-C message or callback that can synchronously re-enter and acquire the same non-recursive lock
- `dispatch_group_enter` lacks a matching `dispatch_group_leave` on a return, error, or cancellation path, so group completion never fires
- A Core Data managed object or context crosses its confinement queue instead of using the context's queue and object IDs
- A long-running background loop creates many autoreleased objects without a scoped `@autoreleasepool`, causing memory growth until the thread or outer pool drains
- Do not infer concurrency from an asynchronous-looking method name; verify queue creation, call sites, or framework callback guarantees

#### KVC, KVO, and Notifications

- A string key/key path no longer names a KVC-compliant property after a rename or refactor, making runtime lookup raise or silently target the wrong member
- `setValue:nil forKey:` can reach a non-object property without a valid `setNilValueForKey:` policy
- Direct ivar mutation bypasses automatic KVO notifications for a property whose observers are verified to require the change
- A setter or KVC mutation triggers automatic KVO while the same change is also wrapped in manual `willChange...`/`didChange...` calls, causing duplicate observations, or the manual calls are unbalanced on an exit path
- A derived observable property changes when one of its inputs changes, but its dependent keys are not registered and existing observers therefore remain stale
- Observer registration and removal use mismatched objects, key paths, contexts, or lifetimes, leaving a callback to a dead observer or removing another registration
- `observeValueForKeyPath:...` consumes unknown contexts instead of forwarding them to `super`, breaking observations installed by a superclass
- KVO change dictionaries are assumed to contain domain objects even though old/new nil values are represented by `NSNull`
- A block-based notification observer token is discarded or retained by an owner captured by its block, preventing correct removal or completing a retain cycle
- A notification callback performs thread-confined work without accounting for the posting thread on which notification delivery occurs
- Do not demand manual observer removal when the verified API and deployment runtime provide token-scoped or automatic teardown

#### Archiving and External Dynamic Input

- Data from an untrusted or replaceable source is decoded with unrestricted `NSKeyedUnarchiver` APIs instead of secure coding with an explicit allowed-class set
- A class claims `NSSecureCoding` support but decodes nested objects with unrestricted `decodeObjectForKey:` before validating their classes
- `initWithCoder:` accepts decoded types, ranges, enum values, or object graphs that violate invariants enforced by normal initializers
- Selector or class names derived from external input are passed to `NSSelectorFromString`, `NSClassFromString`, `performSelector:`, or runtime invocation without an allowlist, exposing unintended code paths
- Deserialization failure is replaced with a partially populated object that callers cannot distinguish from valid persisted state
- Do not report unrestricted decoding for data whose integrity and provenance are both established within the same trust boundary

#### Objective-C Test Correctness

- `XCTAssertEqual`/`XCTAssertNotEqual` is used to test value equality of Objective-C objects, so the test compares identity instead of using `XCTAssertEqualObjects`/`XCTAssertNotEqualObjects`
- An asynchronous test can finish before its completion assertions run, or fulfills its expectation before the behavior under test has completed
- A retain-cycle or deallocation test accidentally keeps an additional strong local reference, making its lifetime assertion incapable of detecting the regression
- A test expects an Objective-C exception for an API that reports ordinary failure through `NSError`, a Boolean, nil, or a sentinel, so it does not exercise the real failure contract
