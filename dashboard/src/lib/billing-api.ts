// billing: plan, credits, Dodo checkout and customer portal.
import { request } from "./api";

export type PlanId = "free" | "starter" | "pro";

export interface Plan {
  id: PlanId;
  name: string;
  price_cents: number;
  monthly_credits: number;
  keyword_limit: number;
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
}

export const BILLING_QUERY = ["billing"];

export function getBilling(): Promise<BillingState> {
  return request<BillingState>("/billing");
}

/** Starts a Dodo hosted checkout and returns the URL to send the user to. */
export async function startCheckout(item: "starter" | "pro" | "topup"): Promise<string> {
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
};
