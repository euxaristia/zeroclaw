## 2024-08-23 - Custom overlays need ARIA roles
**Learning:** Custom overlay dialogs and list boxes built with plain DOM elements lack implicit semantics, rendering them opaque to screen readers.
**Action:** Always add `role="dialog"`, `aria-modal="true"`, and an `aria-label` or `aria-labelledby` to custom dialogs, and use `role="listbox"`, `role="option"`, and `aria-selected` for custom select lists.
