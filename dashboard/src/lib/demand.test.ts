import { test } from "node:test";
import assert from "node:assert/strict";
import { coverageText, followUpEstimate, peopleLine, type DemandTheme } from "./demand.ts";

const theme = (over: Partial<DemandTheme>): DemandTheme => ({
  id: "t1", title: "T", summary: "", coverage: "gap", status: "open", competitors: [],
  people: 0, buyers: 0, new_this_week: 0, updated_at: "2026-11-11T00:00:00Z", asks: [], ...over,
});

test("coverageText says how the product stands in plain words", () => {
  assert.deepEqual(coverageText("covered"), { label: "You already do this", tone: "good" });
  assert.deepEqual(coverageText("partial"), { label: "Partly covered", tone: "warn" });
  assert.deepEqual(coverageText("gap"), { label: "Not covered yet", tone: "gap" });
  assert.deepEqual(coverageText("anything-else"), { label: "Not covered yet", tone: "gap" });
});

test("peopleLine counts people, buyers and what is new", () => {
  assert.equal(peopleLine(theme({ people: 9, buyers: 4, new_this_week: 3 })), "9 people · 4 buyers · 3 new this week");
  assert.equal(peopleLine(theme({ people: 1, buyers: 1, new_this_week: 0 })), "1 person · 1 buyer");
  assert.equal(peopleLine(theme({ people: 2, buyers: 0, new_this_week: 2 })), "2 people · 2 new this week");
});

test("followUpEstimate caps the drafts and prices them", () => {
  assert.deepEqual(followUpEstimate(theme({ people: 4 }), 10, 5), { drafts: 4, credits: 20 });
  assert.deepEqual(followUpEstimate(theme({ people: 37 }), 10, 5), { drafts: 10, credits: 50 });
  assert.deepEqual(followUpEstimate(theme({ people: 0 }), 10, 5), { drafts: 0, credits: 0 });
});
