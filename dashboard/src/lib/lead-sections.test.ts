import { test } from "node:test";
import assert from "node:assert/strict";
import { splitLeadSections } from "./lead-sections.ts";

test("plain post text has no sections", () => {
  assert.equal(splitLeadSections("Looking for a CRM.\n\nAny tips?"), null);
  assert.equal(splitLeadSections("People ask: Who to contact: nobody knows"), null);
  assert.equal(splitLeadSections(""), null);
  assert.equal(splitLeadSections(null), null);
});

test("a hiring signal splits on its labels", () => {
  const body =
    'Acme posted a job for "SDR" (Austin) this week.' +
    "\n\nWho to contact: Ada Lovelace, VP Sales at Acme. Their email could not be verified." +
    "\n\nWhat a web search found about the company (not verified): Sells vertical software. Moved its HQ to Austin." +
    "\n\nWhy this is a signal: A first SDR hire means outbound is starting.";
  assert.deepEqual(splitLeadSections(body), {
    intro: 'Acme posted a job for "SDR" (Austin) this week.',
    sections: [
      { label: "Who to contact", text: "Ada Lovelace, VP Sales at Acme. Their email could not be verified.", research: false },
      { label: "Web search (not verified)", text: "Sells vertical software. Moved its HQ to Austin.", research: true },
      { label: "Why this is a signal", text: "A first SDR hire means outbound is starting.", research: false },
    ],
  });
});

test("a segment prospect keeps its pain out of the research section", () => {
  const body =
    'Acme fits your customer segment "Agencies": 20 people, sells retainers.' +
    "\n\nWho to contact: Ada Lovelace, CEO at Acme." +
    "\n\nWhat a web search found about the company (not verified): Opened a second office." +
    "\n\nWhat this segment struggles with: Finding clients without cold calls.";
  assert.deepEqual(splitLeadSections(body)?.sections, [
    { label: "Who to contact", text: "Ada Lovelace, CEO at Acme.", research: false },
    { label: "Web search (not verified)", text: "Opened a second office.", research: true },
    { label: "Segment pain", text: "Finding clients without cold calls.", research: false },
  ]);
});

test("an unconfirmed signal and a trailing paragraph", () => {
  const got = splitLeadSections("Acme posted a job.\n\nNot confirmed as a lead: no contact found.\n\nMore detail.");
  assert.deepEqual(got, {
    intro: "Acme posted a job.",
    sections: [{ label: "Not confirmed", text: "no contact found.\n\nMore detail.", research: false }],
  });
});
