import { Bell, BookOpen, Globe, KeyRound, PanelRight, Workflow } from "lucide-react";
import { Section, SectionHead } from "./section";

const features = [
  { Icon: Globe, title: "Set up from your URL", desc: "Paste your product's website. LeadEcho writes your pain points, keywords and communities to watch." },
  { Icon: BookOpen, title: "Replies grounded in your docs", desc: "Upload docs, FAQs and positioning. Drafts quote your real product, not invented features." },
  { Icon: PanelRight, title: "Chrome side panel", desc: "See leads and drafts next to the thread you're reading, and post from your own logged-in account." },
  { Icon: Bell, title: "Alerts where you work", desc: "High-intent leads go to Slack, Discord or email the moment they're scored." },
  { Icon: Workflow, title: "CRM sync", desc: "Leads and conversions flow into HubSpot, Attio, or any signed webhook." },
  { Icon: KeyRound, title: "Your own AI keys", desc: "Use Anthropic, OpenAI, Gemini, OpenRouter or a local model. Each stage can use a different one." },
];

export function FeaturesGrid() {
  return (
    <Section id="features">
      <SectionHead label="Everything else" title="The rest of the loop is built in." muted="Even on the free plan." />
      <div className="mt-10 grid gap-3 sm:grid-cols-2">
        {features.map(({ Icon, title, desc }) => (
          <div key={title} className="rounded-[18px] border border-border bg-card p-6">
            <span className="flex size-8 items-center justify-center rounded-[9px] bg-muted text-foreground-soft">
              <Icon className="size-4" aria-hidden="true" />
            </span>
            <h3 className="mt-5 text-[18px] font-medium tracking-[-0.01em]">{title}</h3>
            <p className="mt-1.5 text-[15px] text-foreground-soft">{desc}</p>
          </div>
        ))}
      </div>
    </Section>
  );
}
