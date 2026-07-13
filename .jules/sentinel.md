## 2025-02-28 - Token Comparison Timing Attack and Slowloris
**Vulnerability:** The daemon server used standard string equality (`==` or `!=`) to validate bearer tokens from the client (`r.Header.Get("Authorization") != "Bearer "+s.token`). It also initialized its `http.Server` without timeouts.
**Learning:** Standard string equality checks return as soon as a mismatch is found. This short-circuit behavior allows an attacker to deduce a valid token character by character, measuring the exact time a request takes. Moreover, a lack of `ReadHeaderTimeout` on a server leaves it vulnerable to Slowloris attacks where clients hold connections open indefinitely.
**Prevention:** Use `crypto/subtle.ConstantTimeCompare` when comparing secrets like authentication tokens or passwords. This compares both byte slices in constant time, regardless of their contents, preventing timing side-channels. Always configure `ReadHeaderTimeout` on `http.Server` instances to prevent connection exhaustion attacks.

## 2025-02-28 - Error Detail Leakage in External Channels
**Vulnerability:** The Telegram channel handler `handleUpdate` forwarded the raw error message (`err.Error()`) to users when an internal execution error occurred.
**Learning:** Forwarding raw error strings to end-users can inadvertently expose internal stack traces, system paths, or downstream API details over the external channel.
**Prevention:** Always log the detailed error internally on the server but return a sanitized, generic error message (e.g., "An error occurred. Please check the logs.") to external callers or users to fail securely.
