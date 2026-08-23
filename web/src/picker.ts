import type { CatalogItem } from "./api";

// Each open picker gets its own id namespace. The ids are what wire the filter
// input to the highlighted row through aria-activedescendant, so they must not
// collide with a picker that is still tearing down.
let pickerSeq = 0;

// One entry in the rendered list. Group headers are interleaved with the
// options rather than wrapping them, so DOM position and the filtered-item
// index diverge. Each option therefore carries its own index and id instead of
// leaving either to be recovered from the DOM.
export type PickerEntry =
  | { kind: "group"; text: string }
  | { kind: "option"; item: CatalogItem; id: string; index: number; selected: boolean };

// planRows decides what the list contains. It is separate from the rendering
// because it owns the index and id bookkeeping the ARIA wiring depends on, and
// that is the part worth testing without a DOM.
export function planRows(items: CatalogItem[], selected: number, idPrefix: string): PickerEntry[] {
  const entries: PickerEntry[] = [];
  let lastGroup = "";
  items.forEach((item, index) => {
    if (item.group && item.group !== lastGroup) {
      entries.push({ kind: "group", text: item.group });
      lastGroup = item.group;
    }
    entries.push({ kind: "option", item, id: `${idPrefix}-opt-${index}`, index, selected: index === selected });
  });
  return entries;
}

// activeDescendantId returns the id of the highlighted option, or "" when the
// filter matched nothing and there is no cursor to point at.
export function activeDescendantId(entries: PickerEntry[]): string {
  const active = entries.find((e) => e.kind === "option" && e.selected);
  return active?.kind === "option" ? active.id : "";
}

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
    const uid = `picker-${++pickerSeq}`;
    // Whatever had focus when the picker opened gets it back on close.
    // Without this a keyboard user is dropped at the top of the document
    // instead of back in the composer they invoked the picker from.
    const restore = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const app = document.getElementById("app");

    const overlay = document.createElement("div");
    overlay.className = "picker-overlay";

    const box = document.createElement("div");
    box.className = "picker-box";
    box.setAttribute("role", "dialog");
    box.setAttribute("aria-modal", "true");
    box.setAttribute("aria-labelledby", `${uid}-title`);
    overlay.appendChild(box);

    const titleEl = document.createElement("div");
    titleEl.className = "picker-title";
    titleEl.id = `${uid}-title`;
    titleEl.textContent = title;
    box.appendChild(titleEl);

    const input = document.createElement("input");
    input.className = "picker-filter";
    input.placeholder = "type to filter…";
    input.autocomplete = "off";
    input.spellcheck = false;
    // Focus never leaves this input, so it is the element that has to carry
    // the list semantics: the combobox owns the listbox and names the row the
    // Enter key would take.
    input.setAttribute("role", "combobox");
    input.setAttribute("aria-expanded", "true");
    input.setAttribute("aria-autocomplete", "list");
    input.setAttribute("aria-controls", `${uid}-list`);
    input.setAttribute("aria-labelledby", `${uid}-title`);
    box.appendChild(input);

    const list = document.createElement("div");
    list.className = "picker-list";
    list.id = `${uid}-list`;
    list.setAttribute("role", "listbox");
    list.setAttribute("aria-labelledby", `${uid}-title`);
    box.appendChild(list);

    const hint = document.createElement("div");
    hint.className = "picker-hint";
    box.appendChild(hint);

    // The visible hint carries the match count too, but it is not a live
    // region: it also holds the static key legend, which would be re-read on
    // every keystroke. This one announces the count alone.
    const status = document.createElement("div");
    status.className = "sr-only";
    status.setAttribute("role", "status");
    box.appendChild(status);

    let filtered = items;
    let selected = 0;
    let announced = -1;

    function close(result: CatalogItem | null) {
      document.removeEventListener("keydown", onKeydown, true);
      overlay.remove();
      // Undo the background trap before restoring focus: focus() on an inert
      // subtree is ignored.
      app?.removeAttribute("inert");
      restore?.focus();
      resolve(result);
    }

    function render() {
      list.replaceChildren();
      const entries = planRows(filtered, selected, uid);
      entries.forEach((entry) => {
        if (entry.kind === "group") {
          const groupEl = document.createElement("div");
          groupEl.className = "picker-group";
          // A group header sitting between options would otherwise be an
          // unlabelled child of the listbox, breaking the run of options a
          // screen reader walks.
          groupEl.setAttribute("role", "presentation");
          groupEl.textContent = entry.text;
          list.appendChild(groupEl);
          return;
        }
        const row = document.createElement("div");
        row.id = entry.id;
        row.className = "picker-row" + (entry.selected ? " selected" : "");
        row.setAttribute("role", "option");
        row.setAttribute("aria-selected", entry.selected ? "true" : "false");
        const label = document.createElement("span");
        label.textContent = entry.item.label;
        const meta = document.createElement("span");
        meta.className = "meta";
        meta.textContent = entry.item.meta || entry.item.provider;
        row.appendChild(label);
        row.appendChild(meta);
        row.addEventListener("click", () => close(entry.item));
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
        // Same reason as the group headers: a non-option child of the listbox
        // would otherwise break the run of options a screen reader walks.
        empty.setAttribute("role", "presentation");
        empty.textContent = "no matching items";
        list.appendChild(empty);
      }
      // The selection is otherwise only a CSS class, which is announced as
      // nothing: this is what moves the screen reader cursor as you arrow.
      const activeId = activeDescendantId(entries);
      if (activeId) input.setAttribute("aria-activedescendant", activeId);
      else input.removeAttribute("aria-activedescendant");
      hint.textContent = `↑/↓ move · Enter select · Esc close (${filtered.length ? selected + 1 : 0}/${filtered.length})`;
      // Only when the count changes. Arrowing already speaks through
      // aria-activedescendant, and repeating the count would talk over it.
      if (filtered.length !== announced) {
        announced = filtered.length;
        status.textContent = filtered.length === 1 ? "1 match" : `${filtered.length} matches`;
      }
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
      } else if (e.key === "Tab") {
        // The overlay holds one focusable element, so Tab has nowhere to go
        // inside it. Swallow it rather than let focus walk off the dialog.
        e.preventDefault();
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

    // inert takes the page behind the dialog out of the tab order and the
    // accessibility tree, so the picker is not just visually modal.
    app?.setAttribute("inert", "");
    document.body.appendChild(overlay);
    render();
    input.focus();
  });
}
