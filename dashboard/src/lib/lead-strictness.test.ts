import { test } from "node:test";
import assert from "node:assert/strict";
import { STRICTNESS_LEVELS, leadLineText } from "./lead-strictness.ts";

test("three levels, strictest first, balanced in the middle", () => {
  assert.deepEqual(STRICTNESS_LEVELS.map((l) => l.value), ["strict", "balanced", "loose"]);
});

test("leadLineText shows the score without a trailing .0", () => {
  assert.equal(leadLineText(6), "Posts scoring 6 or more with buying intent are leads.");
  assert.equal(leadLineText(7.5), "Posts scoring 7.5 or more with buying intent are leads.");
});
