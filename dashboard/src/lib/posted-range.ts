// posted-range: the "Posted in the last…" filter. The URL keeps the range
// ("7d"), not a timestamp, so a shared or bookmarked link stays relative.

const RANGE_MS = {
  "24h": 24 * 3600_000,
  "7d": 7 * 86400_000,
  "30d": 30 * 86400_000,
  "90d": 90 * 86400_000,
} as const;

export type PostedRange = keyof typeof RANGE_MS;

export const POSTED_RANGES: { value: PostedRange; label: string }[] = [
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
  { value: "30d", label: "Last 30 days" },
  { value: "90d", label: "Last 90 days" },
];

export function isPostedRange(v: unknown): v is PostedRange {
  return typeof v === "string" && Object.hasOwn(RANGE_MS, v);
}

/** The RFC 3339 start of the window, or undefined for no date filter. */
export function postedSince(range: string | undefined, now: Date = new Date()): string | undefined {
  if (!isPostedRange(range)) return undefined;
  return new Date(now.getTime() - RANGE_MS[range]).toISOString();
}
