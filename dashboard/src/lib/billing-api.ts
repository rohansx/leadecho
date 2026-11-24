// billing: plan, credits, Dodo checkout and customer portal.
import { request } from "./api";

export type PlanId = "free" | "solo" | "starter" | "pro";
export type CheckoutItem = Exclude<PlanId, "free"> | "topup";

export interface Plan {
  id: PlanId;
  name: string;
  price_cents: number;
  monthly_credits: number;
  keyword_limit: number;
  /** Includes paid data (X, Reddit and LinkedIn, less locked_platforms). */
  paid_data: boolean;
  /** Paid platforms the plan still can't search (Solo: X and LinkedIn). */
  locked_platforms: string[] | null;
}

export interface BillingState {
  plan: PlanId;
  credit_balance: number;
  keyword_limit: number;
  keywords_used: number;
  plan_renews_at: string | null;
  has_portal: boolean;
  usage_30d: { reason: string; credits: number }[];
  plans: Plan[];
  topup: { credits: number; price_cents: number };
  costs: { post: number; search: number; draft: number; chat: number };
  enforced: boolean;
  can_checkout: boolean;
  /** Items this server can sell (a plan with no Dodo product is left out). */
  checkout_items: CheckoutItem[];
  /** Platforms this workspace's plan can't search; the app shows them locked. */
  locked_platforms: string[];
  /** The daily prospect scan runs for this workspace (Pro, not switched off). */
  prospect_daily: boolean;
  /** This workspace may switch daily prospecting on (Pro, or self-host). */
  prospect_allowed: boolean;
  /** The plan that includes daily prospecting. */
  prospect_daily_plan: PlanId;
}

export async function setProspectDaily(on: boolean): Promise<void> {
  await request("/billing/prospecting", { method: "PUT", body: JSON.stringify({ on }) });
}

/** Platforms searched with paid data (mirrors billing.PaidPlatforms), with their labels. */
export const PAID_PLATFORMS: readonly (readonly [string, string])[] = [["twitter", "X"], ["reddit", "Reddit"], ["linkedin", "LinkedIn"]];

export const BILLING_QUERY = ["billing"];

export function getBilling(): Promise<BillingState> {
  return request<BillingState>("/billing");
}

/** Starts a Dodo hosted checkout and returns the URL to send the user to. */
export async function startCheckout(item: CheckoutItem): Promise<string> {
  const res = await request<{ checkout_url: string }>("/billing/checkout", {
    method: "POST",
    body: JSON.stringify({ item }),
  });
  return res.checkout_url;
}

export async function openBillingPortal(): Promise<string> {
  const res = await request<{ url: string }>("/billing/portal", { method: "POST" });
  return res.url;
}

export const USAGE_LABELS: Record<string, string> = {
  post_scored: "Posts scored",
  search: "Paid searches",
  draft_reply: "Reply drafts",
  chat: "Chat questions",
  enrichment: "Enrichment",
  research: "Company research",
};
