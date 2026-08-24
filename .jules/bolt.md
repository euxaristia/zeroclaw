## 2025-02-20 - [Avoid repeated allocations of static slices]
**Learning:** Returning static slice constants like `[]Item{...}` from frequently called functions (e.g. HTTP handlers or utility functions like `ContextWindowFor`) causes repeated heap allocations and garbage collection overhead.
**Action:** Move static data to package-level variables and pre-compute O(1) lookup maps in an `init()` function when appropriate to minimize GC pressure and improve performance.
