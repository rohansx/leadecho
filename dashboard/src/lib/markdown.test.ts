import { test } from "node:test";
import assert from "node:assert/strict";
import { parseInline, parseMarkdown, stripMarkdown } from "./markdown.ts";

test("the Hermes post from the inbox renders bold and keeps line breaks", () => {
  const blocks = parseMarkdown("I've been using Hermes.\nHere's what I've tried:\n**OpenRouter:** Free models have limits.\n**What providers are you using?**");
  assert.equal(blocks.length, 1);
  const p = blocks[0];
  assert.equal(p.kind, "paragraph");
  if (p.kind !== "paragraph") return;
  assert.equal(p.lines.length, 4);
  assert.deepEqual(p.lines[2][0], { kind: "bold", children: [{ kind: "text", text: "OpenRouter:" }] });
});

test("lists, headings, quotes and code fences", () => {
  const kinds = parseMarkdown("# Title\n\n- one\n- two\n\n1. first\n2) second\n\n> quoted\n\n```\nx = *not italic*\n```").map((b) => b.kind);
  assert.deepEqual(kinds, ["heading", "list", "list", "quote", "code"]);
});

test("links are http(s) only; javascript and images stay text", () => {
  assert.deepEqual(parseInline("[site](https://a.io/x)")[0], { kind: "link", href: "https://a.io/x", children: [{ kind: "text", text: "site" }] });
  assert.equal(parseInline("[x](javascript:alert(1))").some((n) => n.kind === "link"), false);
  assert.equal(parseInline("see https://b.io/path.").find((n) => n.kind === "link")?.kind, "link");
  // the trailing full stop is not part of the link
  const link = parseInline("see https://b.io/path.").find((n) => n.kind === "link");
  assert.equal(link?.kind === "link" && link.href, "https://b.io/path");
});

test("entities and escapes from scraped posts are undone", () => {
  assert.deepEqual(parseInline("Tom &amp; Jerry \\*not bold\\*"), [{ kind: "text", text: "Tom & Jerry *not bold*" }]);
});

test("stripMarkdown for list previews", () => {
  assert.equal(stripMarkdown("**ChatGPT Go ($10):** Better value\n- [docs](https://x.io) and `code`"), "ChatGPT Go ($10): Better value\ndocs and code");
});
