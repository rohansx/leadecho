import { test } from "node:test";
import assert from "node:assert/strict";
import { companyMismatch, mailtoHref, splitSubject } from "./outreach.ts";

test("splitSubject", () => {
  assert.deepEqual(splitSubject("Subject: Your Sr Full Stack opening\n\nHi Marty,\n\nSaw the role."), {
    subject: "Your Sr Full Stack opening",
    body: "Hi Marty,\n\nSaw the role.",
  });
  assert.deepEqual(splitSubject("Hi Marty, no subject here."), { subject: "", body: "Hi Marty, no subject here." });
});

test("mailtoHref encodes subject and body", () => {
  const href = mailtoHref("marty@example.com", "Subject: A & B?\n\nLine 1\nLine 2");
  assert.equal(href, "mailto:marty%40example.com?subject=A%20%26%20B%3F&body=Line%201%0ALine%202");
  assert.equal(mailtoHref("a@b.co", "just body"), "mailto:a%40b.co?body=just%20body");
});

test("companyMismatch", () => {
  assert.equal(companyMismatch("Stock: The Logistics of Inventory Donation", "The Conversation"), true);
  assert.equal(companyMismatch("LVT (LiveView Technologies)", "LVT"), false);
  assert.equal(companyMismatch("Vista Higher Learning", "vista higher learning, inc."), false);
  assert.equal(companyMismatch("Acme", ""), false);
  assert.equal(companyMismatch(undefined, "Acme"), false);
});
