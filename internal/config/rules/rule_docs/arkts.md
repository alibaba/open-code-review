#### Obvious Typos or Spelling Errors
- Spelling errors in component names, variable names, or function names
- Spelling errors in log or error messages that affect readability

#### Dead Code
- Code blocks that will never be executed (e.g., branches where the condition is always false, code after a return statement)
- Variables that are declared but never read or referenced
- Large blocks of commented-out code (with no apparent intent to retain)

#### State Decorator Usage
- Arrays/objects decorated with `@State` will not trigger UI refresh when modified via push/property changes; references must be replaced
- Verify correct usage of `@Prop` (one-way) vs `@Link` (two-way) for the given scenario
- Nested object state updates must use `@Observed` + `@ObjectLink`
- Props drilling beyond 3 levels should use `@Provide/@Consume` instead
- `@StorageLink/@StorageProp` should only be used for truly global state; avoid overuse

#### Component Lifecycle
- Timers and listeners created in `aboutToAppear` must be released in `aboutToDisappear`
- Page-level logic should be placed in `onPageShow/onPageHide` rather than component lifecycle hooks
- Avoid executing time-consuming synchronous operations in lifecycle hooks that block the UI thread

#### ArkUI Declarative Syntax
- Side effects (network requests, timers, logging) are prohibited in the `build` method
- `ForEach` / `LazyForEach` must provide a unique and stable key generator function
- Use `if/else` for conditional rendering, not `switch`
- Direct manipulation of component instances outside the `build` method is prohibited

#### Performance Optimization
- Large lists (>20 items) must use `LazyForEach` instead of `ForEach`
- Creating new objects, closures, or calling functions that return styles in the `build` method is prohibited, as it causes unnecessary child component rebuilds
- Complex computation results should be cached via `@Watch` to avoid redundant calculations on each render
- Image resources should have proper caching strategies to avoid repeated loading

#### Resource Access Standards
- String hardcoding is prohibited; use `$r('app.string.key')` to support internationalization
- Images must use `$r('app.media.icon')` or `$rawfile('path')`; hardcoded paths are prohibited
- Colors/dimensions should use resource references like `$r('app.color.primary')` to support theme switching

#### Component Communication
- Parent→Child: use `@Prop`/`@Link`; Child→Parent: use callback function `onEvent` pattern
- Cross-component communication: use `@Provide/@Consume`; global state: use `AppStorage`
- Avoid passing local component state through `AppStorage`

#### General TypeScript Standards
- Using `any` type is prohibited; if unavoidable, a comment explaining the reason is required
- Using `var` is prohibited; use `let` or `const`
- Using `==` and `!=` is prohibited; use `===` and `!==`
- Async functions must include try-catch error handling with user-friendly error messages
- Prefer async/await; callback hell is prohibited; use `Promise.all` for independent async operations
- Null checks: perform null checks when accessing values or destructuring to avoid null pointer exceptions

#### Code Security Checks
- User input must be validated (length, format, range); direct concatenation into SQL or command strings is prohibited
- Sensitive information (keys, passwords, tokens) must not be logged or uploaded
- Network requests must use HTTPS with certificate verification
