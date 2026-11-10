import { Section, SectionHead } from "./section";

const features = [
  { title: "Set up from your URL", desc: "Paste your product's website. LeadEcho writes your pain points, keywords and communities to watch." },
  { title: "Replies grounded in your docs", desc: "Upload docs, FAQs and positioning. Drafts quote your real product, not invented features." },
  { title: "Chrome side panel", desc: "See leads and drafts next to the thread you're reading, and post from your own logged-in account." },
  { title: "Alerts where you work", desc: "High-intent leads go to Slack, Discord or email the moment they're scored." },
  { title: "CRM sync", desc: "Leads and conversions flow into HubSpot, Attio, or any signed webhook." },
  { title: "Your own AI keys", desc: "Use Anthropic, OpenAI, Gemini, OpenRouter or a local model. Each stage can use a different one." },
];

export function FeaturesGrid() {
  return (
    <Section id="features">
      <SectionHead title="Also in the box" />
      <dl className="mt-12 grid gap-x-10 sm:grid-cols-2 lg:grid-cols-3">
        {features.map((f) => (
          <div key={f.title} className="border-t border-border py-6">
            <dt className="text-lg font-semibold tracking-[-0.01em]">{f.title}</dt>
            <dd className="mt-2 text-[15px] text-foreground-soft">{f.desc}</dd>
          </div>
        ))}
      </dl>
    </Section>
  );
}
