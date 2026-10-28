# NEAR AI verified inference

The built-in `nearai` provider uses
[`github.com/nearai/inference-sdk/go`](https://github.com/nearai/inference-sdk/tree/main/go)
to encrypt requests and verify attestations and completion signatures. It works
with `ocr review`, `ocr scan` and `ocr llm test`, including streamed responses and
tool calls. Select the `nearai` provider explicitly; setting a generic OpenAI
provider's URL to NEAR AI does not enable verification.

## Setup

Provide a NEAR AI API key in `NEARAI_API_KEY`, or configure
`providers.nearai.api_key` / `providers.nearai.api_key_cmd` using the usual OCR
credential flow. Then select a canonical model ID that supports model attestation
and E2EE:

```bash
ocr config set provider nearai
ocr config set providers.nearai.model '<canonical-model-id>'
ocr llm test
ocr review --provider nearai
```

The provider preset supplies `https://cloud-api.near.ai/v1`. The model list in the
picker is a suggestion, not evidence of a model's current capabilities. The SDK
checks the selected model before sending the review content. An unsupported
model fails with `policy.model_attestation_required` or
`e2ee.model_public_key_required`; OCR never changes the policy automatically.

## Verification policy

| Setting | Default | Meaning |
| --- | --- | --- |
| `providers.nearai.e2ee` | `true` | Encrypt supported message and tool fields to an attested model key. |
| `providers.nearai.verification_level` | `model-tee` | Require model attestation and a `provider_tee` completion signature. |
| `providers.nearai.timeout_sec` | `180` | Bound the whole logical call, including preflight, inference and signature retrieval. |

`verification_level` also accepts `gateway`, which requires a valid `gateway` or
`provider_tee` signature. A gateway signature authenticates the gateway's
request/response bytes; it does not prove that a proxied model ran in a TEE.

To use a model that only supports gateway verification, make both changes
explicitly:

```bash
ocr config set providers.nearai.e2ee false
ocr config set providers.nearai.verification_level gateway
```

Turning E2EE off leaves attestation and signature verification enabled. With
`model-tee`, model evidence is still required before sending plaintext to the
gateway. Conversely, keeping E2EE on still requires an attested encryption key,
even if the minimum signature level is `gateway`.

Restore the defaults with:

```bash
ocr config set providers.nearai.e2ee true
ocr config set providers.nearai.verification_level model-tee
```

Settings are captured per client/run, including separate SDK caches. Changing the
saved configuration does not alter a request already in progress. The selected
policy is part of the review's runtime configuration fingerprint.

## Request and failure behavior

Before sending review content, the SDK verifies gateway attestation and its TLS
binding, then model evidence when required by the policy or encryption. It uses
the SDK's default TDX/DCAP verifier and TCB policy (`UpToDate` and `OutOfDate`);
this integration provides no switch to disable quote or TLS verification.

For each completion, the SDK retains the exact wire request and response bytes
(ciphertext when E2EE is enabled). OCR consumes the entire JSON or SSE response,
then calls `VerifyResponse` and enforces the configured signature level. Only
after that succeeds can the agent use the reply or execute tool calls. An SSE
response is therefore buffered until verification; it is not an advisory badge
on already-consumed text.

OCR disables both OpenAI SDK inference retries and its truncated-response replay
for this provider. A transient signature-fetch failure gets at most two retries
(250 ms and 500 ms delays) for the same completion ID. These retries do not send
another inference request. Failed requests return an error without releasing the
response. HTTP errors, transport failures and request timeouts allow later
independent calls, preserving OCR's fallback for optional tasks such as planning.
Attestation, signature, encryption and policy failures are retained for the run
and block subsequent calls. Calls already in flight still have to pass their own
verification. Cancellation is recorded separately.

Verification failures in optional planning, filtering or background tasks also
make the final review/scan fail. A successful HTTP status or a completed sibling
file cannot turn that failure into a successful review. Review manifests record
the run failure as `verification`; HTTP retry diagnostics use a `verification`
phase for failures discovered after the response arrives.

## Results and local storage

Text output includes the verification status, counts, E2EE setting and policy.
JSON review output includes `llm.verification`, for example:

```json
{
  "status": "verified",
  "verified": 2,
  "failed": 0,
  "request_failed": 0,
  "cancelled": 0,
  "pending": 0,
  "e2ee": true,
  "verification_level": "model-tee",
  "records": []
}
```

The real `records` array identifies each logical call by model, completion ID
when available, stage, status, signature kind, signer and check time. Review
requests also include file/task/request identity where available. Failed calls
include a stable error code and the HTTP `status_code` when available, without
copying arbitrary provider error bodies into the records. Ordinary inference
errors retain the same error messages and causes as other OpenAI-compatible
providers. SDK verification errors report their stage and code; HTTP failures
also identify the status and reason phrase.
`failed` counts verification failures; `request_failed` counts request errors,
including unavailable signature retrieval. The report status is `request_failed`
when requests failed but no verification failure occurred, even if other calls
were verified. The review/scan result follows the task's usual failure handling.
Session JSONL stores verification alongside accepted `llm_response` records and
in `session_end`. Failed scans emit their diagnostic JSON on stderr, following
OCR's failure-output convention. SARIF carries the review failure state; use JSON
or session JSONL for the detailed verification records.

These records are diagnostics, not a portable cryptographic proof bundle or a
claim that the model's review findings are correct. They do not assert source
provenance, an approved deployment image, or a deployment allowlist.

E2EE protects supported fields on the network path to the attested model. It
does not encrypt OCR's local session files, tool results, reports or opt-in raw
logs. Those can contain plaintext code and decrypted model output. Model IDs,
routing metadata and request structure are also outside that confidentiality
claim.

## Compatibility limits

- Only the Chat Completions `openai` protocol is supported. HTTPS overrides must
  implement the NEAR AI attestation and signature APIs as well as completions;
  an ordinary OpenAI-compatible proxy is insufficient.
- URLs containing user credentials, a query or a fragment are rejected.
- Configure the model through `model` / `--model`; `extra_body.model` and
  `retry_codes` are rejected. `extra_body.stream=true` enables SSE as usual.
- `--resume` is rejected for verified runs because existing cached findings do
  not carry replayable verification evidence. Start a new run to verify all
  responses under the current policy.
- Other providers retain their existing behavior. The generated VS Code and
  JetBrains catalogs include NEAR AI; configure advanced verification settings
  through the CLI or config file.

The automated integration tests use a local gateway, real Ed25519 signatures and
E2EE with the released SDK. Only the test fixtures replace hardware quote
verification and TLS binding; they are not a live-service or production TDX
attestation test. Run `ocr llm test` with your account to check the selected live
model's capabilities and tool-call round trip.
