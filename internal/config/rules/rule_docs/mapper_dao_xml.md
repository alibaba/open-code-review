#### Obvious Spelling Error Detection
- Spelling errors in SQL keywords
- Spelling mismatches between mapper interface method names and XML `id` attributes
- Spelling errors in attribute names within dynamic SQL tags (e.g., field names in `test` conditions)

#### SQL Logic Error Detection
- **Condition Errors**: Misuse of logical operators in WHERE conditions (AND/OR confusion)
- **JOIN Condition Errors**: Incorrect fields used in join conditions or missing required join conditions
- **Dynamic SQL Logic Errors**: Incorrect `<if test="">` condition evaluation, such as null check errors or type check errors
- **SQL Syntax Errors**: Obvious syntax errors such as missing commas or unmatched parentheses

#### Critical Performance Issues
- **Full Table Scan Risk**: Missing WHERE conditions
- **Large Query Without Pagination**: Queries that may return large datasets without using LIMIT or pagination
- **Repeated Subqueries**: The same subquery used in multiple places; recommend extracting to a temporary table or optimizing SQL structure

#### SQL Injection Security Risk Detection

**Real security risks that should be reported:**
- **Direct String Concatenation**: Using `${}` to concatenate user input parameters into SQL statements poses SQL injection risks
- **LIKE Query Concatenation**: Directly concatenating LIKE conditions instead of using safe parameter binding

**Cases that should NOT be reported:**
- **Proper Use of #{} Parameter Binding**: MyBatis automatically escapes parameters, ensuring security
- **Static SQL Statements**: Fixed SQL statements that do not involve dynamic parameters

**Review Principles:**
- Focus on critical issues that may cause data corruption, performance problems, or security risks
- Consider the actual execution efficiency of SQL statements and their impact on database performance
- Prioritize identifying critical issues that could cause production failures
- Exercise caution when context is unclear: when the full execution context of SQL cannot be determined, choose to ignore rather than report a false positive
- Require sufficient evidence: only report issues when there is clear evidence of a problem
- Prefer false negatives over false positives: maintain high-precision issue identification to avoid drowning real issues in excessive false reports
