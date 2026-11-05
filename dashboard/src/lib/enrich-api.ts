// enrich (P2 #16): LinkedIn engagers + tracked profiles API. Kept out of
// api.ts so parallel feature branches merge cleanly.
import { request } from "./api";

export type EngagerKind = "comments" | "reactions";

export interface EngagerRow {
  name: string;
  headline?: string;
  profile_url?: string;
  public_id?: string;
  kind: "comment" | "reaction";
  comment?: string;
  reaction?: string;
  title?: string;
  company?: string;
  lead_id: string;
  mention_id: string;
  icp_fit_score: number | null;
}

export interface TrackedProfile {
  id: string;
  workspace_id: string;
  platform: "linkedin" | "x";
  handle: string;
  url: string;
  last_polled_at: string | null;
  created_at: string;
}

export function pullEngagers(body: { post_url: string; kinds: EngagerKind[]; enrich: boolean }) {
  return request<{ data: EngagerRow[]; total: number; truncated?: boolean }>("/linkedin/engagers", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function pushEngagersToCRM(leadIDs: string[]) {
  return request<{ queued: number }>("/linkedin/engagers/push", {
    method: "POST",
    body: JSON.stringify({ lead_ids: leadIDs }),
  });
}

export function listTrackedProfiles() {
  return request<{ data: TrackedProfile[] }>("/tracked-profiles");
}

export function createTrackedProfile(body: { platform: "linkedin" | "x"; handle: string }) {
  return request<TrackedProfile>("/tracked-profiles", { method: "POST", body: JSON.stringify(body) });
}

export async function deleteTrackedProfile(id: string): Promise<void> {
  const res = await fetch(`/api/v1/tracked-profiles/${id}`, { method: "DELETE" });
  if (!res.ok) throw new Error(`Delete failed (${res.status})`);
}

function csvCell(v: unknown): string {
  const s = v == null ? "" : String(v);
  // Neutralise spreadsheet formulas, then quote.
  const safe = /^[=+\-@\t\r]/.test(s) ? `'${s}` : s;
  return `"${safe.replace(/"/g, '""')}"`;
}

export function engagersToCSV(rows: EngagerRow[]): string {
  const cols: (keyof EngagerRow)[] = ["name", "title", "company", "headline", "kind", "reaction", "comment", "profile_url", "icp_fit_score", "lead_id"];
  return [cols.join(","), ...rows.map((r) => cols.map((c) => csvCell(r[c])).join(","))].join("\n");
}
