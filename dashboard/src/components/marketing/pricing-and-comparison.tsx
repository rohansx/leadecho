import { Link } from "@tanstack/react-router";
import { Check, Lock, Minus } from "lucide-react";
import { cn } from "@/lib/utils";
import { Section, Eyebrow, H2 } from "./section";
import { Reveal } from "./fx";
import { btnPrimary, btnSecondary } from "./buttons";

// Must match backend/internal/billing/plans.go (prices, credits, keyword limits).
const plans = [
  {
    name: "Free",
    for: "Try it on the free sources",
    price: 0,
    features: ["300 credits a month", "3 active keywords", "Hacker News, Dev.to, Bluesky", "AI scoring and reply drafts", "Slack, Discord and email alerts"],
    locked: ["X, Reddit and LinkedIn"],
    cta: "Start free",
  },
  {
    name: "Starter",
    for: "For solo founders",
    price: 49,
    features: ["5,000 credits a month", "15 active keywords", "Adds X, Reddit and LinkedIn", "Hiring signals: 3 companies checked a day (about 1 confirmed contact)", "Ask LeadEcho chat", "Knowledge base for replies"],
    cta: "Choose Starter",
    featured: true,
  },
  {
    name: "Pro",
    for: "For small revenue teams",
    price: 149,
    features: ["20,000 credits a month", "50 active keywords", "Everything in Starter", "Hiring signals: 8 companies checked a day (about 3 confirmed contacts)", "HubSpot and Attio sync", "Signed outbound webhooks"],
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
    <Section id="pricing" className="border-t border-border">
      <Reveal className="mx-auto max-w-2xl text-center">
        <Eyebrow>Pricing</Eyebrow>
        <H2 className="mt-4">Pay for what you scan, or self-host it for free.</H2>
        <p className="mx-auto mt-5 max-w-xl text-foreground-soft">
          AI, data and hosting are included and paid with credits. One credit scores one post or runs one paid
          search; a reply draft is five. Top up 1,000 credits for $10, and they never expire.
        </p>
      </Reveal>

      <div className="mt-14 grid gap-4 md:grid-cols-3">
        {plans.map((p, i) => (
          <Reveal
            key={p.name}
            delay={i * 0.06}
            className={cn(
              "flex flex-col rounded-[24px] border p-7",
              p.featured ? "border-foreground bg-card shadow-[0_24px_48px_-28px_rgba(24,24,27,0.35)]" : "border-border bg-card",
            )}
          >
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-[19px] font-semibold">{p.name}</h3>
              {p.featured && (
                <span className="rounded-full bg-chip-bg px-2.5 py-0.5 text-[12px] font-semibold text-chip-fg">Recommended</span>
              )}
            </div>
            <p className="text-[14px] text-muted-foreground">{p.for}</p>
            <p className="mt-7">
              <span className="text-[52px] leading-none font-semibold tracking-[-0.05em]">${p.price}</span>
              <span className="text-[14px] text-muted-foreground"> a month</span>
            </p>
            <ul className="mt-7 flex-1 space-y-2.5 text-[15px]">
              {p.features.map((f) => (
                <li key={f} className="flex gap-2.5 text-foreground-soft">
                  <Check className="mt-1 h-4 w-4 shrink-0 text-foreground" />
                  {f}
                </li>
              ))}
              {p.locked?.map((f) => (
                <li key={f} className="flex gap-2.5 text-muted-foreground">
                  <Lock className="mt-1 h-4 w-4 shrink-0" aria-hidden="true" />
                  <span>
                    {f} <span className="text-[13px]">(Starter and up)</span>
                  </span>
                </li>
              ))}
            </ul>
            <Link to="/register" className={cn("mt-8", p.featured ? btnPrimary : btnSecondary)}>
              {p.cta}
            </Link>
          </Reveal>
        ))}
      </div>

      <Reveal className="mt-14">
        <h3 className="text-[20px] font-semibold tracking-[-0.01em]">Compared with Common Room and Trigify</h3>
        <div className="mt-6 overflow-x-auto rounded-[20px] border border-border bg-card">
          <table className="w-full min-w-[34rem] text-left text-[15px]">
            <thead>
              <tr className="border-b border-border">
                <th className="px-6 py-4 font-medium text-muted-foreground">Capability</th>
                <th className="w-32 py-4 text-center font-semibold">LeadEcho</th>
                <th className="w-32 py-4 text-center font-normal text-muted-foreground">Common Room</th>
                <th className="w-32 py-4 pr-3 text-center font-normal text-muted-foreground">Trigify</th>
              </tr>
            </thead>
            <tbody>
              {compareRows.map((r) => (
                <tr key={r.cap} className="border-b border-border last:border-0">
                  <td className="px-6 py-3.5 text-foreground-soft">{r.cap}</td>
                  <td className="py-3.5 text-center font-semibold">
                    <Cell v={r.us} />
                  </td>
                  <td className="py-3.5 text-center text-muted-foreground">
                    <Cell v={r.common} />
                  </td>
                  <td className="py-3.5 pr-3 text-center text-muted-foreground">
                    <Cell v={r.trigify} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Reveal>
    </Section>
  );
}
