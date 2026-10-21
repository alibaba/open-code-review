> Architecture consistency review: check that code respects the project's architectural boundaries, dependency directions, and separation of concerns. Only flag when you are confident the violation is real and material; do not nitpick minor style differences that don't cross architectural boundaries.

#### Dependency Direction Violations
- High-level policy or domain modules directly import low-level implementation details (e.g., a `domain/` module importing from `infra/db/postgres.py` instead of an abstract interface)
- Core business logic depends on framework-specific classes, making it hard to swap frameworks
- Shared/common utilities depend on feature-specific modules (the wrong direction of abstraction)

#### Boundary Leakage
- Internal implementation details of a module are exposed through its public interface (e.g., returning ORM objects from a service layer instead of domain DTOs)
- Configuration details leak across module boundaries (e.g., one module hardcoding paths or config values that belong to another module)
- Framework types leak into domain models (e.g., `Request`/`Response` objects in the domain layer)

#### Circular Dependencies
- Two or more modules import each other directly or transitively (A imports B, B imports A)
- Feature modules depend on each other for shared utilities that should live in a common layer

#### Single Responsibility Violations (Architectural Scale)
- A single module/file handles multiple unrelated concerns (e.g., a file that both does business logic and directly formats HTTP responses)
- Cross-cutting concerns (logging, metrics, auth) are scattered throughout business logic instead of being centralized in middleware/filters/interceptors

#### Abstraction Level Mismatch
- Code at a high abstraction level is mixed with low-level implementation details in the same function/class (e.g., business logic mixed with raw SQL string construction)
- Callers depend on concrete implementations when an abstraction/interface would be more appropriate (and the concrete has multiple variants or might change)

#### When NOT to flag
- Minor naming differences that don't affect boundary clarity
- Trivial utility functions that are genuinely shared across layers
- Internal helper functions within a single module (not crossing boundaries)
- Test code that intentionally uses implementation details
- Greenfield projects where the architecture is still being established
