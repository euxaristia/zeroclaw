1.  **Refactor `internal/catalog/catalog.go` for O(1) Lookups (Performance Optimization)**:
    -   *Why:* The `catalog.go` file contained functions like `StaticModels`, `ContextWindowFor`, and `KeyedAuth` which were repeatedly allocating static slices or iterating through large lists (like `StaticModels("")`) on each call. This is inefficient, especially when called frequently (e.g., for every `/model` or `/status` request).
    -   *What:* Move the static lists (`Providers` list and `staticModels` list) to package-level variables and use the `init()` function to pre-compute maps for O(1) lookups.
    -   Use `run_in_bash_session` to execute the Python refactoring scripts I previously created to perform these changes.
    -   *Impact:* Reduces garbage collection overhead and CPU cycles for these lookup functions by avoiding repeated array iterations and slice allocations.
2.  **Verify refactoring**:
    -   Use `run_in_bash_session` with `cat internal/catalog/catalog.go` to inspect `internal/catalog/catalog.go` and confirm the refactor was applied correctly.
3.  **Run formatting and tests**:
    -   Use `run_in_bash_session` with commands `go fmt ./...` and `go test ./...` to ensure formatting and correctness.
4.  **Complete pre-commit steps:**
    -   Complete pre-commit steps to ensure proper testing, verification, review, and reflection are done.
5.  **Submit PR:**
    -   Use the `submit` tool to create PR with title "⚡ Bolt: [performance improvement] Pre-compute O(1) lookup maps in catalog"
