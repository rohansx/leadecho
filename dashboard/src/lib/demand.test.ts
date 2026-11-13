import { test } from "node:test";
import assert from "node:assert/strict";
import { askSource, coverageText, followUpEstimate, peopleLine, splitThemes, type DemandTheme } from "./demand.ts";

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

test("splitThemes separates themes from needs raised once", () => {
  const all = [theme({ id: "a", people: 3 }), theme({ id: "b", people: 1 }), theme({ id: "c", people: 1, status: "shipped" })];
  all.push(theme({ id: "d", people: 5, coverage: "outside" }));
  const got = splitThemes(all, 2);
  assert.deepEqual(got.outside.map((t) => t.id), ["d"]);
  assert.deepEqual(got.open.map((t) => t.id), ["a"]);
  assert.deepEqual(got.once.map((t) => t.id), ["b"]);
  assert.deepEqual(got.shipped.map((t) => t.id), ["c"]);
});

test("askSource says where a summary came from without guessing the role", () => {
  assert.equal(askSource({ buyer: true, platform: "reddit" }), "Summary of a post by a buyer on reddit");
  assert.equal(askSource({ buyer: false, platform: "twitter" }), "Summary of a post on twitter, role unknown");
});
