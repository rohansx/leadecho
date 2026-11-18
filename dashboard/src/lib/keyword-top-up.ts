// Pure helper for the "generate more keywords" offer (no imports, so it is unit-testable).

/** How many keywords to offer: enough to reach the target, within the plan limit, at least 5. */
export function topUpCount(s: { active_keywords: number; keyword_limit: number; target: number }): number {
  const room = Math.max(0, s.keyword_limit - s.active_keywords);
  const want = Math.max(5, s.target - s.active_keywords);
  return Math.min(room, want, 40);
}
