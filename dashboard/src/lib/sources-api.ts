// sources: mutes, CSV export URLs, keyword cadence + cost preview.
import { request } from "./api";

export type MuteKind = "author" | "domain" | "keyword" | "subreddit";

export interface Mute {
  id: string;
  kind: MuteKind;
  value: string;
  created_at: string;
}

export function listMutes() {
  return request<Mute[]>("/mutes");
}

export function createMute(kind: MuteKind, value: string) {
  return request<Mute>("/mutes", { method: "POST", body: JSON.stringify({ kind, value }) });
}

export function deleteMute(id: string) {
  return request<{ status: string }>(`/mutes/${id}`, { method: "DELETE" });
}

/** Same-origin download URL; the session cookie authenticates it. */
export function exportUrl(kind: "mentions" | "leads", params: Record<string, string | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
  const qs = q.toString();
  return `/api/v1/${kind}/export.csv${qs ? `?${qs}` : ""}`;
}

export type CadenceMap = Record<string, string>;

export interface PlatformCost {
  platform: string;
  cadence: string;
  calls_per_month: number;
  usd_per_month: number;
}

export interface KeywordCost {
  keyword_id: string;
  term: string;
  is_active: boolean;
  cadence: CadenceMap;
  platforms: PlatformCost[];
  usd_per_month: number;
}

export interface CostPreview {
  keywords: KeywordCost[];
  usd_per_month: number;
  rates: Record<string, number>;
  options: string[];
  default: string;
}

export function getCostPreview() {
  return request<CostPreview>("/keywords/cost-preview");
}

export function updateKeywordCadence(id: string, cadence: CadenceMap) {
  return request<{ id: string; cadence: CadenceMap }>(`/keywords/${id}/cadence`, {
    method: "PUT",
    body: JSON.stringify({ cadence }),
  });
}

export const COST_PREVIEW_KEY = "keyword-cost-preview";
export const MUTES_KEY = "mutes";

export function formatUSD(v: number): string {
  return v === 0 ? "free" : `$${v < 1 ? v.toFixed(2) : v.toFixed(0)}/mo`;
}
