import { test } from "node:test";
import assert from "node:assert/strict";
import { askRole, askSource, coverageText, followUpEstimate, peopleLine, splitThemes, type DemandTheme } from "./demand.ts";

const theme = (over: Partial<DemandTheme>): DemandTheme => ({
  id: "t1", title: "T", summary: "", coverage: "gap", status: "open", competitors: [],
  people: 0, buyers: 0, new_this_week: 0, updated_at: "2026-11-11T00:00:00Z", asks: [], ...over,
});

test("coverageText says how the product stands in plain words", () => {
  assert.deepEqual(coverageText("covered"), { label: "You already do this", tone: "good" });
  assert.deepEqual(coverageText("partial"), { label: "Partly covered", tone: "warn" });
  assert.deepEqual(coverageText("gap"), { label: "Not covered yet", tone: "gap" });
  assert.deepEqual(coverageText("outside"), { label: "Outside your product", tone: "muted" });
  assert.deepEqual(coverageText("anything-else"), { label: "Not covered yet", tone: "gap" });
});

test("peopleLine counts people, buyers and what is new", () => {
  assert.equal(peopleLine(theme({ people: 9, buyers: 4, new_this_week: 3 })), "9 people · 4 fit your buyer profile · 3 new this week");
  assert.equal(peopleLine(theme({ people: 1, buyers: 1, new_this_week: 0 })), "1 person · 1 fits your buyer profile");
  // "new this week" only says something when some asks are older.
  assert.equal(peopleLine(theme({ people: 2, buyers: 0, new_this_week: 2 })), "2 people");
});

test("followUpEstimate counts buyers only, caps the drafts and prices them", () => {
  assert.deepEqual(followUpEstimate(theme({ people: 4, buyers: 3 }), 10, 5), { drafts: 3, credits: 15 });
  assert.deepEqual(followUpEstimate(theme({ people: 40, buyers: 37 }), 10, 5), { drafts: 10, credits: 50 });
  assert.deepEqual(followUpEstimate(theme({ people: 4, buyers: 0 }), 10, 5), { drafts: 0, credits: 0 });
});

// Needs raised once are folded into one group by the server (demand.Fold,
// tested in Go), so the page only separates open, outside and shipped, and
// keeps the server's order: most buyers first.
test("splitThemes separates open, outside and shipped themes and keeps the server's order", () => {
  const all = [
    theme({ id: "a", people: 3, buyers: 2 }),
    theme({ id: "d", people: 5, coverage: "outside" }),
    theme({ id: "b", people: 6, buyers: 0, coverage: "objection" }),
    theme({ id: "c", people: 1, status: "shipped" }),
  ];
  const got = splitThemes(all);
  assert.deepEqual(got.outside.map((t) => t.id), ["d"]);
  assert.deepEqual(got.open.map((t) => t.id), ["a", "b"]);
  assert.deepEqual(got.shipped.map((t) => t.id), ["c"]);
});

test("an objection is never shown as something the product already does", () => {
  assert.deepEqual(coverageText("objection"), { label: "Objection to your approach", tone: "warn" });
});

test("peopleLine says how many are buyers and how many are not", () => {
  assert.equal(
    peopleLine(theme({ people: 9, buyers: 2, non_buyers: 5, new_this_week: 3 })),
    "9 people · 2 fit your buyer profile · 5 not buyers · 3 new this week",
  );
  assert.equal(peopleLine(theme({ people: 3, buyers: 0, non_buyers: 1 })), "3 people · 1 not a buyer");
});

test("askSource says where a summary came from without guessing the role", () => {
  assert.equal(askSource({ buyer: true, platform: "reddit" }), "Summary of a post by a buyer on reddit");
  assert.equal(askSource({ buyer: false, platform: "twitter" }), "Summary of a post on twitter, role unknown");
  // Someone affected by the problem who would not buy (a candidate) is said to be so.
  assert.equal(askSource({ buyer: false, non_buyer: true, platform: "reddit" }), "Summary of a post on reddit, not a buyer");
  assert.equal(askRole({ buyer: true }), "fits your buyer profile");
});
