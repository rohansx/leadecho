import { test } from "node:test";
import assert from "node:assert/strict";
import { hiringText, leadAge } from "./lead-age.ts";

const now = new Date("2026-11-15T12:00:00Z");
const posted = (iso: string | null) => ({ platform_created_at: iso, created_at: "2026-11-14T06:00:00Z" });

test("leadAge counts whole days since the post, or since it was found", () => {
  assert.equal(leadAge(posted("2026-11-06T09:00:00Z"), now), "posted 9 days ago");
  assert.equal(leadAge(posted("2026-11-14T09:00:00Z"), now), "posted yesterday");
  assert.equal(leadAge(posted("2026-11-15T09:00:00Z"), now), "posted today");
  assert.equal(leadAge(posted("2026-11-16T09:00:00Z"), now), "posted today"); // a clock ahead of ours
  assert.equal(leadAge(posted(null), now), "found yesterday");
});

test("hiringText replaces the fixed 'this week' with the posting's real age", () => {
  const old = posted("2026-11-06T09:00:00Z");
  assert.equal(
    hiringText('Acme posted a job for "Software Engineer" (Austin, TX) this week.\n\nWho to contact: Ann, CTO at Acme.', old, now),
    'Acme posted a job for "Software Engineer" (Austin, TX) 9 days ago.\n\nWho to contact: Ann, CTO at Acme.',
  );
  assert.equal(
    hiringText("Company posted a matching job this week, but the contact is not confirmed: no email.", old, now),
    "Company posted a matching job 9 days ago, but the contact is not confirmed: no email.",
  );
  assert.equal(
    hiringText("Company posted a matching job this week. They screen engineers.", posted("2026-11-15T09:00:00Z"), now),
    "Company posted a matching job today. They screen engineers.",
  );
  // No posting date: say nothing rather than guess.
  assert.equal(hiringText("Acme posted a job for \"Dev\" this week.", posted(null), now), 'Acme posted a job for "Dev".');
  // Only the opening sentence is touched.
  const other = "We ship this week.\n\nAcme posted a job this week.";
  assert.equal(hiringText(other, old, now), other);
});
