import { Link } from "@tanstack/react-router";
import { motion } from "motion/react";
import { ArrowRight, Star } from "lucide-react";

const EASE = [0.16, 1, 0.3, 1] as const;

function RadarBackdrop() {
  const rings = [120, 220, 320, 420, 520];
  return (
    <div
      aria-hidden="true"
      className="absolute inset-0 -z-10 overflow-hidden [mask-image:radial-gradient(circle_at_75%_45%,black,transparent_70%)]"
    >
      <svg
        viewBox="0 0 1200 1200"
        className="absolute right-[-15%] top-1/2 -translate-y-1/2 w-[1100px] h-[1100px] text-primary/25"
        fill="none"
        stroke="currentColor"
      >
        {rings.map((r, i) => (
          <circle
            key={r}
            cx="600"
            cy="600"
            r={r}
            strokeDasharray={i === rings.length - 1 ? "3 8" : undefined}
          />
        ))}
      </svg>
      <motion.div
        className="absolute right-[-15%] top-1/2 -translate-y-1/2 w-[1100px] h-[1100px] origin-center"
        animate={{ rotate: 360 }}
        transition={{ duration: 8, repeat: Infinity, ease: "linear" }}
      >
        <div className="absolute inset-0 rounded-full bg-[conic-gradient(from_0deg,transparent_0deg,color-mix(in_oklab,var(--color-primary)_35%,transparent)_18deg,transparent_40deg)]" />
      </motion.div>
      {[
        { top: "38%", left: "62%", delay: 0 },
        { top: "58%", left: "70%", delay: 0.8 },
        { top: "48%", left: "80%", delay: 1.6 },
      ].map((b, i) => (
        <motion.div
          key={i}
          className="absolute h-2 w-2 rounded-full bg-primary"
          style={{ top: b.top, left: b.left }}
          animate={{ opacity: [0, 1, 0], scale: [0.6, 1.4, 0.6] }}
          transition={{ duration: 2.4, repeat: Infinity, delay: b.delay, ease: "easeInOut" }}
        />
      ))}
    </div>
  );
}

const journey = [
  {
    step: "01 · INGEST",
    name: "SCOUT",
    time: "0:00",
    done: true,
    body: (
      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
        <div className="flex items-center gap-2 text-xs text-muted-foreground mb-2">
          <span className="font-semibold text-orange-600">r/</span>
          <b className="text-foreground">r/saas</b>
          <span>·</span>
          <span>u/build_in_public</span>
          <span className="ml-auto">12m ago</span>
        </div>
        <div className="font-medium text-sm mb-1.5">Recs for monitoring Reddit at scale?</div>
        <p className="text-sm text-foreground-soft leading-relaxed">
          I need something that{" "}
          <mark className="bg-accent-soft text-foreground rounded px-0.5">
            monitors Reddit for mentions of my SaaS
          </mark>{" "}
          without hitting API rate limits. Most tools give me{" "}
          <mark className="bg-accent-soft text-foreground rounded px-0.5">keyword noise</mark>. Budget:{" "}
          <mark className="bg-accent-soft text-foreground rounded px-0.5">$50–100/mo</mark>.
        </p>
      </div>
    ),
  },
  {
    step: "02 · SCORE",
    name: "TRIAGE",
    time: "0:02",
    done: true,
    body: (
      <div className="rounded-xl border border-border bg-card p-4 shadow-sm flex items-center gap-4">
        <div className="text-3xl font-[family-name:var(--font-head)] font-medium text-primary-ink">
          9.2<em className="ml-1 not-italic text-xs font-normal text-muted-foreground">intent</em>
        </div>
        <div className="flex flex-wrap gap-1.5 text-xs">
          <span className="rounded-full bg-muted px-2 py-1 text-muted-foreground">not spam</span>
          <span className="rounded-full bg-muted px-2 py-1 text-muted-foreground">semantic match</span>
          <span className="rounded-full bg-accent-soft px-2 py-1 text-primary-ink">buy_signal</span>
          <span className="rounded-full bg-accent-soft px-2 py-1 text-primary-ink">vendor aware</span>
        </div>
      </div>
    ),
  },
  {
    step: "03 · ENRICH",
    name: "RESEARCHER",
    time: "0:14",
    done: true,
    body: (
      <div className="rounded-xl border border-border bg-card p-4 shadow-sm flex items-center gap-3">
        <div className="h-10 w-10 shrink-0 rounded-full bg-primary/20 text-primary-ink flex items-center justify-center text-sm font-medium">
          DR
        </div>
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium">Daniel Reyes</div>
          <div className="text-xs text-muted-foreground truncate">Founder, Cursive Labs · ex-Stripe</div>
        </div>
        <div className="text-right shrink-0">
          <div className="text-lg font-[family-name:var(--font-head)] font-medium text-primary-ink">88%</div>
          <div className="text-[10px] text-muted-foreground uppercase tracking-wide">ICP fit</div>
        </div>
      </div>
    ),
  },
  {
    step: "04 · ENGAGE",
    name: "CONVERSATION",
    time: "0:23",
    done: false,
    body: (
      <div className="rounded-xl border border-primary/40 bg-card p-4 shadow-md ring-1 ring-primary/10">
        <div className="text-xs text-muted-foreground mb-2 flex items-center gap-2">
          Draft ready · awaiting you
          <span className="rounded-full bg-accent-soft px-2 py-0.5 text-primary-ink text-[10px]">storytelling</span>
        </div>
        <p className="text-sm text-foreground-soft leading-relaxed mb-3">
          &ldquo;Hey — I built LeadEcho partly because I was hitting the exact API issue you describe. It&apos;s
          MIT and runs on docker compose, so the only cost is your own AI keys.&rdquo;
        </p>
        <div className="flex gap-2 text-xs">
          <span className="rounded-lg bg-primary text-primary-foreground px-3 py-1.5 font-medium">
            Approve &amp; post →
          </span>
          <span className="rounded-lg border border-border px-3 py-1.5 text-foreground-soft">Edit</span>
          <span className="rounded-lg border border-border px-3 py-1.5 text-foreground-soft">Skip</span>
        </div>
      </div>
    ),
  },
];

export function Hero() {
  return (
    <section className="relative overflow-hidden">
      <RadarBackdrop />
      <div className="max-w-6xl mx-auto px-6 pt-20 pb-24 grid lg:grid-cols-[1.1fr_1fr] gap-16 items-start">
        <div>
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, ease: EASE }}
            className="flex flex-wrap items-center gap-2"
          >
            <span className="inline-flex items-center gap-1.5 rounded-full bg-accent-soft text-primary-ink text-xs font-medium px-3 py-1">
              ⚡ Open source
            </span>
            <span className="inline-flex items-center gap-1.5 rounded-full border border-border text-xs text-foreground-soft px-3 py-1">
              <span className="h-1.5 w-1.5 rounded-full bg-primary" /> Self-hostable · BYOK
            </span>
          </motion.div>

          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.08, ease: EASE }}
            className="mt-6 text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]"
          >
            Buying-intent engine · for technical founders
          </motion.div>

          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7, delay: 0.16, ease: EASE }}
            className="mt-4 font-[family-name:var(--font-head)] font-medium tracking-tight leading-[1.05] text-5xl lg:text-6xl"
          >
            Find real{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">
              buying intent
            </em>{" "}
            where your customers live.
          </motion.div>

          <motion.p
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.24, ease: EASE }}
            className="mt-6 text-lg text-foreground-soft leading-relaxed max-w-xl"
          >
            Your buyers are describing their pain right now — on Reddit, Hacker News, LinkedIn and a dozen dev
            communities. LeadEcho <b className="text-foreground">finds it</b>, tells you{" "}
            <b className="text-foreground">who&apos;s behind it</b>, and helps you{" "}
            <b className="text-foreground">act</b> before anyone else does.
          </motion.p>

          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.32, ease: EASE }}
            className="mt-8 flex flex-wrap gap-3"
          >
            <Link
              to="/inbox"
              className="inline-flex items-center gap-2 rounded-lg bg-primary text-primary-foreground px-6 py-3 font-medium shadow-sm hover:bg-primary-hover transition-colors"
            >
              Get started <ArrowRight className="h-4 w-4" />
            </Link>
            <a
              href="https://github.com/your-org/leadecho"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-lg border border-border px-6 py-3 font-medium hover:bg-accent transition-colors"
            >
              <Star className="h-4 w-4" /> Star on GitHub
            </a>
          </motion.div>

          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.6, delay: 0.4 }}
            className="mt-8 flex flex-wrap gap-x-6 gap-y-2 text-sm text-muted-foreground font-[family-name:var(--font-mono)]"
          >
            {["docker compose up", "your own infra", "MIT licensed"].map((m) => (
              <span key={m} className="inline-flex items-center gap-1.5">
                <span className="h-1.5 w-1.5 rounded-full bg-primary" /> {m}
              </span>
            ))}
          </motion.div>
        </div>

        <div className="space-y-4" aria-label="How a single post flows through the agents">
          {journey.map((j, i) => (
            <motion.div
              key={j.name}
              initial={{ opacity: 0, x: 24 }}
              whileInView={{ opacity: 1, x: 0 }}
              viewport={{ once: true, margin: "-40px" }}
              transition={{ duration: 0.5, delay: i * 0.12, ease: EASE }}
            >
              <div className="flex items-center gap-2 mb-1.5 text-[11px] font-[family-name:var(--font-mono)] text-muted-foreground">
                <span>{j.step}</span>
                <span className="font-semibold text-foreground-soft tracking-wide">{j.name}</span>
                <span className="ml-auto">{j.time}</span>
              </div>
              {j.body}
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  );
}
