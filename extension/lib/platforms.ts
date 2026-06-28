/**
 * Single source of truth for platform display + capability metadata shared
 * across the side panel tabs.
 */

export const PLATFORM_LABELS: Record<string, string> = {
  reddit: "Reddit",
  twitter: "Twitter",
  linkedin: "LinkedIn",
  hackernews: "HN",
  devto: "Dev.to",
  lobsters: "Lobsters",
  indiehackers: "IH",
  quora: "Quora",
};

/**
 * Platforms the extension can auto-post replies to (a content script with an
 * injection handler + matching host permission exists). Everything else must
 * be posted manually by the user.
 */
export const POSTABLE_PLATFORMS = new Set<string>([
  "reddit",
  "twitter",
  "linkedin",
  "hackernews",
]);

export function platformLabel(platform: string): string {
  return PLATFORM_LABELS[platform] ?? platform;
}

export function canAutoPost(platform: string): boolean {
  return POSTABLE_PLATFORMS.has(platform);
}
