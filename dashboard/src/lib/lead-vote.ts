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
  isComposing?: boolean;
  defaultPrevented?: boolean;
  target: unknown;
}

/**
 * y = Lead, n = Not a lead. Null while typing or composing (IME), with a
 * modifier held, on key repeat, when something else handled the key, or while
 * a modal is open.
 */
export function voteKey(e: KeyLike, modalOpen = false): Vote | null {
  if (modalOpen || e.metaKey || e.ctrlKey || e.altKey || e.repeat || e.isComposing || e.defaultPrevented) return null;
  const t = e.target as { tagName?: string; isContentEditable?: boolean } | null;
  if (t?.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(t?.tagName ?? "")) return null;
  if (e.key === "y") return "up";
  if (e.key === "n") return "down";
  return null;
}

export const VOTE_TEXT: Record<Vote, string> = { up: "Lead", down: "Not a lead" };

/** A row button's accessible name, "Lead: <title>", so a screen reader can tell the rows apart. */
export function voteAriaLabel(vote: Vote, m: { title?: string | null; content?: string | null }): string {
  const what = (m.title || m.content || "").replace(/\s+/g, " ").trim();
  if (!what) return VOTE_TEXT[vote];
  return `${VOTE_TEXT[vote]}: ${what.length > 80 ? `${what.slice(0, 79)}…` : what}`;
}
