## 2025-03-09 - Precompute O(1) maps for StaticModels
**Learning:** In Go, avoiding repeatedly allocating static slices or iterating through large lists each time is highly desirable for frequently called functions.
**Action:** Move static data to package-level variables and use `init()` to pre-compute O(1) maps for lookups to minimize garbage collection overhead. This applies to functions like `StaticModels` and `ContextWindowFor`.
