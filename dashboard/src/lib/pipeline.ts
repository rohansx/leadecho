// Pure helpers for the Leads page. No React/alias imports so `node --test` can load it.
import type { Lead, LeadStage } from "./types.ts";

/** User-facing stage names mapped onto the unchanged lead_stage DB enum. */
export const STAGES: readonly { key: LeadStage; label: string }[] = [
  { key: "prospect", label: "To reply" },
  { key: "qualified", label: "Replied" },
  { key: "engaged", label: "In conversation" },
  { key: "converted", label: "Won" },
  { key: "lost", label: "Not a fit" },
];

export function stageLabel(stage: string): string {
  return STAGES.find((s) => s.key === stage)?.label ?? stage;
}

const INTENT_REASON: Record<string, string> = {
  recommendation_ask: "Asking for recommendations",
  buy_signal: "Looking to buy",
  complaint: "Unhappy with their current tool",
  comparison: "Comparing options",
  general: "Talking about your space",
};

/** Plain-words reason this person is a lead. */
export function intentReason(intent: string | null | undefined): string {
  return (intent && INTENT_REASON[intent]) || "Matched your keywords";
}

function metaValue(lead: Lead, key: string): unknown {
  return lead.metadata?.[key];
}

/** Intent from the linked post, falling back to what the scorer stored on the lead. */
export function leadIntent(lead: Lead): string | null {
  const meta = metaValue(lead, "intent");
  return lead.mention?.intent ?? (typeof meta === "string" ? meta : null);
}

export function leadScore(lead: Lead): number | null {
  const meta = metaValue(lead, "relevance");
  return lead.mention?.score ?? (typeof meta === "number" ? meta : null);
}

/** When they posted (falls back to when we found them). */
export function leadPostedAt(lead: Lead): string {
  return lead.mention?.posted_at ?? lead.created_at;
}

/** Best first: score desc (unscored last), then most recent. Returns a new array. */
export function sortLeads(leads: readonly Lead[]): Lead[] {
  return [...leads].sort((a, b) => {
    const sa = leadScore(a) ?? -1;
    const sb = leadScore(b) ?? -1;
    if (sa !== sb) return sb - sa;
    return Date.parse(leadPostedAt(b)) - Date.parse(leadPostedAt(a));
  });
}

export function timeAgo(iso: string, now: number = Date.now()): string {
  const mins = Math.max(0, Math.floor((now - Date.parse(iso)) / 60000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return `${Math.floor(days / 30)}mo ago`;
}
