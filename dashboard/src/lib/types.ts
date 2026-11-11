export type Platform = "reddit" | "hackernews" | "twitter" | "linkedin" | "devto" | "lobsters" | "indiehackers" | "googlenews" | "quora" | "exa" | "bluesky";
export type MentionStatus = "new" | "reviewed" | "replied" | "archived" | "spam";
export type IntentType = "buy_signal" | "complaint" | "recommendation_ask" | "comparison" | "general";
export type AwarenessLevel = "problem_aware" | "solution_aware" | "product_aware" | "purchase_ready";
export type TemplateStyle = "value_first" | "storytelling" | "technical_deep_dive" | "casual_helpful" | "contrarian";
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
  /** Who the author is, once enrichment has found them. */
  person?: { name?: string; title?: string; bio?: string; company?: string; employees?: string; has_email?: boolean; email?: string; linkedin_url?: string };
  author_profile_url: string | null;
  author_karma: number | null;
  author_account_age_days: number | null;
  relevance_score: number | null;
  intent: IntentType | null;
  awareness_level: AwarenessLevel | null;
  next_action?: "ready_to_send" | "needs_draft" | "needs_review" | null;
  conversion_probability: number | null;
  status: MentionStatus;
  assigned_to: string | null;
  platform_metadata: Record<string, unknown>;
  engagement_metrics: Record<string, unknown>;
  scoring_metadata: Record<string, unknown>;
  keyword_matches: string[];
  /** scoring: 13-tag taxonomy + sentiment (absent on older API builds) */
  intent_tags?: string[];
  sentiment?: "positive" | "neutral" | "negative" | null;
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
  /** The post that made them a lead (list endpoint only). */
  mention?: LeadMention | null;
}

export interface LeadMention {
  title: string | null;
  snippet: string;
  url: string;
  intent: string | null;
  score: number | null;
  posted_at: string | null;
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

export interface PersonIdentity {
  id: string;
  person_id: string;
  workspace_id: string;
  platform: string;
  handle: string;
  profile_url: string | null;
  confidence: number;
  source: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface Person360Person {
  id: string;
  workspace_id: string;
  display_name: string | null;
  bio: string | null;
  company: string | null;
  location: string | null;
  icp_fit_score: number | null;
  confidence: number;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Person360Response {
  person?: Person360Person;
  identities: PersonIdentity[];
  enriched: boolean;
}

export interface Reply {
  id: string;
  mention_id: string;
  workspace_id: string;
  content: string;
  edited_content: string | null;
  status: ReplyStatus;
  template_style: TemplateStyle | null;
  thread_context_used?: boolean;
  utm_link_id?: string | null;
  utm_code?: string | null;
  short_url?: string | null;
  created_at: string;
  updated_at: string;
}

export interface ProductAnalysis {
  product_name: string;
  description: string;
  features: string[];
  target_audience: string;
  pain_points: string[];
  competitors: string[];
  suggested_keywords: string[];
  suggested_subreddits: string[];
  suggested_platforms: Platform[];
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
  /** scoring: plain-English listener intent fed to the scorer */
  looks_for?: string;
  skips?: string;
  created_at: string;
  updated_at: string;
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

export interface QueueCount {
  queue: string;
  count: number;
}

export interface EscalationSubcounts {
  needs_draft: number;
  flagged: number;
}

export interface ActionSubcounts {
  ready_to_send: number;
  needs_draft: number;
  needs_review: number;
}

export interface InboxQueueCountsResponse {
  queues: QueueCount[];
  actions: ActionSubcounts;
  /** @deprecated use actions */
  escalations?: EscalationSubcounts;
}

export interface PlatformCount {
  platform: string;
  count: number;
}

export interface HumanProposal {
  id: string;
  workspace_id: string;
  proposal_type: string;
  title: string;
  body: string;
  status: "pending" | "accepted" | "dismissed";
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}
