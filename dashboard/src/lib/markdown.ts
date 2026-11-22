// A small, safe Markdown reader for social post bodies (Reddit, HN, Dev.to,
// LinkedIn). Posts are written by strangers, so nothing here produces HTML:
// it returns plain data that the component renders as React text. Supported:
// paragraphs (single line breaks kept), headings, bullet and numbered lists,
// quotes, fenced code, **bold**, *italic*, `code`, [label](http link) and bare
// http(s) links. Images and raw HTML stay as text.

export type Inline =
  | { kind: "text"; text: string }
  | { kind: "bold"; children: Inline[] }
  | { kind: "italic"; children: Inline[] }
  | { kind: "code"; text: string }
  | { kind: "link"; href: string; children: Inline[] };

export type Block =
  | { kind: "paragraph"; lines: Inline[][] }
  | { kind: "heading"; level: number; content: Inline[] }
  | { kind: "list"; ordered: boolean; items: Inline[][] }
  | { kind: "quote"; lines: Inline[][] }
  | { kind: "code"; text: string };

const ENTITIES: Record<string, string> = { "&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": '"', "&#39;": "'", "&#x27;": "'", "&nbsp;": " " };

// A backslash-escaped character ("\\*") is swapped for a private-use
// character before parsing, so it can never open bold or italic, and put
// back when the text is shown.
const ESC_BASE = 0xe100;
function protect(s: string): string {
  return s.replace(/\\([\\`*_[\]()#+\-.!>~|])/g, (_, c: string) => String.fromCharCode(ESC_BASE + c.charCodeAt(0)));
}

/** Undo HTML entities and the escapes protect() hid. */
function unescape(s: string): string {
  return s
    .replace(/&(amp|lt|gt|quot|#39|#x27|nbsp);/g, (m) => ENTITIES[m] ?? m)
    .replace(/[\ue100-\ue1ff]/g, (c) => String.fromCharCode(c.charCodeAt(0) - ESC_BASE));
}

function safeHref(url: string): string | null {
  try {
    const u = new URL(url);
    return u.protocol === "http:" || u.protocol === "https:" ? u.href : null;
  } catch {
    return null;
  }
}

// One pass over the line: the earliest match of any inline form wins.
const INLINE = /(`[^`\n]+`)|(\*\*[^*\n]+?\*\*|__[^_\n]+?__)|(\*[^*\s][^*\n]*?\*)|(\[[^\]\n]{1,300}\]\((https?:\/\/[^\s)]{1,800})\))|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])/;

export function parseInline(raw: string): Inline[] {
  return inline(protect(raw));
}

function inline(raw: string): Inline[] {
  const out: Inline[] = [];
  let rest = raw;
  while (rest) {
    const m = INLINE.exec(rest);
    if (!m) {
      out.push({ kind: "text", text: unescape(rest) });
      break;
    }
    if (m.index > 0) out.push({ kind: "text", text: unescape(rest.slice(0, m.index)) });
    const [whole, code, bold, italic, link, linkURL, bare] = m;
    if (code) out.push({ kind: "code", text: code.slice(1, -1) });
    else if (bold) out.push({ kind: "bold", children: inline(bold.slice(2, -2)) });
    else if (italic) out.push({ kind: "italic", children: inline(italic.slice(1, -1)) });
    else if (link) {
      const href = safeHref(linkURL);
      const label = inline(link.slice(1, link.indexOf("](")));
      out.push(href ? { kind: "link", href, children: label } : { kind: "text", text: unescape(link) });
    } else if (bare) {
      const href = safeHref(bare);
      out.push(href ? { kind: "link", href, children: [{ kind: "text", text: bare }] } : { kind: "text", text: bare });
    }
    rest = rest.slice(m.index + whole.length);
  }
  return out;
}

const HEADING = /^(#{1,6})\s+(.*)$/;
const BULLET = /^\s{0,3}[-*+•]\s+(.*)$/;
const NUMBERED = /^\s{0,3}\d{1,3}[.)]\s+(.*)$/;
const QUOTE = /^\s{0,3}>\s?(.*)$/;
const FENCE = /^\s{0,3}```/;

export function parseMarkdown(text: string): Block[] {
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) blocks.push({ kind: "paragraph", lines: para.map(parseInline) });
    para = [];
  };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (FENCE.test(line)) {
      flush();
      const body: string[] = [];
      for (i++; i < lines.length && !FENCE.test(lines[i]); i++) body.push(lines[i]);
      blocks.push({ kind: "code", text: unescape(body.join("\n")) });
      continue;
    }
    if (!line.trim()) {
      flush();
      continue;
    }
    const h = HEADING.exec(line);
    if (h) {
      flush();
      blocks.push({ kind: "heading", level: h[1].length, content: parseInline(h[2]) });
      continue;
    }
    const listKind = BULLET.test(line) ? "bullet" : NUMBERED.test(line) ? "numbered" : null;
    if (listKind) {
      flush();
      const re = listKind === "bullet" ? BULLET : NUMBERED;
      const items: Inline[][] = [];
      for (; i < lines.length && re.test(lines[i]); i++) items.push(parseInline(re.exec(lines[i])![1]));
      i--;
      blocks.push({ kind: "list", ordered: listKind === "numbered", items });
      continue;
    }
    if (QUOTE.test(line)) {
      flush();
      const quoted: Inline[][] = [];
      for (; i < lines.length && QUOTE.test(lines[i]); i++) quoted.push(parseInline(QUOTE.exec(lines[i])![1]));
      i--;
      blocks.push({ kind: "quote", lines: quoted });
      continue;
    }
    para.push(line);
  }
  flush();
  return blocks;
}

/** The post as plain text, for one- or two-line previews. */
export function stripMarkdown(text: string): string {
  return unescape(
    protect(text)
      .replace(/```[\s\S]*?```/g, " ")
      .replace(/^\s{0,3}(#{1,6}|>|[-*+•]|\d{1,3}[.)])\s+/gm, "")
      .replace(/\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)/g, "$1")
      .replace(/(\*\*|__)(.+?)\1/g, "$2")
      .replace(/\*([^*\s][^*\n]*?)\*/g, "$1")
      .replace(/`([^`\n]+)`/g, "$1"),
  );
}
