// Pure helpers for the Keywords page: effective scan cadence, cost, one-line
// schedule summary and the "paid sources every 24h" bulk plan. Mirrors
// backend/internal/cadence (Default=1h, PaidDefault=24h for priced platforms).

export type CadenceMap = Record<string, string>;
export type Rates = Record<string, number>;

export const PAID_DEFAULT = "24h";
const FREE_DEFAULT = "1h";
const HOURS: Record<string, number> = { "15m": 0.25, "30m": 0.5, "1h": 1, "3h": 3, "4h": 4, "6h": 6, "12h": 12, "24h": 24 };
const MONTH_HOURS = 30 * 24;

export const isPaid = (platform: string, rates: Rates): boolean => (rates[platform] ?? 0) > 0;

/** Cadence the scheduler actually uses: stored value, else 24h for paid / 1h for free. */
export function effectiveCadence(platform: string, stored: CadenceMap, rates: Rates): string {
  const v = stored[platform];
  if (v && HOURS[v]) return v;
  return isPaid(platform, rates) ? PAID_DEFAULT : FREE_DEFAULT;
}

export function platformCost(platform: string, cadence: string, rates: Rates): number {
  const calls = Math.floor(MONTH_HOURS / (HOURS[cadence] ?? 1));
  return Math.round(calls * (rates[platform] ?? 0) * 1e4) / 1e4;
}

export function keywordCost(platforms: string[], stored: CadenceMap, rates: Rates): number {
  return platforms.reduce((s, p) => s + platformCost(p, effectiveCadence(p, stored, rates), rates), 0);
}

const byHours = (a: string, b: string) => (HOURS[a] ?? 0) - (HOURS[b] ?? 0);

function groupLabel(name: string, platforms: string[], stored: CadenceMap, rates: Rates): string | null {
  if (!platforms.length) return null;
  const set = [...new Set(platforms.map((p) => effectiveCadence(p, stored, rates)))].sort(byHours);
  return `${name} every ${set.join("/")}`;
}

/** e.g. "Paid every 24h · free every 1h". Cost is appended by the caller. */
export function scheduleSummary(platforms: string[], stored: CadenceMap, rates: Rates): string {
  const paid = platforms.filter((p) => isPaid(p, rates));
  const free = platforms.filter((p) => !isPaid(p, rates));
  const parts = [groupLabel("paid", paid, stored, rates), groupLabel("free", free, stored, rates)].filter(Boolean) as string[];
  const s = parts.join(" · ");
  return s ? s[0].toUpperCase() + s.slice(1) : "No platforms";
}

export interface BulkKeyword {
  id: string;
  platforms: string[];
  cadence: CadenceMap;
  isActive: boolean;
}

export interface BulkPlan {
  updates: { id: string; cadence: CadenceMap }[];
  /** Monthly USD saved across active keywords. */
  saving: number;
}

/** Slow every paid platform scanned more often than `target` down to it. Never speeds anything up. */
export function paidSlowdownPlan(keywords: BulkKeyword[], rates: Rates, target = PAID_DEFAULT): BulkPlan {
  const plan: BulkPlan = { updates: [], saving: 0 };
  for (const k of keywords) {
    const fast = k.platforms.filter(
      (p) => isPaid(p, rates) && (HOURS[effectiveCadence(p, k.cadence, rates)] ?? 0) < HOURS[target],
    );
    if (!fast.length) continue;
    const next = { ...k.cadence, ...Object.fromEntries(fast.map((p) => [p, target])) };
    plan.updates.push({ id: k.id, cadence: next });
    if (k.isActive) plan.saving += keywordCost(k.platforms, k.cadence, rates) - keywordCost(k.platforms, next, rates);
  }
  plan.saving = Math.round(plan.saving * 100) / 100;
  return plan;
}

/** First `n` items plus how many are hidden. */
export function truncate<T>(items: T[], n: number, expanded: boolean): { shown: T[]; hidden: number } {
  if (expanded || items.length <= n) return { shown: items, hidden: 0 };
  return { shown: items.slice(0, n), hidden: items.length - n };
}
