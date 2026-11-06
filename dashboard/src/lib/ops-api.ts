// ops: operator console API (server-side email allowlist; 403 for everyone else).
import { request } from "./api";

export interface Spend {
  treg_usd: number;
  treg_calls: number;
  ai_usd: number;
}

export interface OpsOverview {
  treg_balance_usd?: number;
  treg_balance_error?: string;
  spend_today: Spend;
  spend_30d: Spend;
  paid_data_paused: boolean;
  credits_enforced: boolean;
}

export interface OpsWorkspace {
  id: string;
  name: string;
  owner_email: string;
  created_at: string;
  plan: "free" | "starter" | "pro";
  plan_source: "free" | "dodo" | "comp";
  credit_balance: number;
  keyword_limit: number;
  active_keywords: number;
  paid_data: boolean;
  scanning_paused: boolean;
  dodo_subscription: boolean;
  plan_renews_at: string | null;
  last_post_at: string;
  credits_used_30d: number;
  treg_usd_30d: number;
  ai_usd_30d: number;
}

export interface OpsAuditEntry {
  actor_email: string;
  action: string;
  workspace_name: string;
  detail: Record<string, unknown>;
  created_at: string;
}

export const OPS_KEYS = {
  me: ["ops", "me"],
  overview: ["ops", "overview"],
  workspaces: ["ops", "workspaces"],
  audit: ["ops", "audit"],
};

export const opsMe = () => request<{ ops: boolean }>("/ops/me");
export const getOpsOverview = () => request<OpsOverview>("/ops/overview");
export const listOpsWorkspaces = () => request<{ data: OpsWorkspace[] }>("/ops/workspaces");
export const listOpsAudit = () => request<{ data: OpsAuditEntry[] }>("/ops/audit");

const post = (path: string, body: unknown) =>
  request<{ ok: boolean }>(path, { method: "POST", body: JSON.stringify(body) });

export const setPaidDataPaused = (paused: boolean) => post("/ops/paid-data-paused", { paused });
export const setWorkspacePlan = (id: string, plan: string) => post(`/ops/workspaces/${id}/plan`, { plan });
export const adjustCredits = (id: string, delta: number, note: string) => post(`/ops/workspaces/${id}/credits`, { delta, note });
export const setPaidData = (id: string, enabled: boolean) => post(`/ops/workspaces/${id}/paid-data`, { enabled });
export const setScanning = (id: string, paused: boolean) => post(`/ops/workspaces/${id}/scanning`, { paused });

export const usd = (n: number) => (n >= 1 ? `$${n.toFixed(2)}` : `$${n.toFixed(3)}`);
