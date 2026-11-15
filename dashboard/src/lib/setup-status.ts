// What a workspace without a lead is waiting for (GET /onboarding/setup-status),
// and the checklist wording. Every number and time comes from the payload.

export type StageState = "done" | "running" | "waiting" | "not_included";

export interface SetupStage {
  key: "posts" | "scoring" | "paid_sources" | "company_leads";
  state: StageState;
  /** Why it waits or is not included. */
  reason?: string;
  posts?: number;
  scored?: number;
  unscored?: number;
  /** A time the schedule gives (ISO). */
  next_at?: string;
  /** No time can be known: it starts with the next search round, and rounds start this often. */
  every_minutes?: number;
}

export interface SetupStatus {
  stages: SetupStage[];
  leads: { post: number; company: number };
  has_lead: boolean;
  finished_empty: boolean;
  blocked?: "paused" | "out_of_credits";
  email_when_ready: boolean;
}

export interface SetupLine {
  key: string;
  state: StageState;
  text: string;
}

const PAID = "X, LinkedIn and Reddit";
const n = (count: number, one: string, many: string) => `${count.toLocaleString("en-US")} ${count === 1 ? one : many}`;

const stopped = (reason?: string) =>
  reason === "out_of_credits" ? "out of credits" : reason === "paused" ? "scanning is paused for this workspace" : "";

/** "Rounds start every 5 minutes." when the payload gives the interval. */
const rounds = (s: SetupStage) => (s.every_minutes ? ` Rounds start every ${s.every_minutes} minutes.` : "");

function postsLine(s: SetupStage): string {
  const found = n(s.posts ?? 0, "post", "posts");
  if (s.state === "done") return `Posts searched: ${found} collected`;
  if (s.state === "running") return `Searching posts: ${found} collected so far`;
  if (s.reason === "no_keywords") return "No active keywords to search with";
  if (s.reason === "queued") return "The search is queued and starts next";
  if (stopped(s.reason)) return `The search is not running: ${stopped(s.reason)}`;
  return `The search starts with the next round.${rounds(s)}`;
}

function scoringLine(s: SetupStage): string {
  const scored = s.scored ?? 0;
  const total = scored + (s.unscored ?? 0);
  if (s.state === "done") return `${n(scored, "post", "posts")} scored`;
  if (s.state === "running") return `Scoring posts: ${scored.toLocaleString("en-US")} of ${total.toLocaleString("en-US")} done. This usually takes a few minutes.`;
  if (stopped(s.reason)) return `${n(s.unscored ?? 0, "post waits", "posts wait")} to be scored: ${stopped(s.reason)}`;
  return "Scoring starts once posts are collected";
}

function paidLine(s: SetupStage, when: (iso: string) => string): string {
  if (s.state === "not_included") {
    return s.reason === "plan" ? `${PAID} are not in this plan` : `${PAID}: none of your keywords search them`;
  }
  if (s.state === "done") return `${PAID} searched${s.next_at ? `. Next search ${when(s.next_at)}` : ""}`;
  if (s.state === "running") return `Searching ${PAID}`;
  if (s.reason === "queued") return `${PAID} are searched with the queued search`;
  if (stopped(s.reason)) return `${PAID} are not searched: ${stopped(s.reason)}`;
  return `${PAID} join the next search round.${rounds(s)}`;
}

function companyLine(s: SetupStage, found: number, when: (iso: string) => string): string {
  if (s.state === "not_included") {
    if (s.reason === "plan") return "Company leads are not in this plan";
    if (s.reason === "no_fit") return "Company leads: no hiring signal fits this product";
    return "Company leads are not set up on this server";
  }
  if (s.state === "done") {
    const result = found > 0 ? `${n(found, "lead", "leads")} found` : "no confirmed contact yet";
    return `Company search ran: ${result}${s.next_at ? `. Next one ${when(s.next_at)}` : ""}`;
  }
  if (s.state === "running") return "Looking for companies with a matching job posting or funding round";
  if (s.reason === "queued") return "Company search starts when the post search finishes";
  if (s.reason === "paused") return "Company search is paused";
  if (stopped(s.reason)) return `Company search is not running: ${stopped(s.reason)}`;
  return `Company search starts with the next search round.${rounds(s)}`;
}

/** The time as the schedule gives it; "is due now" once it has passed. */
export function scheduleTime(iso: string, now = new Date()): string {
  const t = new Date(iso);
  if (t.getTime() <= now.getTime()) return "is due now";
  const sameDay = t.toDateString() === now.toDateString();
  const time = t.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  return sameDay ? `at ${time}` : `on ${t.toLocaleDateString(undefined, { weekday: "long" })} at ${time}`;
}

/** The checklist, one line per stage, then the leads found so far. */
export function setupLines(status: SetupStatus, when: (iso: string) => string = scheduleTime): SetupLine[] {
  const lines = status.stages.map((s): SetupLine => {
    const text =
      s.key === "posts" ? postsLine(s)
      : s.key === "scoring" ? scoringLine(s)
      : s.key === "paid_sources" ? paidLine(s, when)
      : companyLine(s, status.leads.company, when);
    return { key: s.key, state: s.state, text };
  });
  const { post, company } = status.leads;
  const hasCompany = status.stages.some((s) => s.key === "company_leads" && s.state !== "not_included");
  const leads = !status.has_lead
    ? "No leads yet"
    : hasCompany
      ? `Leads so far: ${post.toLocaleString("en-US")} from posts, ${company.toLocaleString("en-US")} from companies`
      : `Leads so far: ${post.toLocaleString("en-US")}`;
  return [...lines, { key: "leads", state: status.has_lead ? "done" : "waiting", text: leads }];
}

/** What to tell someone who is about to leave the page. */
export function leaveLine(status: SetupStatus): string {
  if (status.blocked === "out_of_credits") return "This workspace is out of credits: nothing more is searched or scored until credits are added.";
  if (status.blocked === "paused") return "Scanning is paused for this workspace.";
  return status.email_when_ready
    ? "You can close this tab. We'll email you when your first leads are ready."
    : "You can close this tab and come back later: leads appear in your inbox as they are found.";
}

/** "crm for agencies: 12 posts, 3 past the noise filter, 0 leads". */
export function keywordResult(term: string, y?: { posts: number; passed: number; leads: number }): string {
  if (!y) return `${term}: no posts`;
  return `${term}: ${n(y.posts, "post", "posts")}, ${y.passed.toLocaleString("en-US")} past the noise filter, ${n(y.leads, "lead", "leads")}`;
}
