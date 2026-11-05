// Human-readable summary of a write tool call, shown before the user confirms
// it. Arguments can come from attacker-written mention text via the model, so
// the user must see in plain words what will happen, not a JSON blob.

export interface CallDescription {
  title: string;
  lines: string[];
}

const MAX = 300;
const clip = (s: string, max = MAX): string => (s.length > max ? `${s.slice(0, max)}…` : s);
const quote = (v: unknown): string => `“${clip(String(v))}”`;
const list = (v: unknown): string => (Array.isArray(v) ? v.map(String).join(", ") : String(v));

type Args = Record<string, unknown>;

const DESCRIBERS: Record<string, (a: Args) => CallDescription> = {
  queue_reply: (a) => ({
    title: "Queue a reply for posting",
    // Reply text is what gets posted publicly: show it in full (server caps it at 10k).
    lines: a.content != null
      ? [`Text: “${clip(String(a.content), 10_000)}”`]
      : [`Posts the saved draft ${String(a.reply_id ?? "")} — open it in the inbox to read it before confirming`],
  }),
  draft_reply: () => ({ title: "Draft a reply", lines: [] }),
  set_stage: (a) => ({ title: "Change lead stage", lines: [`Move lead to stage: ${String(a.stage)}`, `Lead: ${String(a.lead_id ?? "?")}`] }),
  push_to_crm: (a) => ({ title: "Push this lead to your CRM", lines: a.lead_id ? [`Lead: ${String(a.lead_id)}`] : [] }),
  mute: (a) => ({ title: "Mute future mentions", lines: [`Mute ${String(a.kind)}: ${clip(String(a.value))}`] }),
  create_listener: (a) => ({
    title: a.name ? `Create listener ${quote(a.name)}` : "Create a listener",
    lines: [
      a.keywords != null ? `Keywords: ${list(a.keywords)}` : "",
      a.platforms != null ? `Platforms: ${list(a.platforms)}` : "",
      a.looks_for ? `Looks for: ${quote(a.looks_for)}` : "",
      a.skips ? `Skips: ${quote(a.skips)}` : "",
    ].filter(Boolean),
  }),
  tune_listener: (a) => ({
    title: "Change a listener",
    lines: [
      a.listener_id ? `Listener: ${String(a.listener_id)}` : "",
      a.add_keywords != null ? `Add keywords: ${list(a.add_keywords)}` : "",
      a.remove_keywords != null ? `Remove keywords: ${list(a.remove_keywords)}` : "",
      a.platforms != null ? `Platforms: ${list(a.platforms)}` : "",
      a.looks_for != null ? `Looks for: ${quote(a.looks_for)}` : "",
      a.skips != null ? `Skips: ${quote(a.skips)}` : "",
    ].filter(Boolean),
  }),
};

export function describeCall(tool: string, args: Args | undefined): CallDescription {
  const a = args ?? {};
  const fn = DESCRIBERS[tool];
  if (fn) return fn(a);
  return {
    title: `Run ${tool.replace(/_/g, " ")}`,
    lines: Object.entries(a).map(([k, v]) => `${k.replace(/_/g, " ")}: ${clip(typeof v === "string" ? v : JSON.stringify(v))}`),
  };
}
