## 2024-05-24 - Layout Thrashing in LLM Streaming
**Learning:** Performing synchronous DOM layout reads (`scrollTop = scrollHeight`) and expensive stringification logic (`saveTranscript`) on every token of a fast SSE stream causes severe layout thrashing and UI sluggishness.
**Action:** Always batch high-frequency read/write operations (like scroll updates or state saving) using `requestAnimationFrame` when processing LLM stream chunks to guarantee a maximum of one expensive operation per frame (60 FPS).
