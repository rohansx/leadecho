import { Plus } from "lucide-react";
import { Section, SectionHead } from "./section";

const faqs = [
  {
    q: "How is this different from keyword alerts?",
    a: "Keyword alerts match strings and fire on everything. LeadEcho matches meaning, scores intent, tells you who is asking, and tracks which replies convert. You'd rather see 5 real buyers than wade through 50 false positives.",
  },
  {
    q: "Do you post replies automatically?",
    a: "No. LeadEcho drafts the reply; you click post in the platform's own page, in your own browser session. A person approves every reply.",
  },
  {
    q: "Which AI providers can I use?",
    a: "Anthropic, OpenAI, Gemini, OpenRouter, and local models through Ollama. Each stage of the pipeline can use a different one.",
  },
  {
    q: "Can I really self-host the whole thing?",
    a: "Yes. It's MIT licensed and starts with docker compose. It's the full product, with no feature gates and no per-seat pricing.",
  },
  {
    q: "What does intent scoring cost to run?",
    a: "With default settings, most self-hosted workspaces spend $2–8 a month on AI. The cheap first pass removes about 95% of posts before any expensive model runs.",
  },
];

export function FaqSection() {
  return (
    <Section id="faq">
      <SectionHead label="Questions" title="Common questions." />
      <div className="mt-8 overflow-hidden rounded-[18px] border border-border bg-card">
        {faqs.map((f, i) => (
          <details key={f.q} open={i === 0} className="group border-t border-border first:border-t-0">
            <summary className="flex cursor-pointer list-none items-center justify-between gap-4 px-6 py-5 text-[17px] font-medium [&::-webkit-details-marker]:hidden">
              {f.q}
              <Plus className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-45" />
            </summary>
            <p className="max-w-2xl px-6 pb-6 text-[15px] leading-relaxed text-foreground-soft">{f.a}</p>
          </details>
        ))}
      </div>
    </Section>
  );
}
