import { test } from "node:test";
import assert from "node:assert/strict";
import { effectiveCadence, paidSlowdownPlan, platformCost, scheduleSummary, truncate } from "./keyword-schedule.ts";

const rates = { twitter: 0.00075, reddit: 0.0009, hackernews: 0, bluesky: 0 };

test("effectiveCadence: stored wins, else 12h paid / 1h free", () => {
  assert.equal(effectiveCadence("twitter", {}, rates), "12h");
  assert.equal(effectiveCadence("hackernews", {}, rates), "1h");
  assert.equal(effectiveCadence("twitter", { twitter: "1h" }, rates), "1h");
  assert.equal(effectiveCadence("twitter", { twitter: "bogus" }, rates), "12h");
});

test("platformCost matches backend preview math", () => {
  assert.equal(platformCost("twitter", "1h", rates), 0.54); // 720 calls
  assert.equal(platformCost("reddit", "4h", rates), 0.162); // 180 calls
  assert.equal(platformCost("hackernews", "15m", rates), 0);
});

test("scheduleSummary groups paid and free", () => {
  assert.equal(scheduleSummary(["twitter", "reddit", "hackernews"], {}, rates), "Paid every 12h · free every 1h");
  assert.equal(scheduleSummary(["twitter", "reddit"], { twitter: "1h" }, rates), "Paid every 1h/12h");
  assert.equal(scheduleSummary(["bluesky"], { bluesky: "30m" }, rates), "Free every 30m");
  assert.equal(scheduleSummary([], {}, rates), "No platforms");
});

test("paidSlowdownPlan only slows paid platforms faster than 12h", () => {
  const plan = paidSlowdownPlan(
    [
      { id: "a", platforms: ["twitter", "hackernews"], cadence: { twitter: "1h", hackernews: "15m" }, isActive: true },
      { id: "b", platforms: ["twitter"], cadence: { twitter: "24h" }, isActive: true },
      { id: "c", platforms: ["reddit"], cadence: { reddit: "30m" }, isActive: false },
      { id: "d", platforms: ["twitter"], cadence: {}, isActive: true },
    ],
    rates,
  );
  assert.deepEqual(plan.updates, [
    { id: "a", cadence: { twitter: "12h", hackernews: "15m" } },
    { id: "c", cadence: { reddit: "12h" } },
  ]);
  // only active "a" counts: 0.54 -> 0.045
  assert.equal(plan.saving, 0.5);
});

test("truncate", () => {
  assert.deepEqual(truncate([1, 2, 3, 4, 5], 3, false), { shown: [1, 2, 3], hidden: 2 });
  assert.deepEqual(truncate([1, 2, 3, 4, 5], 3, true), { shown: [1, 2, 3, 4, 5], hidden: 0 });
  assert.deepEqual(truncate([1, 2], 3, false), { shown: [1, 2], hidden: 0 });
});
