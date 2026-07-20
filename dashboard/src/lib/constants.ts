export const API_BASE_URL =
  import.meta.env.VITE_API_URL ?? "http://localhost:8090";

export const PLATFORMS = [
  "reddit",
  "hackernews",
  "twitter",
  "linkedin",
] as const;
export type Platform = (typeof PLATFORMS)[number];

export const LEAD_STAGES = [
  "prospect",
  "qualified",
  "engaged",
  "converted",
  "lost",
] as const;
export type LeadStage = (typeof LEAD_STAGES)[number];

export const INTENT_TYPES = [
  "buy_signal",
  "complaint",
  "recommendation_ask",
  "comparison",
  "general",
] as const;
export type IntentType = (typeof INTENT_TYPES)[number];

export const MENTION_TIERS = {
  LEADS_READY: "leads_ready",
  WORTH_WATCHING: "worth_watching",
  FILTERED: "filtered",
} as const;

export const INBOX_QUEUES = {
  AUTO_FLOWING: "auto_flowing",
  ESCALATIONS: "escalations",
  PROPOSALS: "proposals",
} as const;

export const MENTION_STATUSES = ["new", "reviewed", "replied", "archived", "spam"] as const;
export type MentionStatus = (typeof MENTION_STATUSES)[number];

export const QUERY_KEYS = {
  mentions: "mentions",
  mentionCounts: "mentionCounts",
  mentionTierCounts: "mentionTierCounts",
  mentionQueueCounts: "mentionQueueCounts",
  proposals: "proposals",
  proposalCounts: "proposalCounts",
  mentionsPerPlatform: "mentionsPerPlatform",
  replies: "replies",
  person360: "person360",
  scoringPrecision: "scoringPrecision",
  replyAttribution: "replyAttribution",
} as const;

export const GITHUB_URL = "https://github.com/rohansx/leadecho";
