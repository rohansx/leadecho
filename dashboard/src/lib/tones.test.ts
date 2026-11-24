import { test } from "node:test";
import assert from "node:assert/strict";
import { scoreBand, tagTone } from "./tones.ts";

test("scoreBand follows the backend tiers: 6+ is a lead, 4+ worth watching", () => {
  assert.equal(scoreBand(9.2).label, "Strong fit");
  assert.equal(scoreBand(8).label, "Strong fit");
  assert.equal(scoreBand(7.9).label, "Good fit");
  assert.equal(scoreBand(6).label, "Good fit");
  assert.equal(scoreBand(5.9).label, "Worth watching");
  assert.equal(scoreBand(4).label, "Worth watching");
  assert.equal(scoreBand(3.9).label, "Low fit");
  assert.equal(scoreBand(null).label, "Not scored");
});

test("tagTone colours buying signals apart from noise", () => {
  assert.notEqual(tagTone("buying_intent"), tagTone("promotional"));
  assert.equal(tagTone("promotional"), tagTone("not_a_known_tag"));
});
