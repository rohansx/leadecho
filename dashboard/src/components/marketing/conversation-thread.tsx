import { Reveal, StaggerGroup, StaggerItem } from "./reveal";

const nodes = [
  {
    kind: "system",
    when: "Mon 09:14",
    who: "Scout + Triage + Researcher",
    body: "Post detected. Intent 9.2 · ICP 88%. Person360 stitched (3 handles). Routed to Conversation agent.",
  },
  {
    kind: "agent",
    when: "Mon 09:14",
    who: "Conversation · draft (storytelling)",
    body: "“Hey — I built LeadEcho partly because I was hitting the exact API issue you describe. It's MIT-licensed and runs on docker compose, so the only cost is your own AI keys.”",
  },
  {
    kind: "human",
    when: "Mon 09:18",
    who: "You · approved & posted via Chrome ext",
    body: "→ Reply posted as your account, 09:18:42.",
  },
  {
    kind: "system",
    when: "Mon 14:32",
    who: "OP replied",
    body: "“Interesting, thanks. Does the AI scoring work with Anthropic? We're already paying for Claude credits.”",
  },
  {
    kind: "agent",
    when: "Mon 14:32",
    who: "Conversation · follow-up (technical)",
    body: "“Yep — the LLM router supports Anthropic out of the box. Config takes 2 lines in the .env.”",
  },
  {
    kind: "conv",
    when: "Wed 10:14",
    who: "Closed-won",
    body: "$348 / yr · plan: Starter (yearly) · style “storytelling” conversion rate in r/saas now 4.2× baseline.",
  },
];

const kindStyles: Record<string, string> = {
  system: "border-border bg-muted/60 text-muted-foreground",
  agent: "border-primary/25 bg-accent-soft/50",
  human: "border-border bg-card",
  conv: "border-primary/40 bg-primary/10",
};

export function ConversationThreadSection() {
  return (
    <section className="py-24 border-y border-border bg-card">
      <div className="max-w-4xl mx-auto px-6">
        <div className="mb-12">
          <Reveal>
            <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
              Conversation agent
            </div>
            <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight">
              Runs the thread.
              <br />
              You stay in the{" "}
              <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">loop</em>.
            </h2>
          </Reveal>
          <Reveal delay={0.1}>
            <p className="mt-4 text-lg text-foreground-soft leading-relaxed max-w-2xl">
              The Conversation agent owns a thread from first draft to closed conversion. Every step is auditable;
              nothing posts without your click.
            </p>
          </Reveal>
        </div>

        <Reveal>
          <div className="flex items-center justify-between mb-4 text-sm">
            <div>
              <div className="font-medium">r/saas · &ldquo;Recs for monitoring Reddit at scale?&rdquo;</div>
              <div className="text-muted-foreground text-xs mt-0.5">Daniel R. (ICP 88%) · 5 turns · closed-won</div>
            </div>
            <span className="font-[family-name:var(--font-mono)] text-xs text-muted-foreground">
              Conv #1024 · storytelling
            </span>
          </div>
        </Reveal>

        <StaggerGroup className="space-y-3">
          {nodes.map((n, i) => (
            <StaggerItem key={i} className="flex gap-4 items-start">
              <div className="w-16 shrink-0 text-right text-xs text-muted-foreground font-[family-name:var(--font-mono)] pt-3">
                {n.when}
              </div>
              <div className={`flex-1 rounded-xl border p-4 ${kindStyles[n.kind]}`}>
                <div className="text-xs font-medium mb-1">{n.who}</div>
                <div className="text-sm leading-relaxed">{n.body}</div>
              </div>
            </StaggerItem>
          ))}
        </StaggerGroup>
      </div>
    </section>
  );
}
