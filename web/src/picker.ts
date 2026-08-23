import type { CatalogItem } from "./api";

// A minimal port of internal/tui/picker.go's commandPicker: a filterable,
// keyboard-navigable overlay list. Filtering matches the TUI's applyQuery —
// a case-insensitive substring match across label, group, value, meta, and
// provider.
// onHighlight fires as the selection moves, so a caller can preview the
// highlighted item the way internal/tui/model.go previews a theme while
// arrowing through the theme picker.
export function openPicker(
  title: string,
  items: CatalogItem[],
  onHighlight?: (item: CatalogItem | null) => void,
): Promise<CatalogItem | null> {
  return new Promise((resolve) => {
    const overlay = document.createElement("div");
    overlay.className = "picker-overlay";

    const box = document.createElement("div");
    box.className = "picker-box";
    box.setAttribute("role", "dialog");
    box.setAttribute("aria-modal", "true");
    box.setAttribute("aria-label", title);
    overlay.appendChild(box);

    const titleEl = document.createElement("div");
    titleEl.className = "picker-title";
    titleEl.textContent = title;
    titleEl.id = "picker-title-el";
    box.setAttribute("aria-labelledby", "picker-title-el");
    box.appendChild(titleEl);

    const input = document.createElement("input");
    input.className = "picker-filter";
    input.placeholder = "type to filter…";
    input.autocomplete = "off";
    input.spellcheck = false;
    input.setAttribute("aria-label", "Filter options");
    box.appendChild(input);

    const list = document.createElement("div");
    list.className = "picker-list";
    list.setAttribute("role", "listbox");
    box.appendChild(list);

    const hint = document.createElement("div");
    hint.className = "picker-hint";
    box.appendChild(hint);

    let filtered = items;
    let selected = 0;

    function close(result: CatalogItem | null) {
      document.removeEventListener("keydown", onKeydown, true);
      overlay.remove();
      resolve(result);
    }

    function render() {
      list.replaceChildren();
      let lastGroup = "";
      filtered.forEach((item, i) => {
        if (item.group && item.group !== lastGroup) {
          const groupEl = document.createElement("div");
          groupEl.className = "picker-group";
          groupEl.textContent = item.group;
          list.appendChild(groupEl);
          lastGroup = item.group;
        }
        const row = document.createElement("div");
        row.className = "picker-row" + (i === selected ? " selected" : "");
        row.setAttribute("role", "option");
        row.setAttribute("aria-selected", i === selected ? "true" : "false");
        const label = document.createElement("span");
        label.textContent = item.label;
        const meta = document.createElement("span");
        meta.className = "meta";
        meta.textContent = item.meta || item.provider;
        row.appendChild(label);
        row.appendChild(meta);
        row.addEventListener("click", () => close(item));
        // Hover is a pure CSS affordance. It deliberately does not move
        // `selected`: the keyboard cursor is what Enter acts on, and letting
        // a resting mouse hijack it meant arrowing to one row and pressing
        // Enter could choose whichever row the pointer happened to sit over.
        // Not touching the DOM on hover also keeps a row from being rebuilt
        // out from under an in-progress click.
        list.appendChild(row);
      });
      if (filtered.length === 0) {
        const empty = document.createElement("div");
        empty.className = "picker-empty";
        empty.textContent = "no matching items";
        list.appendChild(empty);
      }
      hint.textContent = `↑/↓ move · Enter select · Esc close (${filtered.length ? selected + 1 : 0}/${filtered.length})`;
      const selectedEl = list.querySelector(".picker-row.selected");
      selectedEl?.scrollIntoView({ block: "nearest" });
      onHighlight?.(filtered[selected] ?? null);
    }

    function applyFilter() {
      const q = input.value.trim().toLowerCase();
      filtered = q
        ? items.filter((item) =>
            `${item.label} ${item.group} ${item.value} ${item.meta} ${item.provider}`.toLowerCase().includes(q),
          )
        : items;
      selected = 0;
      render();
    }

    function onKeydown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        close(null);
      } else if (e.key === "ArrowDown") {
        e.preventDefault();
        if (filtered.length) selected = (selected + 1) % filtered.length;
        render();
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        if (filtered.length) selected = (selected - 1 + filtered.length) % filtered.length;
        render();
      } else if (e.key === "Enter") {
        e.preventDefault();
        const item = filtered[selected];
        if (item) close(item);
      }
    }

    overlay.addEventListener("click", (e) => {
      if (e.target === overlay) close(null);
    });
    input.addEventListener("input", applyFilter);
    document.addEventListener("keydown", onKeydown, true);

    document.body.appendChild(overlay);
    render();
    input.focus();
  });
}
