import { motion } from "motion/react";
import { Reveal, StaggerGroup, StaggerItem } from "./reveal";

const funnel = [
  { label: "Scored posts", num: "2,184", sub: "7 platforms · 5-min interval" },
  { label: "Leads Ready", num: "73", sub: "passed all 4 stages" },
  { label: "Replies sent", num: "61", sub: "human-approved" },
  { label: "Signups", num: "14", sub: "23% reply → signup" },
  { label: "ARR added", num: "$4,872", sub: "attributed · 6 closed-won", revenue: true },
];

const replyStyles = [
  { label: "storytelling", pct: 31.4, width: 78 },
  { label: "technical", pct: 22.1, width: 58 },
  { label: "value-first", pct: 14.8, width: 42 },
  { label: "casual", pct: 8.6, width: 25, dim: true },
  { label: "contrarian", pct: 3.2, width: 12, dim: true },
];

const communities = [
  { label: "r/saas", pct: "$5,184", width: 92 },
  { label: "hackernews", pct: "$3,492", width: 62 },
  { label: "indiehackers", pct: "$2,712", width: 48 },
  { label: "twitter / x", pct: "$2,016", width: 36 },
  { label: "linkedin", pct: "$1,236", width: 22, dim: true },
];

function AttrBar({ label, pct, width, dim }: { label: string; pct: string | number; width: number; dim?: boolean }) {
  return (
    <div className="flex items-center gap-3 py-1.5">
      <span className="w-28 shrink-0 text-xs font-[family-name:var(--font-mono)] text-foreground-soft">{label}</span>
      <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
        <motion.div
          className="h-full rounded-full bg-primary"
          initial={{ width: 0 }}
          whileInView={{ width: `${width}%` }}
          viewport={{ once: true }}
          transition={{ duration: 0.9, ease: [0.16, 1, 0.3, 1] }}
        />
      </div>
      <span className={`w-16 text-right text-xs font-medium ${dim ? "text-muted-foreground" : ""}`}>
        {typeof pct === "number" ? `${pct}%` : pct}
      </span>
    </div>
  );
}

const stages = [
  { num: "01", name: "Spam filter", desc: "Drop bots, link-spam, self-promo with Haiku 4.5 in < 50ms.", stat: "in: 2,184", reject: "− 1,218 dropped" },
  { num: "02", name: "Semantic match", desc: "Voyage embeddings against your pain-point profiles.", stat: "in: 966", reject: "− 507 below threshold" },
  { num: "03", name: "LLM intent", desc: "Sonnet 4.5: buy_signal, complaint, recommendation_ask, info.", stat: "in: 459", reject: "− 386 below threshold" },
  { num: "04", name: "Conversion score", desc: "Awareness level: Problem → Solution → Vendor → Purchase Ready.", stat: "in: 73", reject: "→ Researcher" },
];

export function AttributionSection() {
  return (
    <section id="attribution" className="py-24">
      <div className="max-w-6xl mx-auto px-6">
        <Reveal>
          <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
            Attribution
          </div>
          <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight max-w-lg">
            Every reply tied to{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">
              real revenue.
            </em>
          </h2>
        </Reveal>

        <StaggerGroup className="mt-12 grid grid-cols-2 md:grid-cols-5 gap-4">
          {funnel.map((f) => (
            <StaggerItem
              key={f.label}
              className={`rounded-xl border p-4 ${f.revenue ? "border-primary/40 bg-accent-soft/50" : "border-border bg-card"}`}
            >
              <div className="text-xs text-muted-foreground">{f.label}</div>
              <div className="mt-1 text-2xl font-[family-name:var(--font-head)] font-medium">{f.num}</div>
              <div className="mt-1 text-[11px] text-muted-foreground">{f.sub}</div>
            </StaggerItem>
          ))}
        </StaggerGroup>

        <div className="mt-10 grid md:grid-cols-2 gap-8">
          <Reveal>
            <h5 className="text-sm font-medium mb-3">Reply style · conversion rate, last 30 days</h5>
            {replyStyles.map((r) => (
              <AttrBar key={r.label} {...r} />
            ))}
          </Reveal>
          <Reveal delay={0.1}>
            <h5 className="text-sm font-medium mb-3">Community · ARR contribution, last 90 days</h5>
            {communities.map((c) => (
              <AttrBar key={c.label} {...c} />
            ))}
          </Reveal>
        </div>
      </div>
    </section>
  );
}

export function PipelineFlowSection() {
  return (
    <section className="py-24 border-t border-border bg-card">
      <div className="max-w-6xl mx-auto px-6">
        <Reveal>
          <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
            Triage agent · zoomed in
          </div>
          <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight max-w-xl">
            Why ~95% of mentions never reach your{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">inbox.</em>
          </h2>
          <p className="mt-4 text-lg text-foreground-soft leading-relaxed max-w-2xl">
            The Triage agent runs a 4-stage funnel before anything reaches the Researcher. Most posts are filtered
            out at stage 1 in &lt;50ms — only the survivors trigger expensive LLM calls.
          </p>
        </Reveal>

        <StaggerGroup className="mt-10 grid md:grid-cols-4 gap-4">
          {stages.map((s) => (
            <StaggerItem key={s.num} className="rounded-xl border border-border bg-background p-5">
              <div className="text-[10px] font-[family-name:var(--font-mono)] text-muted-foreground uppercase tracking-wide">
                Stage {s.num}
              </div>
              <h4 className="mt-2 font-[family-name:var(--font-head)] font-medium">{s.name}</h4>
              <p className="mt-1.5 text-sm text-foreground-soft leading-relaxed">{s.desc}</p>
              <div className="mt-4 flex items-center justify-between text-xs font-[family-name:var(--font-mono)]">
                <span className="text-muted-foreground">{s.stat}</span>
                <span className={s.num === "04" ? "text-primary-ink font-medium" : "text-destructive"}>
                  {s.reject}
                </span>
              </div>
            </StaggerItem>
          ))}
        </StaggerGroup>
      </div>
    </section>
  );
}
