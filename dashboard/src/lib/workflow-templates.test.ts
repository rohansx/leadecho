import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ACTION_INFO,
  INTENT_LABELS,
  TEMPLATES,
  describeActions,
  describeTrigger,
  splitList,
  validateDefinition,
} from "./workflow-templates.ts";

test("describeTrigger reads like a sentence", () => {
  assert.equal(
    describeTrigger({ min_score: 7, platforms: ["reddit"], intent_tags: ["buying_intent", "seeking_alternative"], keywords: ["crm"] }),
    'Score 7+ · reddit · Ready to buy or Looking to switch · mentions "crm"',
  );
  assert.equal(describeTrigger({ min_score: 0 }), "Any score");
});

test("describeActions joins labels in order", () => {
  assert.equal(describeActions([{ type: "notify_slack" }, { type: "ai_draft" }]), "Send to Slack → Draft an AI reply");
});

test("validateDefinition", () => {
  const base = { name: "x", enabled: true, trigger: { min_score: 6 } };
  assert.equal(validateDefinition({ ...base, actions: [{ type: "ai_draft" }] }), null);
  assert.match(validateDefinition({ ...base, name: " ", actions: [{ type: "ai_draft" }] }) ?? "", /name/);
  assert.match(validateDefinition({ ...base, actions: [] }) ?? "", /action/);
  assert.match(validateDefinition({ ...base, actions: [{ type: "archive" }, { type: "ai_draft" }] }) ?? "", /Archive/);
  assert.match(validateDefinition({ ...base, actions: [{ type: "webhook", config: { url: "http://x" } }] }) ?? "", /https/);
});

test("templates only use known intents and actions, and are valid once filled", () => {
  for (const t of TEMPLATES) {
    for (const tag of t.def.trigger.intent_tags ?? []) assert.ok(INTENT_LABELS[tag], `${t.id}: ${tag}`);
    for (const a of t.def.actions) assert.ok(ACTION_INFO[a.type], `${t.id}: ${a.type}`);
    const filled = { ...t.def, actions: t.def.actions.map((a) => (a.type === "webhook" ? { ...a, config: { url: "https://hooks.example.com/x" } } : a)) };
    assert.equal(validateDefinition(filled), null, t.id);
  }
});

test("splitList trims and drops blanks", () => {
  assert.deepEqual(splitList(" a, b ,, c "), ["a", "b", "c"]);
});
