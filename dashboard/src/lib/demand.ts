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
  coverage: string; // covered | partial | gap
  status: "open" | "shipped";
  competitors: string[];
  people: number;
  buyers: number;
  new_this_week: number;
  shipped_at?: string;
  shipped_note?: string;
  updated_at: string;
  asks: DemandAsk[];
}

export interface DemandResponse {
  themes: DemandTheme[];
  last_run: string | null;
  waiting: number;
  min_new_asks: number;
  follow_up_cost: number;
  max_follow_ups: number;
}

export type CoverageTone = "good" | "warn" | "gap";

/** How the product stands on a theme, in plain words. */
export function coverageText(coverage: string): { label: string; tone: CoverageTone } {
  if (coverage === "covered") return { label: "You already do this", tone: "good" };
  if (coverage === "partial") return { label: "Partly covered", tone: "warn" };
  return { label: "Not covered yet", tone: "gap" };
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** "9 people · 4 buyers · 3 new this week" */
export function peopleLine(t: DemandTheme): string {
  return [
    plural(t.people, "person", "people"),
    t.buyers > 0 ? plural(t.buyers, "buyer", "buyers") : "",
    t.new_this_week > 0 ? `${t.new_this_week} new this week` : "",
  ]
    .filter(Boolean)
    .join(" · ");
}

/** How many follow-ups shipping a theme would draft, and what they cost. */
export function followUpEstimate(t: DemandTheme, maxFollowUps: number, costEach: number): { drafts: number; credits: number } {
  const drafts = Math.min(t.people, maxFollowUps);
  return { drafts, credits: drafts * costEach };
}
