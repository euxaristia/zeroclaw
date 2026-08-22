## $(date +%Y-%m-%d) - Pre-allocate static slices and O(1) map for lookups
**Learning:** Returning inline slice declarations in frequently called functions causes repeated memory allocations and GC overhead. String comparisons in a loop for lookups (O(N)) are slow.
**Action:** When a static list of items is needed, define it as a package-level variable. Use `init()` to pre-compute maps for O(1) lookups to avoid iterating over slices on every call.
