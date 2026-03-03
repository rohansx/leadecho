export type Platform = "reddit" | "hackernews" | "twitter" | "linkedin" | "devto" | "lobsters" | "indiehackers";
export type MentionStatus = "new" | "reviewed" | "replied" | "archived" | "spam";
export type IntentType = "buy_signal" | "complaint" | "recommendation_ask" | "comparison" | "general";
export type LeadStage = "prospect" | "qualified" | "engaged" | "converted" | "lost";
export type ReplyStatus = "draft" | "approved" | "posted" | "failed";

export interface Mention {
  id: string;
  workspace_id: string;
  keyword_id: string | null;
  platform: Platform;
  platform_id: string;
  url: string;
  title: string | null;
  content: string;
  author_username: string | null;
  author_profile_url: string | null;
  author_karma: number | null;
  author_account_age_days: number | null;
  relevance_score: number | null;
  intent: IntentType | null;
  conversion_probability: number | null;
  status: MentionStatus;
  assigned_to: string | null;
  platform_metadata: Record<string, unknown>;
  engagement_metrics: Record<string, unknown>;
  scoring_metadata: Record<string, unknown>;
  keyword_matches: string[];
  platform_created_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface Lead {
  id: string;
  workspace_id: string;
  mention_id: string | null;
  stage: LeadStage;
  contact_name: string | null;
  contact_email: string | null;
  company: string | null;
  username: string | null;
  platform: Platform | null;
  profile_url: string | null;
  estimated_value: number | null;
  notes: string | null;
  tags: string[];
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Keyword {
  id: string;
  workspace_id: string;
  term: string;
  platforms: Platform[];
  is_active: boolean;
  match_type: string;
  negative_terms: string[];
  subreddits: string[];
  created_at: string;
  updated_at: string;
}

export interface Reply {
  id: string;
  mention_id: string;
  workspace_id: string;
  content: string;
  edited_content: string | null;
  status: ReplyStatus;
  created_at: string;
  updated_at: string;
}

export interface Document {
  id: string;
  workspace_id: string;
  title: string;
  content: string;
  content_type: string;
  source_url?: string;
  file_size_bytes?: number;
  chunk_count: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface OverviewStats {
  mentions_30d: number;
  mentions_new: number;
  total_leads: number;
  converted_leads: number;
  replies_posted: number;
  active_keywords: number;
}

export interface WebhookConfig {
  slack_url: string;
  discord_url: string;
  email_to: string;
  enabled: boolean;
  on_new_mention: boolean;
  on_high_intent: boolean;
  on_new_lead: boolean;
  resend_configured?: boolean;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface StatusCount {
  status: string;
  count: number;
}

export interface TierCount {
  tier: string;
  count: number;
}

export interface MonitoringProfile {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  pain_points: string[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface PlatformSession {
  platform: "reddit" | "twitter" | "linkedin";
  username: string | null;
  is_configured: boolean;
  is_pinchtab_online: boolean;
}

export interface ExtensionTokenInfo {
  has_token: boolean;
  masked_token?: string;
  name?: string;
  last_used_at?: string;
  created_at?: string;
}

export interface OnboardingStatus {
  completed: boolean;
  step: number;
}

export interface UTMLink {
  id: string;
  workspace_id: string;
  code: string;
  destination_url: string;
  utm_source: string;
  utm_medium: string;
  utm_campaign: string | null;
  utm_content: string | null;
  click_count: number;
  signup_count: number;
  revenue_cents: number;
  created_at: string;
}
