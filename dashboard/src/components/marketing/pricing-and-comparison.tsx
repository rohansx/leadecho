import { Link } from "@tanstack/react-router";
import { Check, Minus } from "lucide-react";
import { cn } from "@/lib/utils";
import { Section, SectionHead } from "./section";
import { btnPrimary, btnSecondary } from "./buttons";

// Must match backend/internal/billing/plans.go (prices, credits, keyword limits).
const plans = [
  {
    name: "Free",
    for: "Try it on the free sources",
    price: 0,
    features: ["300 credits a month", "3 active keywords", "Hacker News, Dev.to, Bluesky", "AI scoring and reply drafts", "Slack, Discord and email alerts"],
    cta: "Start free",
  },
  {
    name: "Starter",
    for: "For solo founders",
    price: 49,
    features: ["5,000 credits a month", "15 active keywords", "Adds X, Reddit and LinkedIn", "Ask LeadEcho chat", "Knowledge base for replies"],
    cta: "Choose Starter",
    featured: true,
  },
  {
    name: "Pro",
    for: "For small revenue teams",
    price: 149,
    features: ["20,000 credits a month", "50 active keywords", "Everything in Starter", "HubSpot and Attio sync", "Signed outbound webhooks"],
    cta: "Choose Pro",
  },
];

const compareRows: { cap: string; us: boolean | string; common: boolean | string; trigify: boolean | string }[] = [
  { cap: "Matches pain points by meaning", us: true, common: true, trigify: true },
  { cap: "Researches the person behind a post", us: true, common: true, trigify: false },
  { cap: "Scores fit with your customer profile", us: true, common: true, trigify: false },
  { cap: "Replies that read the whole thread", us: true, common: false, trigify: false },
  { cap: "Signups tracked per reply style", us: true, common: false, trigify: false },
  { cap: "Open source", us: "MIT", common: false, trigify: false },
  { cap: "Self-host with your own AI keys", us: true, common: false, trigify: false },
  { cap: "Starting price", us: "Free", common: "$500+/mo", trigify: "$150/mo" },
];

function Cell({ v }: { v: boolean | string }) {
  if (v === true) return <Check aria-label="Yes" className="mx-auto h-4 w-4" />;
  if (v === false) return <Minus aria-label="No" className="mx-auto h-4 w-4 text-muted-foreground/60" />;
  return <span className="text-[14px]">{v}</span>;
}

export function PricingSection() {
  return (
    <Section id="pricing">
      <SectionHead label="Pricing" title="Pay for what you scan." muted="Or self-host it for free.">
        AI, data and hosting are included and paid with credits. One credit scores one post or runs one paid search;
        a reply draft is five. Top up 1,000 credits for $10, and they never expire.
      </SectionHead>

      <div className="mt-10 grid gap-3 md:grid-cols-3">
        {plans.map((p) => (
          <div
            key={p.name}
            className={cn(
              "flex flex-col rounded-[18px] border bg-card p-6",
              p.featured ? "border-yellow-deep ring-4 ring-yellow/25" : "border-border",
            )}
          >
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-[19px] font-medium">{p.name}</h3>
              {p.featured && (
                <span className="rounded-full bg-yellow px-2.5 py-0.5 text-[12px] font-medium text-[#1c1a14]">Recommended</span>
              )}
            </div>
            <p className="text-[14px] text-muted-foreground">{p.for}</p>
            <p className="mt-6">
              <span className="text-[44px] leading-none font-light tracking-[-0.04em]">${p.price}</span>
              <span className="text-[14px] text-muted-foreground"> a month</span>
            </p>
            <ul className="mt-6 flex-1 space-y-2.5 text-[15px]">
              {p.features.map((f) => (
                <li key={f} className="flex gap-2.5 text-foreground-soft">
                  <Check className="mt-1 h-4 w-4 shrink-0 text-foreground" />
                  {f}
                </li>
              ))}
            </ul>
            <Link to="/register" className={`mt-8 ${p.featured ? btnPrimary : btnSecondary}`}>
              {p.cta}
            </Link>
          </div>
        ))}
      </div>
    </Section>
  );
}

export function ComparisonSection() {
  return (
    <Section id="compare">
      <SectionHead label="Comparison" title="Compared with Common Room and Trigify." />
      <div className="mt-10 overflow-x-auto rounded-[18px] border border-border bg-card">
        <table className="w-full min-w-[34rem] text-left text-[15px]">
          <thead>
            <tr className="border-b border-border">
              <th className="px-5 py-3.5 font-medium">Capability</th>
              <th className="w-28 bg-yellow/15 py-3.5 text-center font-medium">LeadEcho</th>
              <th className="w-28 py-3.5 text-center font-normal text-muted-foreground">Common Room</th>
              <th className="w-28 py-3.5 pr-2 text-center font-normal text-muted-foreground">Trigify</th>
            </tr>
          </thead>
          <tbody>
            {compareRows.map((r) => (
              <tr key={r.cap} className="border-b border-border last:border-0">
                <td className="px-5 py-3 text-foreground-soft">{r.cap}</td>
                <td className="bg-yellow/15 py-3 text-center font-medium">
                  <Cell v={r.us} />
                </td>
                <td className="py-3 text-center text-muted-foreground">
                  <Cell v={r.common} />
                </td>
                <td className="py-3 pr-2 text-center text-muted-foreground">
                  <Cell v={r.trigify} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Section>
  );
}
