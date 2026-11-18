import { test } from "node:test";
import assert from "node:assert/strict";
import { topUpCount } from "./keyword-top-up.ts";

test("topUpCount fills to the target within the plan limit", () => {
  assert.equal(topUpCount({ active_keywords: 11, keyword_limit: 50, target: 30 }), 19);
  // Close to the target: still offer a useful batch.
  assert.equal(topUpCount({ active_keywords: 28, keyword_limit: 50, target: 30 }), 5);
  // Never past the plan limit.
  assert.equal(topUpCount({ active_keywords: 47, keyword_limit: 50, target: 30 }), 3);
  assert.equal(topUpCount({ active_keywords: 50, keyword_limit: 50, target: 30 }), 0);
  // Capped per batch.
  assert.equal(topUpCount({ active_keywords: 0, keyword_limit: 100, target: 60 }), 40);
});
