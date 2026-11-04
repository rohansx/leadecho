import { test } from "node:test";
import assert from "node:assert/strict";
import { describeCall } from "./describe-call.ts";

test("queue_reply shows the reply text that will be posted", () => {
  const d = describeCall("queue_reply", { reply_id: "r1", content: "Try LeadEcho, link below" });
  assert.equal(d.title, "Queue a reply for posting");
  assert.deepEqual(d.lines, ["Text: “Try LeadEcho, link below”"]);
});

test("set_stage and push_to_crm name the lead and target", () => {
  assert.deepEqual(describeCall("set_stage", { lead_id: "l1", stage: "qualified" }).lines, ["Move lead to stage: qualified", "Lead: l1"]);
  assert.equal(describeCall("push_to_crm", { lead_id: "l1" }).title, "Push this lead to your CRM");
});

test("mute and listener edits are readable", () => {
  assert.deepEqual(describeCall("mute", { kind: "author", value: "spammer" }).lines, ["Mute author: spammer"]);
  const t = describeCall("tune_listener", { listener_id: "p", add_keywords: ["a", "b"], skips: "vendors" });
  assert.deepEqual(t.lines, ["Listener: p", "Add keywords: a, b", "Skips: “vendors”"]);
});

test("unknown tools fall back to key: value lines, long text truncated", () => {
  const d = describeCall("custom_tool", { note: "x".repeat(400) });
  assert.equal(d.title, "Run custom tool");
  assert.ok(d.lines[0].length < 320);
});

test("queue_reply never hides the posted text", () => {
  const long = "y".repeat(5000);
  assert.ok(describeCall("queue_reply", { reply_id: "r1", content: long }).lines[0].includes(long));
  assert.match(describeCall("queue_reply", { reply_id: "r9" }).lines[0], /r9.*inbox/);
  assert.equal(describeCall("create_listener", {}).title, "Create a listener");
});
