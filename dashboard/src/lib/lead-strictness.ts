// The workspace's lead line: how sure a post must be to show as a lead.
// The server owns the scores; this is only what the control shows.

export type LeadStrictness = "strict" | "balanced" | "loose";

export const STRICTNESS_LEVELS: { value: LeadStrictness; label: string }[] = [
  { value: "strict", label: "Fewer, surer leads" },
  { value: "balanced", label: "Balanced" },
  { value: "loose", label: "More leads" },
];

/** "Posts scoring 7.5 or more with buying intent are leads." */
export function leadLineText(minScore: number): string {
  return `Posts scoring ${Number.isInteger(minScore) ? minScore.toFixed(0) : minScore} or more with buying intent are leads.`;
}
