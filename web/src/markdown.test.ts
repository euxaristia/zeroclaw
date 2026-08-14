import { expect, test, beforeAll } from "bun:test";

// The renderer builds real DOM nodes, so exercising it outside a browser
// needs a stand-in. This stub implements only the handful of DOM calls
// markdown.ts makes and serializes the result for comparison, which keeps
// the parsing logic under test without pulling in a DOM library.
class StubNode {
  children: unknown[] = [];
  appendChild(c: unknown) {
    this.children.push(c);
    return c;
  }
  replaceChildren(...c: unknown[]) {
    this.children = c;
  }
}
class StubText {
  constructor(public data: string) {}
}
class StubElement extends StubNode {
  dataset: Record<string, string> = {};
  href = "";
  target = "";
  rel = "";
  text: string | null = null;
  constructor(public tag: string) {
    super();
  }
  set textContent(v: string) {
    this.text = v;
    this.children = [];
  }
  get textContent(): string {
    return this.text ?? "";
  }
}
class StubFragment extends StubNode {}

function serialize(n: unknown): string {
  if (n instanceof StubText) return n.data;
  if (n instanceof StubFragment) return n.children.map(serialize).join("");
  if (n instanceof StubElement) {
    const inner = n.text !== null ? n.text : n.children.map(serialize).join("");
    return `<${n.tag}${n.href ? ` href="${n.href}"` : ""}>${inner}</${n.tag}>`;
  }
  return "";
}

let renderMarkdown: (src: string) => unknown;

beforeAll(async () => {
  (globalThis as unknown as { document: unknown }).document = {
    createElement: (t: string) => new StubElement(t),
    createTextNode: (d: string) => new StubText(d),
    createDocumentFragment: () => new StubFragment(),
  };
  ({ renderMarkdown } = await import("./markdown"));
});

const cases: [string, string][] = [
  ["plain text", "<p>plain text</p>"],
  ["**bold**", "<p><strong>bold</strong></p>"],
  // Regression: a marker run ending exactly at end-of-input used to fall
  // through the advance check and append the literal string "undefined".
  ["ends with **bold**", "<p>ends with <strong>bold</strong></p>"],
  ["trailing `code`", "<p>trailing <code>code</code></p>"],
  ["***both***", "<p><strong><em>both</em></strong></p>"],
  ["a *it* b", "<p>a <em>it</em> b</p>"],
  ["**a** and **b**", "<p><strong>a</strong> and <strong>b</strong></p>"],
  ["unclosed **bold", "<p>unclosed **bold</p>"],
  // Underscores inside identifiers must not become emphasis.
  ["some_var_name here", "<p>some_var_name here</p>"],
  ["# Head", "<h1>Head</h1>"],
  ["- one\n- two", "<ul><li>one</li><li>two</li></ul>"],
  ["1. one\n2. two", "<ol><li>one</li><li>two</li></ol>"],
  ["> quoted", "<blockquote><p>quoted</p></blockquote>"],
  ["```js\nlet x=1;\n```", "<pre><code>let x=1;</code></pre>"],
  ["para one\n\npara two", "<p>para one</p><p>para two</p>"],
  ["[hi](https://e.com)", '<p><a href="https://e.com">hi</a></p>'],
];

for (const [input, want] of cases) {
  test(`renders ${JSON.stringify(input)}`, () => {
    expect(serialize(renderMarkdown(input))).toBe(want);
  });
}

// Model output is untrusted, so a link scheme that could execute must fall
// back to literal text rather than becoming an anchor.
test("refuses javascript: links", () => {
  expect(serialize(renderMarkdown("[bad](javascript:alert(1))"))).toBe("<p>[bad](javascript:alert(1))</p>");
});
