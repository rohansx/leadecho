import { motion } from "motion/react";
import { Reveal } from "./reveal";

const personRows = [
  { key: "Role", val: <><b>Founder &amp; engineer</b> · Cursive Labs (since 2024)</> },
  { key: "GitHub", val: <><span className="rounded bg-muted px-1.5 py-0.5 text-xs mr-1">danielreyes</span> 1.4k stars · <b>Python · TypeScript</b></> },
  { key: "LinkedIn", val: "SF Bay · ex-Stripe (3y) · ex-Lyft (2y)" },
  { key: "Reddit", val: <><span className="rounded bg-muted px-1.5 py-0.5 text-xs mr-1">u/build_in_public</span> 7.2k karma · active in r/saas, r/startups</> },
  { key: "Recent", val: <>Shipped <b>v2 of inbox classifier</b> 4d ago · 3 HN comments this week</> },
  { key: "Awareness", val: <span className="rounded-full bg-accent-soft text-primary-ink text-xs px-2 py-0.5">Vendor Aware · evaluating, has budget</span> },
];

const accountRows = [
  { key: "Industry", val: <><b>B2B SaaS</b> · developer tools, AI workflow</> },
  { key: "Size", val: "4 employees · 2 engineers · pre-seed stage" },
  { key: "Funding", val: "$420k angel round · Aug 2024" },
  { key: "Stack", val: (
    <div className="flex flex-wrap gap-1">
      {["Next.js 15", "Postgres", "Stripe", "OpenAI", "Vercel"].map((s) => (
        <span key={s} className="rounded bg-muted px-1.5 py-0.5 text-xs">{s}</span>
      ))}
    </div>
  ) },
  { key: "Signals", val: <>Built-in-public on X, MRR shared monthly · <b>$2.4k MRR</b></> },
  { key: "Pain", val: <>3 of last 5 posts reference <span className="rounded-full bg-accent-soft text-primary-ink text-xs px-2 py-0.5">rate limiting</span></> },
];

export function Person360Section() {
  return (
    <section id="person360" className="py-24">
      <div className="max-w-6xl mx-auto px-6">
        <div className="grid lg:grid-cols-2 gap-10 items-end mb-14">
          <Reveal>
            <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
              Person360
            </div>
            <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight">
              Know <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">who</em>{" "}
              asked,
              <br />
              not just <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">what</em>{" "}
              they asked.
            </h2>
          </Reveal>
          <Reveal delay={0.1}>
            <p className="text-lg text-foreground-soft leading-relaxed">
              The Researcher agent stitches one Reddit username into a real human with a GitHub history, a company,
              a tech stack, and an ICP fit score — before you open the lead.
            </p>
          </Reveal>
        </div>

        <div className="grid md:grid-cols-2 gap-6">
          <Reveal>
            <div className="rounded-xl border border-border bg-card p-6 h-full">
              <h5 className="text-xs font-medium mb-1">
                <span className="rounded-full bg-accent-soft text-primary-ink px-2.5 py-1">
                  <span className="inline-block h-1.5 w-1.5 rounded-full bg-primary mr-1.5" />
                  PERSON
                </span>{" "}
                <span className="text-muted-foreground font-normal ml-1">stitched in 6s</span>
              </h5>
              <div className="mt-4 text-2xl font-[family-name:var(--font-head)] font-medium">Daniel Reyes</div>
              <div className="text-sm text-muted-foreground mt-1">
                @build_in_public · 3 handles linked ·{" "}
                <span className="text-primary-ink">writing-style match 0.91</span>
              </div>

              <div className="mt-5 divide-y divide-border">
                {personRows.map((r) => (
                  <div key={r.key} className="flex items-start gap-4 py-2.5 text-sm">
                    <span className="w-20 shrink-0 text-muted-foreground">{r.key}</span>
                    <span className="text-foreground-soft">{r.val}</span>
                  </div>
                ))}
              </div>

              <div className="mt-5">
                <div className="flex items-center justify-between text-sm mb-1.5">
                  <span className="text-muted-foreground">ICP fit</span>
                  <b className="font-[family-name:var(--font-head)]">88%</b>
                </div>
                <div className="h-2 rounded-full bg-muted overflow-hidden">
                  <motion.div
                    className="h-full rounded-full bg-primary"
                    initial={{ width: 0 }}
                    whileInView={{ width: "88%" }}
                    viewport={{ once: true }}
                    transition={{ duration: 1, ease: [0.16, 1, 0.3, 1], delay: 0.2 }}
                  />
                </div>
              </div>
            </div>
          </Reveal>

          <Reveal delay={0.12}>
            <div className="rounded-xl border border-border bg-card p-6 h-full">
              <h5 className="text-xs font-medium mb-1">
                <span className="rounded-full bg-accent-soft text-primary-ink px-2.5 py-1">
                  <span className="inline-block h-1.5 w-1.5 rounded-full bg-primary mr-1.5" />
                  ACCOUNT
                </span>{" "}
                <span className="text-muted-foreground font-normal ml-1">enriched in 4s</span>
              </h5>
              <div className="mt-4 text-2xl font-[family-name:var(--font-head)] font-medium">Cursive Labs</div>
              <div className="text-sm text-muted-foreground mt-1">cursivelabs.dev · founded 2024</div>

              <div className="mt-5 divide-y divide-border">
                {accountRows.map((r) => (
                  <div key={r.key} className="flex items-start gap-4 py-2.5 text-sm">
                    <span className="w-20 shrink-0 text-muted-foreground">{r.key}</span>
                    <span className="text-foreground-soft">{r.val}</span>
                  </div>
                ))}
              </div>

              <div className="mt-5 rounded-lg bg-accent-soft/60 p-4">
                <div className="flex items-center justify-between text-sm">
                  <span className="text-muted-foreground">Recommended playbook</span>
                  <b className="font-[family-name:var(--font-head)]">storytelling · technical</b>
                </div>
                <div className="mt-2 text-xs text-foreground-soft font-[family-name:var(--font-mono)] leading-relaxed">
                  Founder-to-founder voice. Reference the API rate-limit pain by name. Link to install docs, not
                  pricing.
                </div>
              </div>
            </div>
          </Reveal>
        </div>
      </div>
    </section>
  );
}
