// demand: what the market is asking for, grouped into themes. Types and
// pure helpers only (the API calls are in demand-api.ts).

export interface DemandAsk {
  mention_id: string;
  quote: string;
  buyer: boolean;
  platform: string;
  url: string;
  author?: string;
  at: string;
}

export interface DemandTheme {
  id: string;
  title: string;
  summary: string;
  coverage: string; // covered | partial | gap | outside
  why?: string; // the reason for the coverage label
  status: "open" | "shipped";
  competitors: string[];
  people: number;
  buyers: number;
  new_this_week: number;
  shipped_at?: string;
  shipped_note?: string;
  drafted: number;
  updated_at: string;
  asks: DemandAsk[];
}

export interface DemandResponse {
  themes: DemandTheme[];
  last_run: string | null;
  waiting: number;
  min_new_asks: number;
  min_theme_people: number;
  follow_up_cost: number;
  max_follow_ups: number;
}

export type CoverageTone = "good" | "warn" | "gap" | "muted";

/** How the product stands on a theme, in plain words. */
export function coverageText(coverage: string): { label: string; tone: CoverageTone } {
  if (coverage === "covered") return { label: "You already do this", tone: "good" };
  if (coverage === "partial") return { label: "Partly covered", tone: "warn" };
  if (coverage === "outside") return { label: "Outside your product", tone: "muted" };
  return { label: "Not covered yet", tone: "gap" };
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** "9 people · 4 buyers · 3 new this week". "New" is left out when every ask is new. */
export function peopleLine(t: DemandTheme): string {
  return [
    plural(t.people, "person", "people"),
    t.buyers > 0 ? plural(t.buyers, "buyer", "buyers") : "",
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

/** Open themes several people raised, needs raised once, and what shipped. */
export function splitThemes(themes: DemandTheme[], minPeople: number): { open: DemandTheme[]; once: DemandTheme[]; shipped: DemandTheme[] } {
  const live = themes.filter((t) => t.status === "open");
  return {
    open: live.filter((t) => t.people >= minPeople),
    once: live.filter((t) => t.people < minPeople),
    shipped: themes.filter((t) => t.status === "shipped"),
  };
}

/** Where an ask came from. The text shown is LeadEcho's summary, not a quote. */
export function askSource(a: Pick<DemandAsk, "buyer" | "platform">): string {
  return a.buyer ? `Summary of a post by a buyer on ${a.platform}` : `Summary of a post on ${a.platform}, role unknown`;
}
