import { ArrowDownUp, GitBranch, Search, Sparkles, TrendingUp } from "lucide-react";
import { Reveal, StaggerGroup, StaggerItem } from "./reveal";
import { CountUp } from "./count-up";

const stats = [
  { n: 7, label: "Communities watched", sub: "reddit · hn · linkedin · x · dev.to · lobste.rs · ih" },
  { n: 4, suffix: "-stage", label: "Intent scoring pipeline", sub: "spam → semantic → intent → conversion" },
  { n: 0, label: "Keyword rules to maintain", sub: "matches meaning, not strings" },
  { n: 0, prefix: "$", label: "Of AI cost to us", sub: "BYO keys, every call routed to your provider" },
];

const agents = [
  {
    icon: Search,
    num: "01 · ingest",
    name: "Scout",
    desc: "Pulls posts from 7 platforms on a 5-min schedule. Stays intentionally dumb — only collects.",
    tools: ["reddit api", "pinchtab", "camoufox"],
  },
  {
    icon: ArrowDownUp,
    num: "02 · score",
    name: "Triage",
    desc: "4-stage filter: spam → semantic → intent → conversion. Drops ~95% before you ever see them.",
    tools: ["haiku 4.5", "voyage", "sonnet 4.5"],
  },
  {
    icon: Sparkles,
    num: "03 · enrich",
    name: "Researcher",
    desc: "Stitches identity across GitHub / LinkedIn / X / Reddit. Builds Person360 + Account + ICP fit score.",
    tools: ["github", "web_search", "embeddings"],
  },
  {
    icon: GitBranch,
    num: "04 · engage",
    name: "Conversation",
    desc: "Owns a thread end-to-end. Drafts initial reply & follow-ups. Escalates when intent crosses threshold.",
    tools: ["KB RAG", "playbooks", "escalation"],
  },
  {
    icon: TrendingUp,
    num: "05 · close",
    name: "Attribution",
    desc: "UTM clicks → signups → CRM events. Tags every reply with conversion outcome. Feeds the flywheel.",
    tools: ["hubspot", "attio", "webhooks"],
  },
];

export function SubheroLine() {
  return (
    <section className="py-16">
      <div className="max-w-3xl mx-auto px-6 text-center">
        <Reveal>
          <p className="text-2xl lg:text-3xl leading-snug text-balance font-[family-name:var(--font-head)] font-medium">
            Not keyword alerts. Not another AI SDR. A{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">
              buying-intent engine
            </em>{" "}
            that understands meaning, scores intent, and shows you the person behind every signal — open source, on
            your infra.
          </p>
        </Reveal>
      </div>
    </section>
  );
}

export function EditorialStats() {
  return (
    <section className="py-8">
      <div className="max-w-6xl mx-auto px-6">
        <StaggerGroup className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
          {stats.map((s) => (
            <StaggerItem key={s.label} className="border-t-2 border-border pt-4">
              <div className="text-4xl font-[family-name:var(--font-head)] font-medium text-primary-ink">
                <CountUp value={s.n} prefix={s.prefix} suffix={s.suffix} />
              </div>
              <div className="mt-1.5 font-medium text-sm">{s.label}</div>
              <div className="text-xs text-muted-foreground mt-1 font-[family-name:var(--font-mono)]">{s.sub}</div>
            </StaggerItem>
          ))}
        </StaggerGroup>
      </div>
    </section>
  );
}

export function TheShift() {
  return (
    <section id="how" className="py-24 border-y border-border bg-card">
      <div className="max-w-6xl mx-auto px-6">
        <div className="grid lg:grid-cols-2 gap-10 items-end mb-14">
          <Reveal>
            <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
              The shift
            </div>
            <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight">
              Intent first.
              <br />
              <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">Action</em>{" "}
              second.
            </h2>
          </Reveal>
          <Reveal delay={0.1}>
            <p className="text-lg text-foreground-soft leading-relaxed">
              The hard part was never the alert — it&apos;s the <em>understanding.</em> Is this a real buyer, who
              are they, and what&apos;s the right move? LeadEcho gets that right first. Acting on the intent comes
              after, and only after it&apos;s real.
            </p>
          </Reveal>
        </div>

        <div className="text-center mb-8">
          <span className="inline-block rounded-full bg-accent-soft text-primary-ink text-xs font-[family-name:var(--font-mono)] px-4 py-1.5">
            discover · qualify · act · attribute — the closed loop
          </span>
        </div>

        <StaggerGroup className="grid md:grid-cols-3 lg:grid-cols-5 gap-4">
          {agents.map((a) => {
            const Icon = a.icon;
            return (
              <StaggerItem key={a.name}>
                <div className="h-full rounded-xl border border-border bg-background p-5 hover:shadow-sm transition-shadow">
                  <div className="text-[10px] font-[family-name:var(--font-mono)] text-muted-foreground uppercase tracking-wide">
                    {a.num}
                  </div>
                  <Icon className="h-5 w-5 mt-3 text-primary-ink" />
                  <h4 className="mt-3 font-[family-name:var(--font-head)] font-medium">{a.name}</h4>
                  <p className="mt-1.5 text-sm text-foreground-soft leading-relaxed">{a.desc}</p>
                  <div className="mt-3 flex flex-wrap gap-1">
                    {a.tools.map((t) => (
                      <span
                        key={t}
                        className="text-[10px] font-[family-name:var(--font-mono)] text-muted-foreground bg-muted rounded px-1.5 py-0.5"
                      >
                        {t}
                      </span>
                    ))}
                  </div>
                </div>
              </StaggerItem>
            );
          })}
        </StaggerGroup>

        <Reveal delay={0.15} className="mt-10">
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6 rounded-xl border border-border bg-background p-6">
            <div className="flex items-start gap-4">
              <div className="shrink-0 h-11 w-11 rounded-full bg-accent-soft flex items-center justify-center text-primary-ink">
                <GitBranch className="h-5 w-5" />
              </div>
              <div>
                <h4 className="font-[family-name:var(--font-head)] font-medium">Human-in-the-loop, by design.</h4>
                <p className="mt-1 text-sm text-foreground-soft leading-relaxed max-w-xl">
                  Drafts are posted via your real browser session through the Chrome extension — your account, your
                  click, never our headless infra.
                </p>
              </div>
            </div>
            <span className="shrink-0 inline-flex items-center gap-1.5 rounded-full bg-accent-soft text-primary-ink text-xs font-medium px-3 py-1.5">
              <span className="h-1.5 w-1.5 rounded-full bg-primary" /> No auto-poster, ever
            </span>
          </div>
        </Reveal>
      </div>
    </section>
  );
}
