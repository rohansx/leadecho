// demand: what the market is asking for, grouped into themes. Types and
// pure helpers only (the API calls are in demand-api.ts).

export interface DemandAsk {
  mention_id: string;
  quote: string;
  buyer: boolean;
  /** Affected by the problem but would not buy (a candidate, for a hiring product). */
  non_buyer?: boolean;
  platform: string;
  url: string;
  author?: string;
  at: string;
}

export interface DemandTheme {
  id: string;
  title: string;
  summary: string;
  coverage: string; // covered | partial | gap | outside | objection
  why?: string; // the reason for the coverage label
  status: "open" | "shipped";
  competitors: string[];
  people: number;
  buyers: number;
  /** Affected by the problem but would not buy. The rest (people - buyers - non_buyers) are unknown. */
  non_buyers?: number;
  new_this_week: number;
  shipped_at?: string;
  shipped_note?: string;
  drafted: number;
  updated_at: string;
  asks: DemandAsk[];
}

export interface DemandResponse {
  /** Themes several people raised, most buyers first, then what shipped. */
  themes: DemandTheme[];
  /** Needs one person raised each, as one group. Null when there are none. */
  other?: DemandTheme | null;
  last_run: string | null;
  waiting: number;
  min_new_asks: number;
  min_theme_people: number;
  follow_up_cost: number;
  max_follow_ups: number;
  /** Product changes the themes point to. Null until first worked out. */
  improvements?: DemandImprovements | null;
}

export interface DemandEvidence {
  mention_id: string;
  quote: string;
  url?: string;
  platform?: string;
  author?: string;
  buyer?: boolean;
}

export interface DemandImprovement {
  title: string;
  change: string;
  why: string;
  for_whom: string;
  kind: "feature" | "pricing" | "positioning" | "onboarding";
  themes: string[];
  evidence: DemandEvidence[];
}

export interface DemandImprovements {
  items: DemandImprovement[];
  /** Unset until a run succeeded. */
  generated_at?: string;
}

export type CoverageTone = "good" | "warn" | "gap" | "muted";

/** How the product stands on a theme, in plain words. */
export function coverageText(coverage: string): { label: string; tone: CoverageTone } {
  if (coverage === "covered") return { label: "You already do this", tone: "good" };
  if (coverage === "partial") return { label: "Partly covered", tone: "warn" };
  if (coverage === "outside") return { label: "Outside your product", tone: "muted" };
  // People arguing against how the product works: not demand it already meets.
  if (coverage === "objection") return { label: "Objection to your approach", tone: "warn" };
  return { label: "Not covered yet", tone: "gap" };
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** "9 people · 4 fit your buyer profile · 3 not buyers · 3 new this week". "New" is left out when every ask is new. */
export function peopleLine(t: DemandTheme): string {
  return [
    plural(t.people, "person", "people"),
    // "Buyer" is a role match, not a sign they are shopping: say so.
    t.buyers > 0 ? `${t.buyers} ${t.buyers === 1 ? "fits" : "fit"} your buyer profile` : "",
    (t.non_buyers ?? 0) > 0 ? plural(t.non_buyers ?? 0, "not a buyer", "not buyers") : "",
    t.new_this_week > 0 && t.new_this_week < t.people ? `${t.new_this_week} new this week` : "",
  ]
    .filter(Boolean)
    .join(" · ");
}

/** How many follow-ups shipping a theme would draft at most (buyers only), and what they cost. */
export function followUpEstimate(t: DemandTheme, maxFollowUps: number, costEach: number): { drafts: number; credits: number } {
  const drafts = Math.min(t.buyers, maxFollowUps);
  return { drafts, credits: drafts * costEach };
}

/**
 * Open themes, needs for a different kind of product, and what shipped. The
 * order the server sent (most buyers first) is kept. Needs raised once are
 * not here: the server folds them into `other`.
 */
export function splitThemes(themes: DemandTheme[]): { open: DemandTheme[]; outside: DemandTheme[]; shipped: DemandTheme[] } {
  const open = themes.filter((t) => t.status === "open");
  return {
    outside: open.filter((t) => t.coverage === "outside"),
    open: open.filter((t) => t.coverage !== "outside"),
    shipped: themes.filter((t) => t.status === "shipped"),
  };
}

/** Where an ask came from. The text shown is LeadEcho's summary, not a quote. */
export function askSource(a: Pick<DemandAsk, "buyer" | "non_buyer" | "platform">): string {
  if (a.buyer) return `Summary of a post by a buyer on ${a.platform}`;
  return `Summary of a post on ${a.platform}, ${askRole(a)}`;
}

/** Who wrote the post, as far as the scorer could tell. */
export function askRole(a: Pick<DemandAsk, "buyer" | "non_buyer">): string {
  if (a.buyer) return "fits your buyer profile";
  return a.non_buyer ? "not a buyer" : "role unknown";
}
