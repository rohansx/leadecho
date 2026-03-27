import { useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';

const faqs = [
  {
    q: 'Does it work without API keys?',
    a: 'Reddit and Hacker News work completely without any API keys. For AI scoring you need to provide your own key from OpenAI, Anthropic, or a compatible provider. Twitter and LinkedIn require browser session cookies for authenticated access.',
  },
  {
    q: 'How is this different from keyword alerts?',
    a: "Keyword tools match exact strings. LeadEcho uses sentence embeddings to match by meaning against your pain-point profiles. Someone asking \"how do I stop losing clients to bigger agencies\" won't match a keyword alert for \"agency management software\" — but LeadEcho will score it as a hot lead if that's your ICP pain point.",
  },
  {
    q: 'How does the reply posting work?',
    a: 'The Chrome extension types replies using a human-mimicry engine: Gaussian keystroke timing (~80ms per character), punctuation pauses, and a 2–8 second pre-engagement delay. It uses the platform\'s own input system, so it behaves like a human typing. You review and approve every reply before posting.',
  },
  {
    q: 'What AI providers are supported?',
    a: 'LeadEcho is BYOK. It supports OpenAI, Anthropic, and any OpenAI-compatible endpoint (Groq, Together AI, local Ollama, etc.). Voyage AI is used for semantic matching, but a keyword-only fallback mode works without it.',
  },
  {
    q: 'How much does AI scoring cost?',
    a: 'Very little. Each mention uses one LLM call (~200 tokens). At 2,000 mentions/month with GPT-4o-mini, that\'s ~$0.05. Embeddings cost ~$0.002 per 1,000 mentions. Most users spend under $2/month in API costs.',
  },
  {
    q: 'Can I use it without the Chrome extension?',
    a: 'Yes. The backend monitors platforms on its own schedule. The extension adds passive signal collection and the reply queue side panel, but the core inbox, scoring, and AI drafts all work from the web dashboard alone.',
  },
  {
    q: 'Is LinkedIn monitoring reliable?',
    a: "LinkedIn is the hardest platform to monitor. LeadEcho uses Pinchtab with your own session cookie. For more stealth, the Pro tier includes Camoufox (fingerprint-spoofing Firefox). You'll need to occasionally refresh session cookies.",
  },
];

export default function FAQ() {
  const [openIndex, setOpenIndex] = useState<number | null>(null);

  return (
    <section className="faq" id="faq">
      <div className="container-sm">
        <div className="section-label section-label-bracket">FAQ</div>
        <div className="section-title">Common questions</div>
        <div className="faq-list">
          {faqs.map((item, i) => {
            const isOpen = openIndex === i;
            return (
              <div key={i} style={{ borderTop: '1px solid var(--border)', ...(i === faqs.length - 1 ? { borderBottom: '1px solid var(--border)' } : {}) }}>
                <button
                  onClick={() => setOpenIndex(isOpen ? null : i)}
                  style={{
                    width: '100%',
                    padding: '18px 0',
                    fontSize: '0.9rem',
                    fontWeight: 600,
                    cursor: 'pointer',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    color: isOpen ? 'var(--accent)' : 'var(--text)',
                    transition: 'color 0.2s',
                    fontFamily: 'var(--font-display)',
                    background: 'none',
                    border: 'none',
                    textAlign: 'left',
                  }}
                  onMouseEnter={(e) => { if (!isOpen) (e.currentTarget as HTMLElement).style.color = 'var(--accent)'; }}
                  onMouseLeave={(e) => { if (!isOpen) (e.currentTarget as HTMLElement).style.color = 'var(--text)'; }}
                >
                  <span>{item.q}</span>
                  <motion.span
                    animate={{ rotate: isOpen ? 45 : 0 }}
                    transition={{ duration: 0.25 }}
                    style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: '1.2rem',
                      color: 'var(--muted)',
                      flexShrink: 0,
                      marginLeft: '16px',
                    }}
                  >
                    +
                  </motion.span>
                </button>
                <AnimatePresence initial={false}>
                  {isOpen && (
                    <motion.div
                      key="content"
                      initial={{ height: 0, opacity: 0 }}
                      animate={{ height: 'auto', opacity: 1 }}
                      exit={{ height: 0, opacity: 0 }}
                      transition={{ duration: 0.35, ease: [0.22, 1, 0.36, 1] }}
                      style={{ overflow: 'hidden' }}
                    >
                      <p style={{ paddingBottom: '20px', color: 'var(--muted)', fontSize: '0.85rem', lineHeight: 1.7, maxWidth: '640px' }}>
                        {item.a}
                      </p>
                    </motion.div>
                  )}
                </AnimatePresence>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
