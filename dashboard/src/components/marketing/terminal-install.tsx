import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { motion, AnimatePresence } from "motion/react";
import { ArrowRight, Star } from "lucide-react";
import { Reveal } from "./reveal";

const commands: Record<string, string[]> = {
  docker: [
    "$ git clone https://github.com/your-org/leadecho.git",
    "$ cd leadecho",
    "$ cp .env.example .env   # add your AI keys",
    "$ docker compose up -d",
    "✓ scout · triage · researcher · conversation · attribution · discovery running",
  ],
  manual: [
    "$ git clone https://github.com/your-org/leadecho.git",
    "$ cd leadecho/backend && go run ./cmd/api",
    "$ cd ../dashboard && pnpm install && pnpm dev",
    "✓ open http://localhost:5173",
  ],
};

export function TerminalInstall() {
  const [tab, setTab] = useState<"docker" | "manual">("docker");

  return (
    <section id="docs" className="py-24">
      <div className="max-w-6xl mx-auto px-6 grid lg:grid-cols-2 gap-12 items-center">
        <Reveal>
          <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
            Self-host
          </div>
          <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight">
            All six agents,
            <br />
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">
              one docker-compose.
            </em>
          </h2>
          <p className="mt-5 text-lg text-foreground-soft leading-relaxed">
            Scout, Triage, Researcher, Conversation, Attribution and Discovery all run as services in the same
            compose file. Your infra, your data, your AI keys.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <a
              href="https://github.com/your-org/leadecho"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-lg bg-primary text-primary-foreground px-6 py-3 font-medium shadow-sm hover:bg-primary-hover transition-colors"
            >
              <Star className="h-4 w-4" /> Star on GitHub <ArrowRight className="h-4 w-4" />
            </a>
            <Link
              to="/inbox"
              className="inline-flex items-center gap-2 rounded-lg border border-border px-6 py-3 font-medium hover:bg-accent transition-colors"
            >
              Read the docs
            </Link>
          </div>
        </Reveal>

        <Reveal delay={0.1}>
          <div className="rounded-xl border border-border bg-[#16130e] text-[#e6ded0] overflow-hidden shadow-lg">
            <div className="flex items-center gap-3 px-4 py-3 border-b border-white/10">
              <div className="flex gap-1.5">
                <span className="h-2.5 w-2.5 rounded-full bg-white/20" />
                <span className="h-2.5 w-2.5 rounded-full bg-white/20" />
                <span className="h-2.5 w-2.5 rounded-full bg-white/20" />
              </div>
              <div className="flex gap-1 ml-2">
                {(["docker", "manual"] as const).map((t) => (
                  <button
                    key={t}
                    onClick={() => setTab(t)}
                    className={`rounded px-2.5 py-1 text-xs font-[family-name:var(--font-mono)] cursor-pointer transition-colors ${
                      tab === t ? "bg-white/15 text-white" : "text-white/50 hover:text-white/80"
                    }`}
                  >
                    {t}
                  </button>
                ))}
              </div>
              <div className="ml-auto text-[11px] font-[family-name:var(--font-mono)] text-white/40">~/leadecho</div>
            </div>
            <div className="p-5 font-[family-name:var(--font-mono)] text-sm min-h-[180px]">
              <AnimatePresence mode="wait">
                <motion.div
                  key={tab}
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.2 }}
                >
                  {commands[tab].map((line, i) => (
                    <motion.div
                      key={line}
                      initial={{ opacity: 0, x: -6 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ duration: 0.3, delay: i * 0.12 }}
                      className={line.startsWith("✓") ? "text-primary-ink mt-2" : "text-[#e6ded0]/90 leading-7"}
                    >
                      {line}
                    </motion.div>
                  ))}
                </motion.div>
              </AnimatePresence>
            </div>
          </div>
        </Reveal>
      </div>
    </section>
  );
}
