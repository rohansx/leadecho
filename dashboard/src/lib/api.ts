import type {
  Mention,
  Lead,
  Keyword,
  Reply,
  Document,
  OverviewStats,
  WebhookConfig,
  PaginatedResponse,
  StatusCount,
  TierCount,
  MonitoringProfile,
  PlatformSession,
  ExtensionTokenInfo,
  OnboardingStatus,
  UTMLink,
} from "./types";

const BASE = "/api/v1";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status}: ${body}`);
  }
  return res.json();
}

// ─── Mentions ──────────────────────────────────────────

export function listMentions(params?: {
  status?: string;
  platform?: string;
  intent?: string;
  search?: string;
  tier?: string;
  limit?: number;
  offset?: number;
}) {
  const q = new URLSearchParams();
  if (params?.status) q.set("status", params.status);
  if (params?.platform) q.set("platform", params.platform);
  if (params?.intent) q.set("intent", params.intent);
  if (params?.search) q.set("search", params.search);
  if (params?.tier) q.set("tier", params.tier);
  if (params?.limit) q.set("limit", String(params.limit));
  if (params?.offset) q.set("offset", String(params.offset));
  const qs = q.toString();
  return request<PaginatedResponse<Mention>>(`/mentions${qs ? `?${qs}` : ""}`);
}

export function getMention(id: string) {
  return request<Mention>(`/mentions/${id}`);
}

export function updateMentionStatus(id: string, status: string) {
  return request<Mention>(`/mentions/${id}/status`, {
    method: "PATCH",
    body: JSON.stringify({ status }),
  });
}

export function mentionCounts() {
  return request<StatusCount[]>("/mentions/counts");
}

export function mentionTierCounts() {
  return request<TierCount[]>("/mentions/tier-counts");
}

// ─── Profiles (Pain-Point Monitoring) ─────────────────

export function listProfiles() {
  return request<MonitoringProfile[]>("/profiles");
}

export function getProfile(id: string) {
  return request<MonitoringProfile>(`/profiles/${id}`);
}

export function createProfile(data: {
  name: string;
  description?: string;
  pain_points?: string[];
}) {
  return request<MonitoringProfile>("/profiles", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function updateProfile(
  id: string,
  data: { name?: string; description?: string; pain_points?: string[]; is_active?: boolean },
) {
  return request<MonitoringProfile>(`/profiles/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export function deleteProfile(id: string) {
  return request<{ status: string }>(`/profiles/${id}`, { method: "DELETE" });
}

// ─── Leads ─────────────────────────────────────────────

export function listLeads(params?: {
  stage?: string;
  limit?: number;
  offset?: number;
}) {
  const q = new URLSearchParams();
  if (params?.stage) q.set("stage", params.stage);
  if (params?.limit) q.set("limit", String(params.limit));
  if (params?.offset) q.set("offset", String(params.offset));
  const qs = q.toString();
  return request<PaginatedResponse<Lead>>(`/leads${qs ? `?${qs}` : ""}`);
}

export function getLead(id: string) {
  return request<Lead>(`/leads/${id}`);
}

export function updateLeadStage(id: string, stage: string) {
  return request<Lead>(`/leads/${id}/stage`, {
    method: "PATCH",
    body: JSON.stringify({ stage }),
  });
}

export function createLead(data: Partial<Lead>) {
  return request<Lead>("/leads", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function leadCounts() {
  return request<StatusCount[]>("/leads/counts");
}

// ─── Keywords ──────────────────────────────────────────

export function listKeywords() {
  return request<Keyword[]>("/keywords");
}

export function createKeyword(data: {
  term: string;
  platforms?: string[];
  match_type?: string;
  negative_terms?: string[];
  subreddits?: string[];
}) {
  return request<Keyword>("/keywords", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function updateKeyword(
  id: string,
  data: Partial<Keyword & { is_active: boolean }>,
) {
  return request<Keyword>(`/keywords/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export function deleteKeyword(id: string) {
  return request<{ status: string }>(`/keywords/${id}`, { method: "DELETE" });
}

// ─── Replies ───────────────────────────────────────────

export function listReplies(mentionId: string) {
  return request<Reply[]>(`/mentions/${mentionId}/replies`);
}

export function createReply(mentionId: string, content: string) {
  return request<Reply>("/replies", {
    method: "POST",
    body: JSON.stringify({ mention_id: mentionId, content }),
  });
}

export function updateReplyContent(id: string, content: string) {
  return request<Reply>(`/replies/${id}/content`, {
    method: "PATCH",
    body: JSON.stringify({ content }),
  });
}

export function updateReplyStatus(id: string, status: string) {
  return request<Reply>(`/replies/${id}/status`, {
    method: "PATCH",
    body: JSON.stringify({ status }),
  });
}

// ─── Documents (Knowledge Base) ────────────────────────

export function listDocuments() {
  return request<Document[]>("/documents");
}

export function getDocument(id: string) {
  return request<Document>(`/documents/${id}`);
}

export function createDocument(data: {
  title: string;
  content: string;
  content_type?: string;
  source_url?: string;
}) {
  return request<Document>("/documents", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function updateDocument(
  id: string,
  data: { title?: string; content?: string },
) {
  return request<Document>(`/documents/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export function deleteDocument(id: string) {
  return request<{ status: string }>(`/documents/${id}`, { method: "DELETE" });
}

// ─── Analytics ─────────────────────────────────────────

export function analyticsOverview() {
  return request<OverviewStats>("/analytics/overview");
}

export function mentionsPerDay() {
  return request<{ day: string; count: number }[]>(
    "/analytics/mentions-per-day",
  );
}

export function mentionsPerPlatform() {
  return request<{ platform: string; count: number }[]>(
    "/analytics/mentions-per-platform",
  );
}

export function mentionsPerIntent() {
  return request<{ intent: string; count: number }[]>(
    "/analytics/mentions-per-intent",
  );
}

export function conversionFunnel() {
  return request<{ stage: string; count: number }[]>(
    "/analytics/conversion-funnel",
  );
}

export function topKeywords() {
  return request<{ term: string; mention_count: number }[]>(
    "/analytics/top-keywords",
  );
}

// ─── Notifications (Webhooks) ──────────────────────────

export function getWebhookConfig() {
  return request<WebhookConfig>("/notifications/webhooks");
}

export function saveWebhookConfig(config: WebhookConfig) {
  return request<WebhookConfig>("/notifications/webhooks", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

export function testWebhook(channel: string, webhookUrl: string) {
  return request<{ status: string }>("/notifications/webhooks/test", {
    method: "POST",
    body: JSON.stringify({ channel, webhook_url: webhookUrl }),
  });
}

// ─── Auth (Email/Password) ────────────────────────────

export interface AuthResponse {
  user_id: string;
  workspace_id: string;
  email: string;
  name: string;
  role: string;
}

export function registerWithEmail(email: string, password: string, name: string) {
  return request<AuthResponse>("/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password, name }),
  });
}

export function loginWithEmail(email: string, password: string) {
  return request<AuthResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

// ─── AI (Intent Classification + Reply Drafting) ──────

export interface ClassifyResponse {
  mention: Mention;
  reasoning: string;
}

export interface DraftReplyResponse {
  reply: Reply;
  tone: string;
}

export function classifyMention(id: string) {
  return request<ClassifyResponse>(`/mentions/${id}/classify`, {
    method: "POST",
  });
}

export function draftReply(id: string) {
  return request<DraftReplyResponse>(`/mentions/${id}/draft-reply`, {
    method: "POST",
  });
}

// ─── Browser Sessions ──────────────────────────────────

export function listSessions() {
  return request<PlatformSession[]>("/settings/sessions");
}

export function saveSession(platform: string, data: { session_cookie: string; username?: string }) {
  return request<PlatformSession>(`/settings/sessions/${platform}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export function deleteSession(platform: string) {
  return request<PlatformSession>(`/settings/sessions/${platform}`, { method: "DELETE" });
}

export function testSession(platform: string) {
  return request<{ pinchtab_online: boolean; message: string }>(
    `/settings/sessions/${platform}/test`,
    { method: "POST" },
  );
}

// ─── Settings (BYOK API Keys) ─────────────────────────

export interface APIKeyStatus {
  provider: string;
  is_set: boolean;
  masked_key?: string;
}

export function getAPIKeys() {
  return request<APIKeyStatus[]>("/settings/api-keys");
}

export function saveAPIKey(provider: string, apiKey: string) {
  return request<APIKeyStatus>("/settings/api-keys", {
    method: "PUT",
    body: JSON.stringify({ provider, api_key: apiKey }),
  });
}

export function deleteAPIKey(provider: string) {
  return request<APIKeyStatus>("/settings/api-keys", {
    method: "DELETE",
    body: JSON.stringify({ provider }),
  });
}

// ─── Chrome Extension Token ───────────────────────────

export function getExtensionToken() {
  return request<ExtensionTokenInfo>("/settings/extension-token");
}

export function rotateExtensionToken(name?: string) {
  return request<{ token: string; name: string; created_at: string }>(
    "/settings/extension-token",
    { method: "POST", body: JSON.stringify({ name: name ?? "Default" }) },
  );
}

export function revokeExtensionToken() {
  return request<{ status: string }>("/settings/extension-token", {
    method: "DELETE",
  });
}

// ─── Onboarding ────────────────────────────────────────

export function getOnboardingStatus() {
  return request<OnboardingStatus>("/settings/onboarding");
}

export function updateOnboarding(data: { step?: number; completed?: boolean }) {
  return request<OnboardingStatus>("/settings/onboarding", {
    method: "PATCH",
    body: JSON.stringify(data),
  });
}

// ─── UTM Tracking Links ────────────────────────────────

export function listUTMLinks() {
  return request<UTMLink[]>("/utm-links");
}

export function createUTMLink(data: {
  destination_url: string;
  utm_source: string;
  utm_campaign?: string;
  utm_medium?: string;
  utm_content?: string;
}) {
  return request<UTMLink>("/utm-links", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function deleteUTMLink(id: string) {
  return request<{ status: string }>(`/utm-links/${id}`, { method: "DELETE" });
}
