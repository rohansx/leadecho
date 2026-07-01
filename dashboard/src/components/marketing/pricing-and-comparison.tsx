import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { motion, AnimatePresence } from "motion/react";
import { Check, Github, Minus } from "lucide-react";
import { Reveal, StaggerGroup, StaggerItem } from "./reveal";

type Period = "monthly" | "yearly";

const plans = [
  {
    name: "Free",
    for: "For trying it out",
    monthly: 0,
    yearly: 0,
    features: ["1 workspace", "Reddit + Hacker News", "3 pain-point profiles", "50 enriched leads / mo", "Person360 (basic)", "Chrome extension"],
    cta: "Get started",
    featured: false,
  },
  {
    name: "Starter",
    for: "For solo technical founders",
    monthly: 99,
    yearly: 79,
    features: ["All 7 platforms", "15 pain-point profiles", "500 enriched leads / mo", "Conversation agent (full)", "5 community playbooks", "Knowledge base RAG"],
    cta: "Start free trial",
    featured: true,
  },
  {
    name: "Pro",
    for: "For small revenue teams",
    monthly: 249,
    yearly: 199,
    features: ["Everything in Starter", "Unlimited leads", "HubSpot / Attio sync", "Custom playbooks", "Discovery agent (weekly)"],
    cta: "Start free trial",
    featured: false,
  },
  {
    name: "Team",
    for: "For 3+ seat GTM teams",
    monthly: 599,
    yearly: 479,
    features: ["Everything in Pro", "Up to 5 seats included", "Salesforce sync", "SSO & audit log", "Priority support · 4h SLA"],
    cta: "Book a demo",
    featured: false,
  },
];

const compareRows = [
  { cap: "Semantic pain-point matching", us: true, common: true, trigify: true },
  { cap: "Person360 / identity stitching", us: true, common: true, trigify: false },
  { cap: "Account ICP fit scoring", us: true, common: true, trigify: false },
  { cap: "Conversation agent (multi-turn)", us: true, common: false, trigify: false },
  { cap: "Per-style conversion attribution", us: true, common: false, trigify: false },
  { cap: "Self-hostable / open source", us: "MIT", common: false, trigify: false },
  { cap: "BYO AI keys (zero margin)", us: true, common: false, trigify: false },
  { cap: "Starting price", us: "Free / $99", common: "$500+/mo", trigify: "$150/mo" },
];

function Cell({ v }: { v: boolean | string }) {
  if (v === true) return <Check className="h-4 w-4 text-primary-ink mx-auto" />;
  if (v === false) return <Minus className="h-4 w-4 text-muted-foreground/50 mx-auto" />;
  return <span className="text-xs font-medium">{v}</span>;
}

export function PricingSection() {
  const [period, setPeriod] = useState<Period>("monthly");

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
            Or self-host for free, forever. All AI calls go to your own API key.
          </p>
        </Reveal>

        <Reveal delay={0.08} className="mt-8 flex justify-center">
          <div className="inline-flex rounded-lg border border-border bg-card p-1">
            {(["monthly", "yearly"] as const).map((p) => (
              <button
                key={p}
                onClick={() => setPeriod(p)}
                className={`px-4 py-1.5 rounded-md text-sm font-medium cursor-pointer transition-colors ${
                  period === p ? "bg-primary text-primary-foreground" : "text-foreground-soft hover:text-foreground"
                }`}
              >
                {p === "monthly" ? "Monthly" : "Yearly"}
                {p === "yearly" && <span className="ml-1.5 text-xs opacity-80">save 20%</span>}
              </button>
            ))}
          </div>
        </Reveal>

        <StaggerGroup className="mt-10 grid sm:grid-cols-2 lg:grid-cols-4 gap-5">
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
                <AnimatePresence mode="wait">
                  <motion.span
                    key={`${p.name}-${period}`}
                    initial={{ opacity: 0, y: -6 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: 6 }}
                    transition={{ duration: 0.2 }}
                    className="text-3xl font-[family-name:var(--font-head)] font-medium"
                  >
                    {period === "monthly" ? p.monthly : p.yearly}
                  </motion.span>
                </AnimatePresence>
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
                to="/inbox"
                className={`mt-6 inline-flex items-center justify-center rounded-lg px-4 py-2.5 text-sm font-medium transition-colors ${
                  p.featured
                    ? "bg-primary text-primary-foreground hover:bg-primary-hover shadow-sm"
                    : "border border-border hover:bg-accent"
                }`}
              >
                {p.cta}
              </Link>
            </StaggerItem>
          ))}
        </StaggerGroup>

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
              href="https://github.com/your-org/leadecho"
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
      <div className="max-w-5xl mx-auto px-6">
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
