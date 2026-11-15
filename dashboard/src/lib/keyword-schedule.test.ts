import { test } from "node:test";
import assert from "node:assert/strict";
import { effectiveCadence, paidSlowdownPlan, platformCost, scheduleSummary, truncate, yieldLine } from "./keyword-schedule.ts";

const rates = { twitter: 0.00075, reddit: 0.0009, hackernews: 0, bluesky: 0 };

test("effectiveCadence: stored wins, else 24h paid / 1h free", () => {
  assert.equal(effectiveCadence("twitter", {}, rates), "24h");
  assert.equal(effectiveCadence("hackernews", {}, rates), "1h");
  assert.equal(effectiveCadence("twitter", { twitter: "1h" }, rates), "1h");
  assert.equal(effectiveCadence("twitter", { twitter: "bogus" }, rates), "24h");
});

test("platformCost matches backend preview math", () => {
  assert.equal(platformCost("twitter", "1h", rates), 0.54); // 720 calls
  assert.equal(platformCost("reddit", "4h", rates), 0.162); // 180 calls
  assert.equal(platformCost("hackernews", "15m", rates), 0);
});

test("scheduleSummary groups paid and free", () => {
  assert.equal(scheduleSummary(["twitter", "reddit", "hackernews"], {}, rates), "Paid every 24h · free every 1h");
  assert.equal(scheduleSummary(["twitter", "reddit"], { twitter: "1h" }, rates), "Paid every 1h/24h");
  assert.equal(scheduleSummary(["bluesky"], { bluesky: "30m" }, rates), "Free every 30m");
  assert.equal(scheduleSummary([], {}, rates), "No platforms");
});

test("paidSlowdownPlan only slows paid platforms faster than 24h", () => {
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
    { id: "a", cadence: { twitter: "24h", hackernews: "15m" } },
    { id: "c", cadence: { reddit: "24h" } },
  ]);
  // only active "a" counts: 0.54 -> 0.0225
  assert.equal(plan.saving, 0.52);
});

test("truncate", () => {
  assert.deepEqual(truncate([1, 2, 3, 4, 5], 3, false), { shown: [1, 2, 3], hidden: 2 });
  assert.deepEqual(truncate([1, 2, 3, 4, 5], 3, true), { shown: [1, 2, 3, 4, 5], hidden: 0 });
  assert.deepEqual(truncate([1, 2], 3, false), { shown: [1, 2], hidden: 0 });
});

test("yieldLine says what a keyword brought in", () => {
  assert.equal(
    yieldLine({ posts: 535, passed: 33, leads: 0, asks: 12, days: 14 }),
    "535 posts, 0 leads in 14 days · 33 past the noise filter · 12 state a need",
  );
  assert.equal(yieldLine({ posts: 63, passed: 20, leads: 6, asks: 1, days: 14 }), "63 posts, 6 leads in 14 days · 20 past the noise filter · 1 states a need");
  assert.equal(yieldLine({ posts: 1, passed: 0, leads: 1, asks: 0, days: 14 }), "1 post, 1 lead in 14 days · 0 past the noise filter");
  // Zero hits is simply zero.
  assert.equal(yieldLine({ posts: 0, passed: 0, leads: 0, asks: 0, days: 14 }), "0 posts, 0 leads in 14 days");
});
