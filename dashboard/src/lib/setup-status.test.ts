import { test } from "node:test";
import assert from "node:assert/strict";
import { keywordResult, leaveLine, scheduleTime, setupLines, type SetupStatus } from "./setup-status.ts";

const when = (iso: string) => `@${iso}`;
const texts = (s: SetupStatus) => setupLines(s, when).map((l) => l.text);

const free: SetupStatus = {
  stages: [
    { key: "posts", state: "running", posts: 40 },
    { key: "scoring", state: "running", scored: 25, unscored: 15 },
    { key: "paid_sources", state: "not_included", reason: "plan" },
    { key: "company_leads", state: "not_included", reason: "plan" },
  ],
  leads: { post: 0, company: 0 },
  has_lead: false,
  finished_empty: false,
  email_when_ready: true,
};

test("a free workspace mid-scan: counts, and what the plan leaves out", () => {
  assert.deepEqual(texts(free), [
    "Searching posts: 40 posts collected so far",
    "Scoring posts: 25 of 40 done. This usually takes a few minutes.",
    "X, LinkedIn and Reddit are not in this plan",
    "Company leads are not in this plan",
    "No leads yet",
  ]);
  assert.deepEqual(setupLines(free, when).map((l) => l.state), ["running", "running", "not_included", "not_included", "waiting"]);
});

test("a paid workspace: times come from the payload, never from the page", () => {
  const paid: SetupStatus = {
    stages: [
      { key: "posts", state: "done", posts: 1 },
      { key: "scoring", state: "done", scored: 1, unscored: 0 },
      { key: "paid_sources", state: "done", next_at: "2026-11-16T06:00:00Z" },
      { key: "company_leads", state: "done", next_at: "2026-11-16T09:00:00Z" },
    ],
    leads: { post: 1, company: 2 },
    has_lead: true,
    finished_empty: false,
    email_when_ready: false,
  };
  assert.deepEqual(texts(paid), [
    "Posts searched: 1 post collected",
    "1 post scored",
    "X, LinkedIn and Reddit searched. Next search @2026-11-16T06:00:00Z",
    "Company search ran: 2 leads found. Next one @2026-11-16T09:00:00Z",
    "Leads so far: 1 from posts, 2 from companies",
  ]);
});

test("no time known: the interval is given, or nothing", () => {
  const waiting: SetupStatus = {
    ...free,
    stages: [
      { key: "posts", state: "waiting", posts: 0, every_minutes: 5 },
      { key: "scoring", state: "waiting", scored: 0, unscored: 0 },
      { key: "paid_sources", state: "waiting", every_minutes: 5 },
      { key: "company_leads", state: "waiting", reason: "queued" },
    ],
  };
  assert.deepEqual(texts(waiting).slice(0, 4), [
    "The search starts with the next round. Rounds start every 5 minutes.",
    "Scoring starts once posts are collected",
    "X, LinkedIn and Reddit join the next search round. Rounds start every 5 minutes.",
    "Company search starts when the post search finishes",
  ]);
  const done: SetupStatus = { ...waiting, stages: [{ key: "company_leads", state: "done" }] };
  assert.equal(texts(done)[0], "Company search ran: no confirmed contact yet");
});

test("out of credits is said, not shown as still working", () => {
  const blocked: SetupStatus = {
    ...free,
    blocked: "out_of_credits",
    stages: [{ key: "scoring", state: "waiting", reason: "out_of_credits", scored: 3, unscored: 30 }],
  };
  assert.equal(texts(blocked)[0], "30 posts wait to be scored: out of credits");
  assert.match(leaveLine(blocked), /out of credits/);
});

test("the email is only promised when it will be sent", () => {
  assert.equal(leaveLine(free), "You can close this tab. We'll email you when your first leads are ready.");
  assert.doesNotMatch(leaveLine({ ...free, email_when_ready: false }), /email/);
});

test("scheduleTime: a past time is due now; another day names the day", () => {
  const now = new Date("2026-11-15T09:00:00");
  assert.equal(scheduleTime("2026-11-15T08:00:00", now), "is due now");
  assert.match(scheduleTime("2026-11-15T15:30:00", now), /^at /);
  assert.match(scheduleTime("2026-11-16T06:05:00", now), /^on \S+ at /);
});

test("keywordResult says what a keyword brought", () => {
  assert.equal(keywordResult("crm for agencies", { posts: 12, passed: 3, leads: 0 }), "crm for agencies: 12 posts, 3 past the noise filter, 0 leads");
  assert.equal(keywordResult("acme crm"), "acme crm: no posts");
});
