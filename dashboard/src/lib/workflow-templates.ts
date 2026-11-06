// workflows: shared types, labels, ready-made templates and summary helpers.
// No imports so `node --test` can load it directly.

export type ActionType =
  | "notify_slack"
  | "notify_discord"
  | "notify_email"
  | "ai_draft"
  | "create_lead"
  | "archive"
  | "webhook"
  | "push_crm";

export interface WorkflowAction {
  type: ActionType;
  config?: { url?: string; tag?: string };
}

export interface WorkflowTrigger {
  min_score: number;
  platforms?: string[];
  intent_tags?: string[];
  keywords?: string[];
  exclude_keywords?: string[];
}

export interface WorkflowDefinition {
  name: string;
  enabled: boolean;
  trigger: WorkflowTrigger;
  actions: WorkflowAction[];
  max_per_hour?: number;
}

/** The classifier's intent tags (backend/internal/ai/taxonomy.go), in plain words. */
export const INTENT_LABELS: Record<string, string> = {
  buying_intent: "Ready to buy",
  recommendation_request: "Asking for recommendations",
  seeking_alternative: "Looking to switch",
  comparison: "Comparing tools",
  competitor_complaint: "Unhappy with a competitor",
  competitor_mention: "Mentions a competitor",
  brand_mention: "Mentions you",
  praise: "Praise",
  pain_point: "Describes the problem you solve",
  feature_request: "Feature request",
  content_opportunity: "Content idea",
  hiring: "Hiring post",
  promotional: "Self-promotion",
};

export const ACTION_INFO: Record<ActionType, { label: string; hint: string }> = {
  notify_slack: { label: "Send to Slack", hint: "Uses the Slack webhook from Alerts" },
  notify_discord: { label: "Send to Discord", hint: "Uses the Discord webhook from Alerts" },
  notify_email: { label: "Email me", hint: "Alerts email, or the workspace owner" },
  ai_draft: { label: "Draft an AI reply", hint: "Waits in the inbox for your review · 5 credits" },
  create_lead: { label: "Add to pipeline", hint: "Optionally with a tag" },
  push_crm: { label: "Push to CRM", hint: "HubSpot / Attio, if connected" },
  webhook: { label: "Send to a webhook", hint: "Zapier, Make, n8n… (https)" },
  archive: { label: "Archive it", hint: "Hide it from the inbox" },
};

export const ACTION_ORDER: ActionType[] = [
  "notify_slack", "notify_discord", "notify_email", "ai_draft", "create_lead", "push_crm", "webhook", "archive",
];

export interface WorkflowTemplate {
  id: string;
  title: string;
  why: string;
  def: WorkflowDefinition;
}

const BUYING = ["buying_intent", "recommendation_request", "seeking_alternative"];

export const TEMPLATES: WorkflowTemplate[] = [
  {
    id: "hot-buyers",
    title: "Hot buyer alert",
    why: "Someone is ready to buy: ping the team, draft a reply and track it.",
    def: {
      name: "Hot buyer alert", enabled: true,
      trigger: { min_score: 7, intent_tags: BUYING },
      actions: [{ type: "notify_slack" }, { type: "ai_draft" }, { type: "create_lead", config: { tag: "hot" } }],
    },
  },
  {
    id: "switchers",
    title: "Competitor switchers",
    why: "People unhappy with a competitor or looking to switch — reply while they're shopping.",
    def: {
      name: "Competitor switchers", enabled: true,
      trigger: { min_score: 6, intent_tags: ["seeking_alternative", "competitor_complaint"] },
      actions: [{ type: "ai_draft" }, { type: "create_lead", config: { tag: "switcher" } }, { type: "notify_email" }],
    },
  },
  {
    id: "recommendations",
    title: "Answer recommendation threads",
    why: "\"What tool do you use for…?\" threads: have a reply drafted for each one.",
    def: {
      name: "Recommendation threads", enabled: true,
      trigger: { min_score: 6, intent_tags: ["recommendation_request", "comparison"] },
      actions: [{ type: "ai_draft" }],
    },
  },
  {
    id: "crm",
    title: "Best leads to CRM",
    why: "Only the strongest buying signals land in HubSpot / Attio.",
    def: {
      name: "Best leads to CRM", enabled: true,
      trigger: { min_score: 8, intent_tags: BUYING },
      actions: [{ type: "create_lead" }, { type: "push_crm" }],
    },
  },
  {
    id: "feedback",
    title: "Product feedback",
    why: "Feature requests and pain points, straight to your product channel.",
    def: {
      name: "Product feedback", enabled: true,
      trigger: { min_score: 5, intent_tags: ["feature_request", "pain_point"] },
      actions: [{ type: "notify_slack" }],
    },
  },
  {
    id: "brand",
    title: "Brand mentions & praise",
    why: "Know when people talk about you — great for social proof.",
    def: {
      name: "Brand mentions", enabled: true,
      trigger: { min_score: 0, intent_tags: ["brand_mention", "praise"] },
      actions: [{ type: "notify_slack" }],
    },
  },
  {
    id: "content",
    title: "Content ideas",
    why: "Questions worth a blog post or a thread, emailed as they come in.",
    def: {
      name: "Content ideas", enabled: true, max_per_hour: 5,
      trigger: { min_score: 5, intent_tags: ["content_opportunity"] },
      actions: [{ type: "notify_email" }],
    },
  },
  {
    id: "noise",
    title: "Hide hiring & self-promo",
    why: "Keep the inbox clean: archive job posts and self-promotion automatically.",
    def: {
      name: "Hide noise", enabled: true,
      trigger: { min_score: 0, intent_tags: ["hiring", "promotional"] },
      actions: [{ type: "archive" }],
    },
  },
  {
    id: "zapier",
    title: "Send to Zapier / Make / n8n",
    why: "Pipe strong leads into any tool with a webhook.",
    def: {
      name: "Leads to webhook", enabled: false,
      trigger: { min_score: 7, intent_tags: BUYING },
      actions: [{ type: "webhook", config: { url: "" } }],
    },
  },
];

export function blankDefinition(): WorkflowDefinition {
  return { name: "", enabled: true, trigger: { min_score: 6, intent_tags: [] }, actions: [{ type: "ai_draft" }] };
}

/** "Score 7+ · reddit, hackernews · Ready to buy or Looking to switch". */
export function describeTrigger(t: WorkflowTrigger): string {
  const parts: string[] = [t.min_score > 0 ? `Score ${t.min_score}+` : "Any score"];
  if (t.platforms?.length) parts.push(t.platforms.join(", "));
  if (t.intent_tags?.length) parts.push(t.intent_tags.map((x) => INTENT_LABELS[x] ?? x.replace(/_/g, " ")).join(" or "));
  if (t.keywords?.length) parts.push(`mentions ${t.keywords.map((k) => `"${k}"`).join(" or ")}`);
  if (t.exclude_keywords?.length) parts.push(`not ${t.exclude_keywords.map((k) => `"${k}"`).join(", ")}`);
  return parts.join(" · ");
}

export function describeActions(actions: WorkflowAction[]): string {
  return actions.map((a) => ACTION_INFO[a.type]?.label ?? a.type).join(" → ");
}

/** Client-side check mirroring the server's, so the form can explain itself. */
export function validateDefinition(d: WorkflowDefinition): string | null {
  if (!d.name.trim()) return "Give it a name.";
  if (d.actions.length === 0) return "Pick at least one action.";
  const types = new Set(d.actions.map((a) => a.type));
  if (types.has("archive") && (types.has("ai_draft") || types.has("create_lead") || types.has("push_crm")))
    return "Archive can't be combined with drafting or pipeline actions.";
  const hook = d.actions.find((a) => a.type === "webhook");
  if (hook && !/^https:\/\/\S+$/.test(hook.config?.url ?? "")) return "Webhook needs an https:// URL.";
  return null;
}

/** "a, b ,, c" -> ["a", "b", "c"] */
export function splitList(s: string): string[] {
  return s.split(",").map((x) => x.trim()).filter(Boolean);
}
