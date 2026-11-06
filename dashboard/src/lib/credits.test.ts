import { test } from "node:test";
import assert from "node:assert/strict";
import { creditState, formatPrice } from "./credits.ts";

test("creditState: out, low (under 10% of the monthly allowance), ok", () => {
  assert.equal(creditState(0, 5000, true), "out");
  assert.equal(creditState(499, 5000, true), "low");
  assert.equal(creditState(500, 5000, true), "ok");
  assert.equal(creditState(29, 300, true), "low");
  // Billing not enforced (self-host): never warn.
  assert.equal(creditState(0, 300, false), "ok");
});

test("formatPrice: whole dollars, Free for zero", () => {
  assert.equal(formatPrice(4900), "$49");
  assert.equal(formatPrice(14900), "$149");
  assert.equal(formatPrice(1000), "$10");
  assert.equal(formatPrice(0), "Free");
});
