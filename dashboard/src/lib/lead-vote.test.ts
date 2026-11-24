import { test } from "node:test";
import assert from "node:assert/strict";
import { lastVote, notAFitNote, voteKey } from "./lead-vote.ts";

test("lastVote reads up/down only; archive and spam flags are not votes", () => {
  assert.equal(lastVote({ scoring_metadata: { last_feedback: { label: "up" } } }), "up");
  assert.equal(lastVote({ scoring_metadata: { last_feedback: { label: "down", reason: "x" } } }), "down");
  assert.equal(lastVote({ scoring_metadata: { last_feedback: { label: "archived" } } }), null);
  assert.equal(lastVote({ scoring_metadata: null }), null);
  assert.equal(lastVote({}), null);
});

test("notAFitNote matches the archive reason the inbox always sent", () => {
  assert.equal(notAFitNote("Wrong company"), "Not a fit: wrong company");
});

const key = (k: string, extra: Partial<Parameters<typeof voteKey>[0]> = {}) =>
  voteKey({ key: k, metaKey: false, ctrlKey: false, altKey: false, target: { tagName: "BODY" }, ...extra });

test("voteKey maps y / n and ignores everything else", () => {
  assert.equal(key("y"), "up");
  assert.equal(key("n"), "down");
  assert.equal(key("Y"), null); // shift held
  assert.equal(key("j"), null);
  assert.equal(key("y", { target: null }), "up");
});

test("voteKey stays out of the way while typing or with a modifier", () => {
  for (const tagName of ["INPUT", "TEXTAREA", "SELECT"]) assert.equal(key("y", { target: { tagName } }), null);
  assert.equal(key("n", { target: { tagName: "DIV", isContentEditable: true } }), null);
  assert.equal(key("n", { metaKey: true }), null);
  assert.equal(key("n", { ctrlKey: true }), null);
  assert.equal(key("n", { altKey: true }), null);
  assert.equal(key("n", { repeat: true }), null);
});
