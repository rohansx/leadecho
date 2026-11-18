// The Keywords page's AI helpers: "generate more keywords" and the keyword
// judge's report (GET/POST /keywords/review).
import { request } from "./api";
import type { ManualScan } from "./ops-scan";

export interface SuggestedKeyword {
  term: string;
  platforms: string[];
  angle?: string;
  why: string;
}

export type SuggestionKind = "add" | "pause" | "platforms" | "keep";

export interface KeywordSuggestion {
  id: string;
  kind: SuggestionKind;
  term: string;
  platforms?: string[];
  angle?: string;
  why: string;
}

export interface KeywordReview {
  generated_at: string;
  by: string;
  days: number;
  posts: number;
  leads: number;
  summary: string;
  lead_quality: string;
  suggestions: KeywordSuggestion[];
  scoring: { post: string; note: string }[];
  decisions: Record<string, "applied" | "dismissed">;
}

export interface KeywordReviewState {
  review: KeywordReview | null;
  scan: ManualScan | null;
  active_keywords: number;
  keyword_limit: number;
  target: number;
  plan: string;
  top_up: boolean;
  can_review: boolean;
  /** The plan includes the keyword AI (Pro, or self-host). */
  allowed: boolean;
}

export const getKeywordReview = (): Promise<KeywordReviewState> => request("/keywords/review");

export const requestKeywordReview = (): Promise<ManualScan> => request("/keywords/review", { method: "POST" });

export const decideSuggestion = (id: string, decision: "applied" | "dismissed"): Promise<{ status: string }> =>
  request("/keywords/review/decision", { method: "POST", body: JSON.stringify({ id, decision }) });

export const suggestKeywords = (count: number): Promise<{ keywords: SuggestedKeyword[] }> =>
  request("/keywords/suggest", { method: "POST", body: JSON.stringify({ count }) });
