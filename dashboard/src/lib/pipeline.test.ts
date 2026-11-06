import { test } from "node:test";
import assert from "node:assert/strict";
import { intentReason, leadIntent, leadScore, sortLeads, stageLabel, timeAgo } from "./pipeline.ts";
import type { Lead } from "./types.ts";

function lead(p: Partial<Lead>): Lead {
  return {
    id: "x", workspace_id: "w", mention_id: null, stage: "prospect",
    contact_name: null, contact_email: null, company: null, username: null,
    platform: null, profile_url: null, estimated_value: null, notes: null,
    tags: [], metadata: {}, created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z", ...p,
  };
}

test("stageLabel maps the DB enum to user-facing names", () => {
  assert.equal(stageLabel("prospect"), "To reply");
  assert.equal(stageLabel("qualified"), "Replied");
  assert.equal(stageLabel("engaged"), "In conversation");
  assert.equal(stageLabel("converted"), "Won");
  assert.equal(stageLabel("lost"), "Not a fit");
  assert.equal(stageLabel("weird"), "weird");
});

test("intentReason explains the lead in plain words", () => {
  assert.equal(intentReason("recommendation_ask"), "Asking for recommendations");
  assert.equal(intentReason("complaint"), "Unhappy with their current tool");
  assert.equal(intentReason(null), "Matched your keywords");
  assert.equal(intentReason("unknown"), "Matched your keywords");
});

test("intent/score prefer the mention, fall back to scorer metadata", () => {
  const meta = lead({ metadata: { intent: "buy_signal", relevance: 7.5 } });
  assert.equal(leadIntent(meta), "buy_signal");
  assert.equal(leadScore(meta), 7.5);
  const both = lead({
    metadata: { intent: "buy_signal", relevance: 7.5 },
    mention: { title: null, snippet: "", url: "u", intent: "comparison", score: 9, posted_at: null },
  });
  assert.equal(leadIntent(both), "comparison");
  assert.equal(leadScore(both), 9);
  assert.equal(leadScore(lead({})), null);
});

test("sortLeads: score desc, unscored last, then newest; does not mutate", () => {
  const input = [
    lead({ id: "none", created_at: "2026-01-05T00:00:00Z" }),
    lead({ id: "low", metadata: { relevance: 6 } }),
    lead({ id: "hiOld", metadata: { relevance: 9 }, created_at: "2026-01-01T00:00:00Z" }),
    lead({ id: "hiNew", metadata: { relevance: 9 }, created_at: "2026-01-03T00:00:00Z" }),
  ];
  assert.deepEqual(sortLeads(input).map((l) => l.id), ["hiNew", "hiOld", "low", "none"]);
  assert.equal(input[0].id, "none");
});

test("timeAgo", () => {
  const now = Date.parse("2026-01-02T00:00:00Z");
  assert.equal(timeAgo("2026-01-02T00:00:00Z", now), "just now");
  assert.equal(timeAgo("2026-01-01T23:15:00Z", now), "45m ago");
  assert.equal(timeAgo("2026-01-01T21:00:00Z", now), "3h ago");
  assert.equal(timeAgo("2025-12-30T00:00:00Z", now), "3d ago");
});
