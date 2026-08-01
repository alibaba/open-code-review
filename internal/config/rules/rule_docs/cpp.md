#### Obvious Typos or Spelling Errors
- Spelling errors in variable names, constant names, or function names at their declaration sites; do not report spelling errors at call sites
- Strings in log messages or exception messages containing spelling errors that affect readability

#### Dead Code
- Code blocks that can never be reached (e.g., branches where the condition is always false, code after a return statement)
- Variables that are declared but never read or referenced
- Large blocks of commented-out code (with no apparent intent to preserve)

#### Smart Pointer Usage
**Key checks:**
- Prefer `std::unique_ptr` for managing exclusively owned resources
- Use `std::shared_ptr` for managing shared resources
- Avoid using raw pointers to manage dynamic memory
- Use `std::weak_ptr` correctly to break circular references

**Example:**
```cpp
// Bad: using raw pointers
Widget* widget = new Widget();
delete widget; // easy to forget or skipped during exceptions

// Good: using smart pointers
auto widget = std::make_unique<Widget>();
// automatically destroyed, exception-safe
```

#### RAII Principle
**Key checks:**
- Resources are acquired in constructors
- Resources are released in destructors
- Use stack objects to manage resources
- Avoid manual resource management

#### STL Containers and Algorithms
**Key checks:**
- Prefer STL containers over raw arrays
- Use STL algorithms instead of hand-written loops
- Choose the appropriate container type
- Understand the performance characteristics of containers

**Example:**
```cpp
// Bad: hand-written loop
std::vector<int> vec = {1, 2, 3, 4, 5};
for (int i = 0; i < vec.size(); ++i) {
    vec[i] *= 2;
}

// Good: using algorithms
std::transform(vec.begin(), vec.end(), vec.begin(),
               [](int x) { return x * 2; });
```

#### The auto Keyword
**Key checks:**
- Use auto when the type is complex
- Avoid overusing auto for simple types
- Use auto& and const auto& to avoid unnecessary copies

#### Exception Handling Completeness
**Key checks:**
- Catch specific exception types rather than using ...
- Do not silently ignore errors in exception handlers

#### const Correctness
**Key checks:**
- Apply const to member functions where appropriate
- Pass parameters by const reference
- Correct placement of const for pointers and references
- Use const member variables judiciously
