import { test } from "node:test";
import assert from "node:assert/strict";
import { postedSince, isPostedRange } from "./posted-range.ts";

const now = new Date("2026-11-23T12:00:00Z");

test("postedSince turns a range into the RFC 3339 start of the window", () => {
  assert.equal(postedSince("24h", now), "2026-11-22T12:00:00.000Z");
  assert.equal(postedSince("7d", now), "2026-11-16T12:00:00.000Z");
  assert.equal(postedSince("30d", now), "2026-10-24T12:00:00.000Z");
  assert.equal(postedSince("90d", now), "2026-08-25T12:00:00.000Z");
});

test("no range, or one we do not know, means no date filter", () => {
  assert.equal(postedSince(undefined, now), undefined);
  assert.equal(postedSince("", now), undefined);
  assert.equal(postedSince("forever", now), undefined);
});

test("isPostedRange accepts only the offered ranges (URL input is untrusted)", () => {
  assert.equal(isPostedRange("7d"), true);
  assert.equal(isPostedRange("8d"), false);
  assert.equal(isPostedRange(7), false);
});
