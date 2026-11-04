// Single source of truth for categorical colors (platforms, lead stages, intents).
// Tinted backgrounds + theme-aware text so chips read in both light and dark mode.

const TONE = {
  neutral: "bg-muted text-foreground-soft",
  orange: "bg-orange-500/10 text-orange-700 dark:text-orange-300",
  amber: "bg-amber-500/12 text-amber-700 dark:text-amber-300",
  sky: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  blue: "bg-blue-500/10 text-blue-700 dark:text-blue-300",
  violet: "bg-violet-500/10 text-violet-700 dark:text-violet-300",
  emerald: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  red: "bg-red-500/10 text-red-700 dark:text-red-300",
} as const;

export const PLATFORM_TONE: Record<string, string> = {
  reddit: TONE.orange,
  hackernews: TONE.amber,
  twitter: TONE.sky,
  linkedin: TONE.blue,
  quora: TONE.violet,
  exa: TONE.emerald,
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
