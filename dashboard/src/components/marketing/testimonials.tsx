import { Reveal, StaggerGroup, StaggerItem } from "./reveal";

const testimonials = [
  {
    quote:
      "The Researcher agent surfaced a $4,200 ARR customer whose Reddit post never even mentioned my product category. ICP scoring caught what keyword alerts missed for six months.",
    name: "Jake R.",
    role: "Founder · DevOps SaaS",
    initials: "JR",
  },
  {
    quote:
      "We dropped Common Room and replaced it with a self-hosted LeadEcho. Same workflow, $500/mo savings, and we own the data.",
    name: "Sarah M.",
    role: "Head of Growth · Analytics Startup",
    initials: "SM",
  },
  {
    quote:
      "I approve drafts on my phone between meetings. The agent does the research, the writing, and the attribution.",
    name: "Priya K.",
    role: "Solo Founder",
    initials: "PK",
  },
  {
    quote:
      "Per-style conversion-rate data is the unlock. I now know storytelling beats technical 2× in r/SaaS and the reverse on HN.",
    name: "Marcus T.",
    role: "Developer Advocate",
    initials: "MT",
  },
];

export function Testimonials() {
  return (
    <section className="py-24 border-t border-border bg-card">
      <div className="max-w-6xl mx-auto px-6">
        <Reveal>
          <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
            From the wild
          </div>
          <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight max-w-xl">
            Built by founders using it on{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">themselves.</em>
          </h2>
        </Reveal>

        <StaggerGroup className="mt-12 grid sm:grid-cols-2 gap-6">
          {testimonials.map((t) => (
            <StaggerItem key={t.name} className="rounded-xl border border-border bg-background p-6">
              <blockquote className="text-sm leading-relaxed text-foreground-soft">&ldquo;{t.quote}&rdquo;</blockquote>
              <div className="mt-4 flex items-center gap-3">
                <div className="h-9 w-9 rounded-full bg-accent-soft text-primary-ink flex items-center justify-center text-xs font-medium">
                  {t.initials}
                </div>
                <div>
                  <div className="text-sm font-medium">{t.name}</div>
                  <div className="text-xs text-muted-foreground">{t.role}</div>
                </div>
              </div>
            </StaggerItem>
          ))}
        </StaggerGroup>
      </div>
    </section>
  );
}
