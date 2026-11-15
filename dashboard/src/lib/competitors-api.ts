// competitor watch: API client and the small pure helpers the page uses.
import { request } from "./api";

export type SourceKind = "x" | "linkedin" | "page_landing" | "page_pricing" | "changelog" | "blog" | "github";
export type MoveKind = "launch" | "pricing" | "positioning" | "news";

export interface CompetitorSource {
  id: string;
  kind: SourceKind;
  url: string;
  broken: boolean;
  locked: boolean;
  last_checked_at: string | null;
}

export interface CompetitorMove {
  id: string;
  kind: MoveKind;
  title: string;
  summary: string;
  impact: string;
  action: string;
  urls: string[];
  theme_id: string;
  theme_title: string;
  importance: "high" | "normal";
  at: string;
}

export interface Competitor {
  id: string;
  name: string;
  website: string;
  status: "active" | "paused";
  summary: string;
  sources: CompetitorSource[];
  moves: CompetitorMove[];
  people: number;
}

export interface CompetitorsResponse {
  data: Competitor[];
  unresolved: string[];
  suggestions: string[];
  limit: number;
  set_up: boolean;
  paid_locked: boolean;
}

export interface CompetitorPerson {
  mention_id: string;
  platform: string;
  url: string;
  author: string;
  quote: string;
  tags: string[];
  at: string;
}

export const COMPETITORS_KEY = ["competitors"];

export const getCompetitors = () => request<CompetitorsResponse>("/competitors");

export const addCompetitor = (name: string, website?: string) =>
  request<{ data: { id: string } }>("/competitors", { method: "POST", body: JSON.stringify({ name, website: website ?? "" }) });

export const findCompetitors = () => request<{ created: number; unresolved: string[] }>("/competitors/setup", { method: "POST" });

export const updateCompetitor = (id: string, patch: { website?: string; status?: "active" | "paused" }) =>
  request<{ data: { id: string } }>(`/competitors/${id}`, { method: "PATCH", body: JSON.stringify(patch) });

export const removeCompetitor = (id: string) => request<{ status: string }>(`/competitors/${id}`, { method: "DELETE" });

export const addSource = (id: string, kind: SourceKind, url: string) =>
  request<{ data: { kind: string; url: string } }>(`/competitors/${id}/sources`, { method: "POST", body: JSON.stringify({ kind, url }) });

export const removeSource = (id: string, sourceId: string) =>
  request<{ status: string }>(`/competitors/${id}/sources/${sourceId}`, { method: "DELETE" });

export const getCompetitorPeople = (id: string) => request<{ data: CompetitorPerson[] }>(`/competitors/${id}/people`);

export const SOURCE_LABEL: Record<SourceKind, string> = {
  page_landing: "Homepage",
  page_pricing: "Pricing page",
  changelog: "Changelog",
  blog: "Blog",
  github: "GitHub releases",
  x: "X",
  linkedin: "LinkedIn",
};

export const MOVE_LABEL: Record<MoveKind, string> = {
  launch: "Launch",
  pricing: "Pricing",
  positioning: "Positioning",
  news: "News",
};

export type SourceState = "broken" | "locked" | "waiting" | "ok";

/** Where a source stands, in the order a customer needs to know it. */
export function sourceState(s: CompetitorSource): SourceState {
  if (s.broken) return "broken";
  if (s.locked) return "locked";
  if (!s.last_checked_at) return "waiting";
  return "ok";
}

/** "3 moves in 90 days · 6 people unhappy · 1 source broken" */
export function competitorLine(c: Competitor): string {
  const parts = [c.moves.length === 0 ? "No moves seen yet" : `${c.moves.length} ${c.moves.length === 1 ? "move" : "moves"} in 90 days`];
  if (c.people > 0) parts.push(`${c.people} ${c.people === 1 ? "person" : "people"} unhappy with them`);
  const broken = c.sources.filter((s) => s.broken).length;
  if (broken > 0) parts.push(`${broken} ${broken === 1 ? "source" : "sources"} broken`);
  if (c.status === "paused") parts.push("paused");
  return parts.join(" · ");
}

/** Moves linked to a demand theme, across all competitors, newest first. */
export function movesForTheme(competitors: Competitor[], themeId: string): { competitor: string; move: CompetitorMove }[] {
  return competitors
    .flatMap((c) => c.moves.filter((m) => m.theme_id === themeId).map((move) => ({ competitor: c.name, move })))
    .sort((a, b) => b.move.at.localeCompare(a.move.at));
}
