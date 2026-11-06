import { Link } from "@tanstack/react-router";
import { Check, Github, Minus } from "lucide-react";
import { Reveal, StaggerGroup, StaggerItem } from "./reveal";
import { GITHUB_URL } from "@/lib/constants";

// Must match backend/internal/billing/plans.go (prices, credits, keyword limits).
const plans = [
  {
    name: "Free",
    for: "Try it on the free sources",
    price: 0,
    features: ["300 credits / mo", "3 active keywords", "Hacker News, Dev.to, Bluesky", "AI scoring + reply drafts", "Slack, Discord & email alerts"],
    cta: "Get started",
    featured: false,
  },
  {
    name: "Starter",
    for: "For solo founders",
    price: 49,
    features: ["5,000 credits / mo", "15 active keywords", "Adds X, Reddit, LinkedIn", "Ask LeadEcho (chat)", "Knowledge base for replies"],
    cta: "Choose Starter",
    featured: true,
  },
  {
    name: "Pro",
    for: "For small revenue teams",
    price: 149,
    features: ["20,000 credits / mo", "50 active keywords", "Everything in Starter", "HubSpot / Attio sync", "Signed outbound webhooks"],
    cta: "Choose Pro",
    featured: false,
  },
];

const compareRows = [
  { cap: "Semantic pain-point matching", us: true, common: true, trigify: true },
  { cap: "Person360 enrichment", us: true, common: true, trigify: false },
  { cap: "Account ICP fit scoring", us: true, common: true, trigify: false },
  { cap: "Thread-aware AI replies", us: true, common: false, trigify: false },
  { cap: "Per-style conversion attribution", us: true, common: false, trigify: false },
  { cap: "Self-hostable / open source", us: "MIT", common: false, trigify: false },
  { cap: "Self-host with your own AI keys", us: true, common: false, trigify: false },
  { cap: "Starting price", us: "Free / $49", common: "$500+/mo", trigify: "$150/mo" },
];

function Cell({ v }: { v: boolean | string }) {
  if (v === true) return <Check className="h-4 w-4 text-primary-ink mx-auto" />;
  if (v === false) return <Minus className="h-4 w-4 text-muted-foreground/50 mx-auto" />;
  return <span className="text-xs font-medium">{v}</span>;
}

export function PricingSection() {
  return (
    <section id="pricing" className="py-24">
      <div className="max-w-6xl mx-auto px-6">
        <Reveal>
          <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
            Pricing
          </div>
          <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight max-w-lg">
            Cheaper than one Common Room{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">seat.</em>
          </h2>
          <p className="mt-4 text-lg text-foreground-soft leading-relaxed max-w-xl">
            AI, scraping and hosting included. Pay for what you use with credits, or self-host free forever.
          </p>
        </Reveal>

        <StaggerGroup className="mt-10 grid sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {plans.map((p) => (
            <StaggerItem
              key={p.name}
              className={`relative rounded-xl border p-6 flex flex-col ${
                p.featured ? "border-primary/50 shadow-md bg-card" : "border-border bg-card"
              }`}
            >
              {p.featured && (
                <span className="absolute -top-3 left-6 rounded-full bg-primary text-primary-foreground text-[10px] font-medium px-2.5 py-1">
                  Most popular
                </span>
              )}
              <h4 className="font-[family-name:var(--font-head)] font-medium text-lg">{p.name}</h4>
              <div className="text-xs text-muted-foreground mt-1">{p.for}</div>
              <div className="mt-4 flex items-baseline gap-1">
                <span className="text-lg">$</span>
                <span className="text-3xl font-[family-name:var(--font-head)] font-medium">{p.price}</span>
                <span className="text-sm text-muted-foreground"> / month</span>
              </div>
              <ul className="mt-5 space-y-2 text-sm flex-1">
                {p.features.map((f) => (
                  <li key={f} className="flex items-start gap-2 text-foreground-soft">
                    <Check className="h-3.5 w-3.5 text-primary-ink mt-0.5 shrink-0" />
                    {f}
                  </li>
                ))}
              </ul>
              <Link
                to="/register"
                className={`mt-6 inline-flex items-center justify-center rounded-lg px-4 py-2.5 text-sm font-medium transition-colors ${
                  p.featured
                    ? "bg-foreground text-background hover:bg-foreground/85 shadow-sm"
                    : "border border-border hover:bg-accent"
                }`}
              >
                {p.cta}
              </Link>
            </StaggerItem>
          ))}
        </StaggerGroup>

        <p className="mt-6 text-sm text-muted-foreground max-w-3xl">
          1 credit = 1 post scored by AI or 1 paid search (X, Reddit, LinkedIn); a reply draft is 5. Hacker News, Dev.to
          and Bluesky searches are free. Need more? Top up 1,000 credits for $10 — they never expire.
        </p>

        <Reveal delay={0.1} className="mt-8">
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6 rounded-xl border border-border bg-card p-6">
            <div>
              <h4 className="font-[family-name:var(--font-head)] font-medium">
                Or self-host the whole thing, free forever.
              </h4>
              <p className="mt-1 text-sm text-foreground-soft max-w-xl">
                Docker-compose, MIT license, your infra, your AI keys. Full product, no feature gates, no seat tax.
              </p>
            </div>
            <a
              href={GITHUB_URL}
              target="_blank"
              rel="noreferrer"
              className="shrink-0 inline-flex items-center gap-2 rounded-lg border border-border px-5 py-2.5 text-sm font-medium hover:bg-accent transition-colors"
            >
              <Github className="h-4 w-4" /> View on GitHub
            </a>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

export function ComparisonSection() {
  return (
    <section className="py-24 border-t border-border bg-card">
      <div className="max-w-6xl mx-auto px-6">
        <Reveal>
          <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
            Comparison
          </div>
          <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight">
            How LeadEcho{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">compares.</em>
          </h2>
        </Reveal>

        <Reveal delay={0.1} className="mt-10 overflow-x-auto rounded-xl border border-border">
          <table className="w-full text-sm bg-background">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left font-medium p-4">Capability</th>
                <th className="p-4 text-primary-ink font-medium">LeadEcho</th>
                <th className="p-4 font-medium text-muted-foreground">Common Room</th>
                <th className="p-4 font-medium text-muted-foreground">Trigify</th>
              </tr>
            </thead>
            <tbody>
              {compareRows.map((r) => (
                <tr key={r.cap} className="border-b border-border last:border-0">
                  <td className="p-4 text-foreground-soft">{r.cap}</td>
                  <td className="p-4 text-center bg-accent-soft/30">
                    <Cell v={r.us} />
                  </td>
                  <td className="p-4 text-center">
                    <Cell v={r.common} />
                  </td>
                  <td className="p-4 text-center">
                    <Cell v={r.trigify} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Reveal>
      </div>
    </section>
  );
}
