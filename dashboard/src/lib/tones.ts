// Single source of truth for categorical colors (platforms, lead stages, intents).
// Tinted backgrounds + theme-aware text so chips read in both light and dark mode.

const TONE = {
  neutral: "bg-muted text-foreground-soft",
  blue: "bg-blue-500/10 text-blue-700 dark:text-blue-300",
  violet: "bg-violet-500/10 text-violet-700 dark:text-violet-300",
  emerald: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  red: "bg-red-500/10 text-red-700 dark:text-red-300",
} as const;

// Platforms are a category, not a state: a faint tint and near-neutral text.
export const PLATFORM_TONE: Record<string, string> = {
  reddit: "bg-orange-500/8 text-orange-900/80 dark:text-orange-200/80",
  hackernews: "bg-amber-500/8 text-amber-900/80 dark:text-amber-200/80",
  twitter: "bg-sky-500/8 text-sky-900/80 dark:text-sky-200/80",
  linkedin: "bg-blue-500/8 text-blue-900/80 dark:text-blue-200/80",
  quora: "bg-violet-500/8 text-violet-900/80 dark:text-violet-200/80",
  exa: "bg-emerald-500/8 text-emerald-900/80 dark:text-emerald-200/80",
};

export const STAGE_TONE: Record<string, string> = {
  prospect: TONE.neutral,
  qualified: TONE.blue,
  engaged: TONE.violet,
  converted: TONE.emerald,
  lost: TONE.red,
};

export function platformTone(platform: string | null | undefined): string {
  return (platform && PLATFORM_TONE[platform]) || TONE.neutral;
}

export function stageTone(stage: string | null | undefined): string {
  return (stage && STAGE_TONE[stage]) || TONE.neutral;
}
