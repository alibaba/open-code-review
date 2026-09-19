> LLM application security review: check for prompt injection risks, unsafe LLM output handling, and insecure agent tool usage. Only flag when you are confident the issue is real and exploitable; do not flag theoretical risks that cannot be triggered in practice.

#### Prompt Injection Risks
- User input is directly concatenated into LLM prompts without sanitization or delimiting, allowing injection of new instructions (e.g., `f"System: {system_prompt}\nUser: {user_input}"` where user_input contains "Ignore previous instructions")
- Retrieved context (RAG) is inserted into prompts without isolation, allowing malicious documents to override system instructions
- Tool outputs are fed back into the prompt without sanitization, allowing prompt injection through tool results

#### Unsafe LLM Output Handling
- LLM-generated output is passed to `eval`, `exec`, `subprocess`, or shell commands without validation
- LLM-generated SQL queries are executed without parameterization or safety checks
- LLM-generated file paths are used without path traversal validation
- LLM-generated code is executed in production without sandboxing or review

#### Agent Tool Safety
- Agent tools allow arbitrary command execution without restrictions (e.g., a "run shell command" tool with no allowlist)
- Agent tools have access to sensitive operations (file deletion, network requests, credential access) without explicit user approval gates
- Agent loops are unbounded (no max iterations, no cost limit, no timeout)

#### Sensitive Data Leakage
- API keys, passwords, or PII are hardcoded in prompts or inlined in example conversations
- Sensitive conversation history is included in prompts sent to third-party LLM providers without encryption or redaction
- User data is logged in plaintext in LLM request/response logs

#### When NOT to flag
- Internal tools used only by trusted developers in a sandbox environment
- Local-only LLM deployments with no external input sources
- Prompt templates that are static and never include untrusted input
- Test code or example code that intentionally demonstrates vulnerabilities
