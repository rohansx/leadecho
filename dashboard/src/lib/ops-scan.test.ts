import { test } from "node:test";
import assert from "node:assert/strict";
import { canRequestScan, manualScanStatus, scanPending } from "./ops-scan.ts";

const now = new Date("2026-11-12T09:30:00Z");

test("manualScanStatus says what the last manual scan is doing", () => {
  assert.equal(manualScanStatus(null, now), "");
  assert.equal(
    manualScanStatus({ kind: "hiring", state: "requested", requested_at: "2026-11-12T09:29:50Z" }, now),
    "Hiring scan requested. It starts within a minute.",
  );
  assert.equal(
    manualScanStatus({ kind: "posts", state: "running", requested_at: "2026-11-12T09:20:00Z", started_at: "2026-11-12T09:21:00Z" }, now),
    "Posts scan running for 9 min.",
  );
  assert.equal(
    manualScanStatus({ kind: "posts", state: "done", requested_at: "2026-11-12T08:00:00Z", finished_at: "2026-11-12T09:25:00Z" }, now),
    "Posts scan ran, finished 5 min ago.",
  );
  assert.equal(
    manualScanStatus({ kind: "rescore", state: "running", requested_at: "2026-11-12T09:20:00Z", started_at: "2026-11-12T09:21:00Z" }, now),
    "Re-score running for 9 min.",
  );
  assert.equal(
    manualScanStatus({ kind: "hiring", state: "done", requested_at: "2026-11-12T08:00:00Z", finished_at: "2026-11-12T06:30:00Z", note: "paid data is off for this workspace" }, now),
    "Hiring scan ended 3 h ago: paid data is off for this workspace.",
  );
});

test("scanPending is true while a scan is waiting or running", () => {
  assert.equal(scanPending([{ manual_scan: null }, { manual_scan: { kind: "posts", state: "done", requested_at: "2026-11-12T09:00:00Z" } }]), false);
  assert.equal(scanPending([{ manual_scan: null }, { manual_scan: { kind: "posts", state: "running", requested_at: "2026-11-12T09:00:00Z" } }]), true);
});

test("canRequestScan blocks a second request while one is waiting or running, unless stale", () => {
  assert.equal(canRequestScan(null, now), true);
  assert.equal(canRequestScan({ kind: "posts", state: "done", requested_at: "2026-11-12T09:00:00Z" }, now), true);
  assert.equal(canRequestScan({ kind: "posts", state: "requested", requested_at: "2026-11-12T09:29:00Z" }, now), false);
  assert.equal(canRequestScan({ kind: "posts", state: "running", requested_at: "2026-11-12T09:00:00Z", started_at: "2026-11-12T09:01:00Z" }, now), false);
  assert.equal(canRequestScan({ kind: "posts", state: "running", requested_at: "2026-11-12T07:00:00Z", started_at: "2026-11-12T07:01:00Z" }, now), true);
});
