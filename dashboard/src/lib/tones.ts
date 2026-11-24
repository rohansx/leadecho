// Single source of truth for categorical colors (platforms, lead stages, intents).
// Chips: a tint with -700 text in light mode and -300 in dark, so they read in
// both. Bars: the same hue, solid. Class names stay literal for Tailwind.

const TONE = {
  neutral: "bg-muted text-foreground-soft",
  orange: "bg-orange-500/10 text-orange-700 dark:text-orange-300",
  amber: "bg-amber-500/10 text-amber-700 dark:text-amber-300",
  sky: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  blue: "bg-blue-500/10 text-blue-700 dark:text-blue-300",
  violet: "bg-violet-500/10 text-violet-700 dark:text-violet-300",
  emerald: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  red: "bg-red-500/10 text-red-700 dark:text-red-300",
  indigo: "bg-indigo-500/10 text-indigo-700 dark:text-indigo-300",
  rose: "bg-rose-500/10 text-rose-700 dark:text-rose-300",
  teal: "bg-teal-500/10 text-teal-700 dark:text-teal-300",
  cyan: "bg-cyan-500/10 text-cyan-700 dark:text-cyan-300",
  lime: "bg-lime-500/10 text-lime-700 dark:text-lime-300",
} as const;

const BAR = {
  neutral: "bg-foreground/40",
  orange: "bg-orange-500",
  amber: "bg-amber-500",
  sky: "bg-sky-500",
  blue: "bg-blue-500",
  violet: "bg-violet-500",
  emerald: "bg-emerald-500",
  red: "bg-red-500",
  indigo: "bg-indigo-500",
  rose: "bg-rose-500",
  teal: "bg-teal-500",
  cyan: "bg-cyan-500",
  lime: "bg-lime-500",
} as const;

type Hue = keyof typeof TONE;

const PLATFORM_HUE: Record<string, Hue> = {
  reddit: "orange",
  hackernews: "amber",
  twitter: "sky",
  linkedin: "blue",
  quora: "violet",
  exa: "emerald",
  devto: "indigo",
  lobsters: "rose",
  indiehackers: "teal",
  bluesky: "cyan",
  googlenews: "lime",
};

const STAGE_HUE: Record<string, Hue> = {
  prospect: "neutral",
  qualified: "blue",
  engaged: "violet",
  converted: "emerald",
  lost: "red",
};

const INTENT_HUE: Record<string, Hue> = {
  buy_signal: "emerald",
  recommendation_ask: "blue",
  comparison: "violet",
  complaint: "rose",
  general: "neutral",
};

// Intent tags by what they mean for a seller, in the same hues as the intents
// above: buying, asking, comparing, pain, talk about you or a competitor.
// Anything else (launches, hiring, essays) is noise and stays neutral.
const TAG_HUE: Record<string, Hue> = {
  buying_intent: "emerald",
  seeking_alternative: "emerald",
  recommendation_request: "blue",
  comparison: "violet",
  competitor_complaint: "rose",
  pain_point: "rose",
  brand_mention: "indigo",
  competitor_mention: "indigo",
  praise: "indigo",
};

const hue = (map: Record<string, Hue>, key: string | null | undefined): Hue => (key && map[key]) || "neutral";

export const platformTone = (platform: string | null | undefined): string => TONE[hue(PLATFORM_HUE, platform)];
export const platformBar = (platform: string | null | undefined): string => BAR[hue(PLATFORM_HUE, platform)];
export const stageTone = (stage: string | null | undefined): string => TONE[hue(STAGE_HUE, stage)];
export const stageBar = (stage: string | null | undefined): string => BAR[hue(STAGE_HUE, stage)];
export const intentBar = (intent: string | null | undefined): string => BAR[hue(INTENT_HUE, intent)];
export const intentTone = (intent: string | null | undefined): string => TONE[hue(INTENT_HUE, intent)];
export const voteTone = (vote: "up" | "down"): string => (vote === "up" ? TONE.emerald : TONE.red);
export const tagTone = (tag: string): string => TONE[hue(TAG_HUE, tag)];

export interface ScoreBand {
  label: string;
  tone: string;
}

// Mirrors the backend tiers: 6+ can be a lead, 4+ is worth watching.
export function scoreBand(score: number | null | undefined): ScoreBand {
  if (score == null) return { label: "Not scored", tone: TONE.neutral };
  if (score >= 8) return { label: "Strong fit", tone: TONE.emerald };
  if (score >= 6) return { label: "Good fit", tone: TONE.blue };
  if (score >= 4) return { label: "Worth watching", tone: TONE.amber };
  return { label: "Low fit", tone: TONE.neutral };
}

// What is still open for the author (scoring_metadata.open_need). "none" and
// absent (posts scored before the field existed) show nothing.
const OPEN_NEED: Record<string, ScoreBand> = {
  asking: { label: "Asking for a tool", tone: TONE.emerald },
  evaluating: { label: "Evaluating options", tone: TONE.violet },
  switching: { label: "Switching away", tone: TONE.blue },
  unsolved_pain: { label: "Unsolved pain", tone: TONE.rose },
};

export const openNeedTone = (need: unknown): ScoreBand | null => typeof need === "string" && Object.hasOwn(OPEN_NEED, need) ? OPEN_NEED[need] : null;
