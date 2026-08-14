// Minimal markdown renderer for agent replies. internal/cli/style.go's
// FormatMarkdown does the terminal equivalent (inline bold/italic/code via
// ANSI); a browser can afford block-level structure too, so this also
// handles fenced code, headings, lists, blockquotes, and links.
//
// Everything is built as DOM nodes with text going through .textContent,
// never innerHTML, so model output cannot inject markup. Link hrefs are
// scheme-checked for the same reason.

function isSafeHref(href: string): boolean {
  const trimmed = href.trim().toLowerCase();
  return (
    trimmed.startsWith("http://") ||
    trimmed.startsWith("https://") ||
    trimmed.startsWith("mailto:") ||
    trimmed.startsWith("#") ||
    trimmed.startsWith("/")
  );
}

// renderInline handles `code`, ***bold italic***, **bold**, __bold__,
// *italic*, _italic_, and [text](href), matching FormatMarkdown's inline
// set plus links.
function renderInline(text: string, into: Node) {
  let i = 0;
  let plain = "";

  const flush = () => {
    if (plain) {
      into.appendChild(document.createTextNode(plain));
      plain = "";
    }
  };

  const emit = (tag: string, content: string) => {
    flush();
    const el = document.createElement(tag);
    renderInline(content, el);
    into.appendChild(el);
  };

  while (i < text.length) {
    const rest = text.slice(i);

    if (text[i] === "`") {
      const end = text.indexOf("`", i + 1);
      if (end !== -1) {
        flush();
        const code = document.createElement("code");
        code.textContent = text.slice(i + 1, end);
        into.appendChild(code);
        i = end + 1;
        continue;
      }
    }

    const link = /^\[([^\]]*)\]\(([^)\s]+)\)/.exec(rest);
    if (link) {
      flush();
      const [, label, href] = link;
      if (isSafeHref(href!)) {
        const a = document.createElement("a");
        a.href = href!;
        a.target = "_blank";
        a.rel = "noopener noreferrer";
        renderInline(label!, a);
        into.appendChild(a);
      } else {
        into.appendChild(document.createTextNode(link[0]));
      }
      i += link[0].length;
      continue;
    }

    let matched = false;
    for (const [marker, tag] of [
      ["***", "strong-em"],
      ["**", "strong"],
      ["__", "strong"],
    ] as const) {
      if (rest.startsWith(marker)) {
        const end = text.indexOf(marker, i + marker.length);
        if (end !== -1) {
          const content = text.slice(i + marker.length, end);
          if (tag === "strong-em") {
            flush();
            const strong = document.createElement("strong");
            const em = document.createElement("em");
            renderInline(content, em);
            strong.appendChild(em);
            into.appendChild(strong);
          } else {
            emit(tag, content);
          }
          i = end + marker.length;
          matched = true;
          break;
        }
      }
    }
    if (matched) continue;

    // Single-char emphasis only at a word boundary, so identifiers like
    // some_var_name and a*b don't get mangled.
    if ((text[i] === "*" || text[i] === "_") && /[\s(>]|^$/.test(text[i - 1] ?? "")) {
      const marker = text[i]!;
      const end = text.indexOf(marker, i + 1);
      if (end !== -1 && end > i + 1) {
        emit("em", text.slice(i + 1, end));
        i = end + 1;
        continue;
      }
    }

    plain += text[i];
    i++;
  }
  flush();
}

// render turns a full markdown string into block elements appended to a
// fragment. Called fresh on every streaming delta, so it must stay cheap
// and fully deterministic.
export function renderMarkdown(src: string): DocumentFragment {
  const frag = document.createDocumentFragment();
  const lines = src.split("\n");
  let i = 0;

  while (i < lines.length) {
    const line = lines[i]!;

    const fence = /^```(\S*)\s*$/.exec(line);
    if (fence) {
      const lang = fence[1] ?? "";
      const body: string[] = [];
      i++;
      while (i < lines.length && !/^```\s*$/.test(lines[i]!)) {
        body.push(lines[i]!);
        i++;
      }
      i++; // closing fence (or end of input mid-stream)
      const pre = document.createElement("pre");
      const code = document.createElement("code");
      if (lang) code.dataset.lang = lang;
      code.textContent = body.join("\n");
      pre.appendChild(code);
      frag.appendChild(pre);
      continue;
    }

    const heading = /^(#{1,6})\s+(.*)$/.exec(line);
    if (heading) {
      const h = document.createElement(`h${heading[1]!.length}`);
      renderInline(heading[2]!, h);
      frag.appendChild(h);
      i++;
      continue;
    }

    if (/^\s*>\s?/.test(line)) {
      const body: string[] = [];
      while (i < lines.length && /^\s*>\s?/.test(lines[i]!)) {
        body.push(lines[i]!.replace(/^\s*>\s?/, ""));
        i++;
      }
      const quote = document.createElement("blockquote");
      quote.appendChild(renderMarkdown(body.join("\n")));
      frag.appendChild(quote);
      continue;
    }

    if (/^\s*([-*+])\s+/.test(line) || /^\s*\d+[.)]\s+/.test(line)) {
      const ordered = /^\s*\d+[.)]\s+/.test(line);
      const list = document.createElement(ordered ? "ol" : "ul");
      while (i < lines.length) {
        const m = ordered ? /^\s*\d+[.)]\s+(.*)$/.exec(lines[i]!) : /^\s*[-*+]\s+(.*)$/.exec(lines[i]!);
        if (!m) break;
        const li = document.createElement("li");
        renderInline(m[1]!, li);
        list.appendChild(li);
        i++;
      }
      frag.appendChild(list);
      continue;
    }

    if (line.trim() === "") {
      i++;
      continue;
    }

    // Paragraph: consume until a blank line or the start of another block.
    const body: string[] = [];
    while (
      i < lines.length &&
      lines[i]!.trim() !== "" &&
      !/^```/.test(lines[i]!) &&
      !/^#{1,6}\s/.test(lines[i]!) &&
      !/^\s*>\s?/.test(lines[i]!) &&
      !/^\s*([-*+])\s+/.test(lines[i]!) &&
      !/^\s*\d+[.)]\s+/.test(lines[i]!)
    ) {
      body.push(lines[i]!);
      i++;
    }
    const p = document.createElement("p");
    renderInline(body.join("\n"), p);
    frag.appendChild(p);
  }

  return frag;
}
