import { useState } from "react";
import { motion, AnimatePresence } from "motion/react";
import { Plus } from "lucide-react";
import { Reveal } from "./reveal";

const faqs = [
  {
    q: "How is this different from keyword alerts?",
    a: "Keyword alerts match strings and fire on everything. LeadEcho matches meaning, scores intent, tells you who's asking, and tracks what converts. You'd rather miss 5 mentions than wade through 50 false positives.",
  },
  {
    q: "Isn't this just another AI SDR?",
    a: "No. The crowded part is reply-drafting. The hard, valuable part is discovering and qualifying real intent across communities — that's our core. Drafting sits on top, and you always approve the send.",
  },
  {
    q: "Do you auto-post replies?",
    a: "No. We draft; you click post in the platform's real UI, in your own browser session. Human-in-the-loop by design — no platform-ToS risk.",
  },
  {
    q: "What AI providers do you support?",
    a: "BYOK across Anthropic, OpenAI, Gemini, OpenRouter, and local models via Ollama, routed per-stage. Self-hosting means you pay only your own API cost.",
  },
  {
    q: "Can I really self-host the whole thing?",
    a: "Yes. MIT licensed, docker compose up, full product, no feature gates and no seat tax. A managed cloud is coming for teams that don't want to run infra.",
  },
  {
    q: "How much does intent scoring cost in practice?",
    a: "At default settings — 7 communities, continuous scanning — typical workspaces land at $2–8/mo in inference. The cheap pass rejects ~95% of posts before any expensive call.",
  },
];

function FaqItem({ q, a, defaultOpen }: { q: string; a: string; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(!!defaultOpen);
  return (
    <div className="border-b border-border">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center justify-between gap-4 py-5 text-left cursor-pointer"
      >
        <span className="font-medium">{q}</span>
        <motion.span animate={{ rotate: open ? 45 : 0 }} transition={{ duration: 0.2 }} className="shrink-0">
          <Plus className="h-4 w-4 text-muted-foreground" />
        </motion.span>
      </button>
      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.25, ease: [0.16, 1, 0.3, 1] }}
            className="overflow-hidden"
          >
            <p className="pb-5 text-sm text-foreground-soft leading-relaxed max-w-2xl">{a}</p>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

export function FaqSection() {
  return (
    <section id="faq" className="py-24">
      <div className="max-w-3xl mx-auto px-6">
        <Reveal>
          <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
            FAQ
          </div>
          <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight">
            Common <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">questions.</em>
          </h2>
        </Reveal>

        <Reveal delay={0.1} className="mt-10">
          {faqs.map((f, i) => (
            <FaqItem key={f.q} q={f.q} a={f.a} defaultOpen={i === 0} />
          ))}
        </Reveal>
      </div>
    </section>
  );
}
