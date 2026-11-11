import { test } from "node:test";
import assert from "node:assert/strict";
import { parseResearch, sourceHost } from "./research.ts";

test("parseResearch reads a stored brief and keeps only web links", () => {
  const got = parseResearch({
    signal: "hiring",
    research: {
      what_they_do: "Sells vertical software.",
      why_now: "Plans to hire 250 people.",
      hook: "Many roles open at once.",
      facts: [
        { text: "Moved HQ to Austin.", url: "https://news.example/a" },
        { text: "Bad link.", url: "javascript:alert(1)" },
        { text: "", url: "https://news.example/b" },
        "not an object",
      ],
    },
  });
  assert.deepEqual(got, {
    whatTheyDo: "Sells vertical software.",
    whyNow: "Plans to hire 250 people.",
    hook: "Many roles open at once.",
    facts: [{ text: "Moved HQ to Austin.", url: "https://news.example/a" }],
  });
});

test("parseResearch returns null when there is nothing to show", () => {
  assert.equal(parseResearch(null), null);
  assert.equal(parseResearch({ signal: "hiring" }), null);
  assert.equal(parseResearch({ research: "text" }), null);
  assert.equal(
    parseResearch({
      research: { what_they_do: "", why_now: "", hook: "", facts: [] },
    }),
    null,
  );
});

test("sourceHost names the site a fact came from", () => {
  assert.equal(
    sourceHost("https://www.finance.yahoo.com/news/x"),
    "finance.yahoo.com",
  );
  assert.equal(sourceHost("not a url"), "source");
});
