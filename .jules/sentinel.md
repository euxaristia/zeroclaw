## 2024-07-09 - [Constant Time Compare]
**Vulnerability:** Timing attack in auth token verification
**Learning:** Standard string comparison (`==` or `!=`) is susceptible to timing attacks, where an attacker can guess the expected token by measuring the time it takes for the comparison to fail.
**Prevention:** Always use `crypto/subtle.ConstantTimeCompare` for comparing sensitive tokens or passwords.
