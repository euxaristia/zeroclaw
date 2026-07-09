module zeroclaw

go 1.26

// Toolchain floor for CI's govulncheck gate: GO-2026-5856 (crypto/tls
// Encrypted Client Hello privacy leak, reachable via daemon.Stop ->
// http.Client.Do) requires a patched stdlib; go1.26.5 is verified clean.
toolchain go1.26.5
