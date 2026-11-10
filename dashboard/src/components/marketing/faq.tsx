import { Plus } from "lucide-react";
import { Section, Eyebrow, H2 } from "./section";
import { Reveal } from "./fx";

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
      <div className="grid gap-10 lg:grid-cols-[0.8fr_1.2fr] lg:gap-20">
        <Reveal>
          <Eyebrow>Questions</Eyebrow>
          <H2 className="mt-4">Answers before you ask.</H2>
        </Reveal>
        <Reveal delay={0.08} className="border-t border-border">
          {faqs.map((f, i) => (
            <details key={f.q} open={i === 0} className="group border-b border-border">
              <summary className="flex cursor-pointer list-none items-center justify-between gap-4 py-5 text-[18px] font-semibold tracking-[-0.01em] [&::-webkit-details-marker]:hidden">
                {f.q}
                <span className="flex size-7 shrink-0 items-center justify-center rounded-full border border-border transition-transform group-open:rotate-45">
                  <Plus className="h-3.5 w-3.5" />
                </span>
              </summary>
              <p className="max-w-2xl pb-6 text-[16px] leading-relaxed text-foreground-soft">{f.a}</p>
            </details>
          ))}
        </Reveal>
      </div>
    </Section>
  );
}
