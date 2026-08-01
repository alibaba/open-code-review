### 1. Null Safety
- **Issue**: Nullable types not handled correctly, leading to potential `NullPointerException`.
- **Key checks**:
  - Avoid overusing `!!` (non-null assertion); prefer safe calls `?.` or the Elvis operator `?:`.
  - Ensure nullable properties from data classes or API responses are handled properly.
- **Bad example**:
  ```kotlin
  val length: Int = text!!.length // Risk: text may be null
  ```
- **Improvement**:
  ```kotlin
  val length: Int = text?.length ?: 0 // Safe handling
  ```

#### Dead Code
- Code blocks that can never be reached (e.g., branches where the condition is always false, code after a return statement)
- Variables that are declared but never read or referenced
- Large blocks of commented-out code (with no apparent intent to preserve)

### 2. Function and Expression Conciseness
- **Issue**: Redundant code undermines Kotlin's conciseness.
- **Key checks**:
  - Use `=` to simplify single-expression functions (e.g., `fun sum(a: Int, b: Int) = a + b`).
  - Replace complex `if-else` chains with `when`.
  - Avoid unnecessary `return` (e.g., use expression results directly in lambdas).
- **Bad example**:
  ```kotlin
  fun getGrade(score: Int): String {
      if (score >= 90) return "A"
      else if (score >= 80) return "B"
      else return "C"
  }
  ```
- **Improvement**:
  ```kotlin
  fun getGrade(score: Int) = when {
      score >= 90 -> "A"
      score >= 80 -> "B"
      else -> "C"
  }
  ```

### 3. Collection Operation Optimization
- **Issue**: Inefficient collection operations causing performance problems.
- **Key checks**:
  - Prefer `Sequence` for large collections (lazy evaluation reduces intermediate objects).
  - Avoid redundant operations (e.g., merge multiple `filter` calls into one).
  - Use `groupBy`, `associate`, etc. instead of manual iteration.
- **Bad example**:
  ```kotlin
  val evenSquares = listOf(1, 2, 3).map { it * it }.filter { it % 2 == 0 } // creates intermediate collections
  ```
- **Improvement**:
  ```kotlin
  val evenSquares = listOf(1, 2, 3).asSequence()
      .map { it * it }
      .filter { it % 2 == 0 }
      .toList() // lazy evaluation
  ```

---

### 4. Proper Use of Coroutines
- **Issue**: Coroutine leaks or improper exception handling.
- **Key checks**:
  - Use structured concurrency (`coroutineScope` or `supervisorScope` to manage lifecycle).
  - Avoid `GlobalScope` (prone to resource leaks).
  - Exception handling: wrap `withContext` or `async` in `try/catch`.
- **Bad example**:
  ```kotlin
  GlobalScope.launch { // escapes scope, may leak
      fetchData()
  }
  ```
- **Improvement**:
  ```kotlin
  viewModelScope.launch { // structured concurrency
      try {
          withContext(Dispatchers.IO) { fetchData() }
      } catch (e: Exception) { /* handle exception */ }
  }
  ```

### 5. Class and Object Design
- **Issue**: Not leveraging Kotlin features leads to redundancy.
- **Key checks**:
  - **Data classes**: Use `data class` for pure data objects (auto-generates `equals`/`hashCode`).
  - **Sealed classes/interfaces**: Use `sealed class` for restricted type hierarchies (enables exhaustive `when` branch checking).
  - **Delegation**: Property delegation (e.g., `by lazy`) or class delegation (`by` for the decorator pattern).
- **Bad example**:
  ```kotlin
  class User(val name: String) {
      // manually implementing toString()/equals()...
  }
  ```
- **Improvement**:
  ```kotlin
  data class User(val name: String) // standard methods auto-generated
  ```

### 6. Resource Management and Scope Functions
- **Issue**: Resources not released or scope functions misused.
- **Key checks**:
  - Use `use` to auto-close file/network resources (e.g., `FileInputStream().use { ... }`).
  - Scope functions (`let`, `apply`, etc.) should maintain readability; avoid excessive nesting.
- **Bad example**:
  ```kotlin
  val file = File("path")
  val reader = BufferedReader(FileReader(file))
  // forgot to call reader.close()
  ```
- **Improvement**:
  ```kotlin
  File("path").inputStream().use { stream ->
      // resource auto-closed
  }
  ```

### 7. Performance Pitfalls
- **Issue**: Hidden performance overhead.
- **Key checks**:
  - Inline functions: Use `inline` for higher-order functions to reduce lambda overhead (but avoid inlining large functions).
  - Constants: Use `const val` for compile-time constants (instead of `val`).
  - Avoid creating objects inside loops (e.g., `Regex` instances).

### 8. Interoperability (Java Interaction)
- **Issue**: Compatibility problems when Java code calls Kotlin code.
- **Key checks**:
  - Use `@JvmStatic` and `@JvmOverloads` to optimize APIs exposed to Java callers.
  - Null safety annotations: Use `@Nullable`/`@NonNull` to help Java recognize nullability.

### 9. Other Key Points
- **Immutability**: Prefer `val` over `var`.
- **String handling**: Use string templates (`"Value: $value"`) instead of concatenation.
