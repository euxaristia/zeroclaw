## 2025-02-28 - Token Comparison Timing Attack and Slowloris
**Vulnerability:** The daemon server used standard string equality (`==` or `!=`) to validate bearer tokens from the client (`r.Header.Get("Authorization") != "Bearer "+s.token`). It also initialized its `http.Server` without timeouts.
**Learning:** Standard string equality checks return as soon as a mismatch is found. This short-circuit behavior allows an attacker to deduce a valid token character by character, measuring the exact time a request takes. Moreover, a lack of `ReadHeaderTimeout` on a server leaves it vulnerable to Slowloris attacks where clients hold connections open indefinitely.
**Prevention:** Use `crypto/subtle.ConstantTimeCompare` when comparing secrets like authentication tokens or passwords. This compares both byte slices in constant time, regardless of their contents, preventing timing side-channels. Always configure `ReadHeaderTimeout` on `http.Server` instances to prevent connection exhaustion attacks.
## 2025-02-28 - Secure Token Comparison with Hashing
**Vulnerability:** Length-based timing leak in token comparison. `subtle.ConstantTimeCompare` returns immediately if the lengths of the two byte slices differ.
**Learning:** Even when using `crypto/subtle.ConstantTimeCompare`, if one of the inputs can be of variable length, an attacker can determine the exact length of the expected secret by measuring response times.
**Prevention:** To guarantee strict constant-time comparison regardless of input length, hash both the user-provided token and the expected secret (e.g., using `crypto/sha256`) before passing them to `ConstantTimeCompare`. This ensures both inputs are always exactly the same length (e.g., 32 bytes for SHA-256).

## 2025-03-05 - DoS via Connection and Memory Exhaustion
**Vulnerability:** The daemon's `http.Server` was initialized without `ReadTimeout` or `IdleTimeout`, making it susceptible to Slowloris connection exhaustion attacks. In addition, the `/turn` endpoint lacked limits on the size of the request payload causing it to read an unbounded JSON request body which could lead to Out of Memory (OOM) errors.
**Learning:** Even internal or local-only Go HTTP servers should have explicit timeouts to prevent attackers or misbehaving clients from consuming all available connection slots. In addition, parsing arbitrary JSON data from `r.Body` without wrapping it in a `MaxBytesReader` can let an attacker exhaust memory resources.
**Prevention:** Always configure `ReadTimeout` and `IdleTimeout` on `http.Server` definitions. Remember that `WriteTimeout` shouldn't be added for endpoints returning streaming data (like SSE or long polls). To limit request size in Go, apply `http.MaxBytesReader` before passing `r.Body` to Decoders or reading it into memory.

## 2025-02-28 - Error Detail Leakage in External Channels
**Vulnerability:** The Telegram channel handler `handleUpdate` forwarded the raw error message (`err.Error()`) to users when an internal execution error occurred.
**Learning:** Forwarding raw error strings to end-users can inadvertently expose internal stack traces, system paths, or downstream API details over the external channel.
**Prevention:** Always log the detailed error internally on the server but return a sanitized, generic error message (e.g., "An error occurred. Please check the logs.") to external callers or users to fail securely.

## 2026-07-17 - Prevent Secret Leakage in Go http.Client URL Errors
**Vulnerability:** Go's `http.Client` operations return `*url.Error` upon failure, which includes the full requested URL string. When calling the Telegram Bot API (or other services embedding secrets in the URL like `https://api.telegram.org/bot<TOKEN>/...`), returning or logging this error directly leaks the bot token in plaintext.
**Learning:** Returning standard library errors directly without sanitization can inadvertently leak secrets when those secrets are part of the request coordinates (URLs, paths) rather than headers or body. Simple string replacement on the error output loses the underlying error type chain unless properly wrapped.
**Prevention:** Implement a custom error struct (e.g., `sanitizedError`) that implements both `Error() string` (to redact the token via `strings.ReplaceAll`) and `Unwrap() error` (to preserve the original error for `errors.Is`/`errors.As` checks). Wrap errors returning from sensitive `http.Client` calls using a deferred helper.
## 2025-03-08 - Missing Timeout on http.Client Leads to DoS
**Vulnerability:** Go's `http.Client`s and `http.DefaultClient` were used without explicit timeouts for communicating with external APIs (Telegram) and internal streaming APIs.
**Learning:** Default HTTP clients in Go have no timeout. An unresponsive server or a slow network can cause the client to hang indefinitely, tying up system resources (goroutines, file descriptors) which can easily lead to a Denial of Service (DoS).
**Prevention:** Always initialize `http.Client` with an explicit `Timeout`. For long-polling endpoints, ensure the timeout is slightly longer than the maximum poll duration to accommodate the expected network hold while still providing an upper bound.
