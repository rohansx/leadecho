// "Lead / Not a lead": the customer's vote on a mention, and the keys for it.

export type Vote = "up" | "down";

// Why the customer says a lead is wrong; sent as the vote's note and stored
// with the archived mention.
export const NOT_A_FIT_REASONS = ["Wrong company", "Not a buyer", "Too old", "Other"];
export const notAFitNote = (reason: string): string => `Not a fit: ${reason.toLowerCase()}`;

/** The mention's latest vote. An archive or spam flag overwrites it, so those read as no vote. */
export function lastVote(m: { scoring_metadata?: Record<string, unknown> | null }): Vote | null {
  const last = (m.scoring_metadata?.last_feedback ?? null) as { label?: string } | null;
  return last?.label === "up" || last?.label === "down" ? last.label : null;
}

interface KeyLike {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  repeat?: boolean;
  target: unknown;
}

/** y = Lead, n = Not a lead; null while typing, with a modifier held, or on key repeat. */
export function voteKey(e: KeyLike): Vote | null {
  if (e.metaKey || e.ctrlKey || e.altKey || e.repeat) return null;
  const t = e.target as { tagName?: string; isContentEditable?: boolean } | null;
  if (t?.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(t?.tagName ?? "")) return null;
  if (e.key === "y") return "up";
  if (e.key === "n") return "down";
  return null;
}
