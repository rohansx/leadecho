import { test } from "node:test";
import assert from "node:assert/strict";
import { leadFacts } from "./lead-facts.ts";

test("leadFacts keeps well-formed facts in order and ignores the rest", () => {
  assert.deepEqual(leadFacts(undefined), []);
  assert.deepEqual(leadFacts({ facts: "nope" }), []);
  assert.deepEqual(
    leadFacts({
      facts: [{ name: "Role", value: "CTO" }, null, { name: "Team size", value: 12 }, { name: "", value: "x" }, { name: "Current tool", value: "Jira" }],
    }),
    [
      { name: "Role", value: "CTO" },
      { name: "Current tool", value: "Jira" },
    ],
  );
});
