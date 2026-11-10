import { Bell, Globe, KeyRound, PanelRight, ShieldCheck, Workflow } from "lucide-react";
import { Section, Eyebrow, H2 } from "./section";
import { Reveal } from "./fx";

const extras = [
  { Icon: Globe, title: "Set up from your URL", desc: "Paste your website. LeadEcho writes your pain points, keywords and communities to watch." },
  { Icon: PanelRight, title: "Chrome side panel", desc: "Leads and drafts next to the thread you're reading, posted from your own account." },
  { Icon: Bell, title: "Alerts where you work", desc: "High-intent leads reach Slack, Discord or email the moment they're scored." },
  { Icon: Workflow, title: "CRM sync", desc: "Leads and conversions flow into HubSpot, Attio, or any signed webhook." },
  { Icon: KeyRound, title: "Your own AI keys", desc: "Anthropic, OpenAI, Gemini, OpenRouter or a local model, chosen per stage." },
  { Icon: ShieldCheck, title: "No scraping with your login", desc: "X, LinkedIn and Reddit are searched through an API. Your accounts are never used by a bot." },
];

export function Extras() {
  return (
    <Section className="pt-0 sm:pt-0">
      <Reveal className="max-w-2xl">
        <Eyebrow>Also included</Eyebrow>
        <H2 className="mt-4">The rest of the loop, built in. Even on the free plan.</H2>
      </Reveal>
      <div className="mt-12 grid gap-px overflow-hidden rounded-[24px] border border-border bg-border sm:grid-cols-2 lg:grid-cols-3">
        {extras.map(({ Icon, title, desc }, i) => (
          <Reveal key={title} delay={(i % 3) * 0.06} className="bg-card p-7">
            <Icon className="size-5 text-foreground" aria-hidden="true" />
            <h3 className="mt-6 text-[17px] font-semibold tracking-[-0.01em]">{title}</h3>
            <p className="mt-1.5 text-[15px] text-foreground-soft">{desc}</p>
          </Reveal>
        ))}
      </div>
    </Section>
  );
}
