import { Link } from "@tanstack/react-router";
import { motion } from "motion/react";
import { ArrowRight, Github, MessageCircle, BookOpen, Star } from "lucide-react";
import { Reveal, StaggerGroup, StaggerItem } from "./reveal";
import { Logo } from "./logo";

const community = [
  { icon: Github, title: "GitHub", desc: "See our complete codebase, issues, and the agents in action.", cta: "Star (MIT)" },
  { icon: MessageCircle, title: "Community", desc: "Get support, share your playbooks, swap reply styles with other founders.", cta: "Join today" },
  { icon: BookOpen, title: "Docs", desc: "Agent architecture, setup guide, LLM router reference, playbooks.", cta: "Browse" },
];

export function CommunitySection() {
  return (
    <section className="py-20">
      <div className="max-w-6xl mx-auto px-6">
        <StaggerGroup className="grid sm:grid-cols-3 gap-6">
          {community.map((c) => {
            const Icon = c.icon;
            return (
              <StaggerItem key={c.title} className="rounded-xl border border-border bg-card p-6 text-center">
                <Icon className="h-6 w-6 mx-auto text-primary-ink" />
                <h4 className="mt-3 font-[family-name:var(--font-head)] font-medium">{c.title}</h4>
                <p className="mt-1.5 text-sm text-foreground-soft leading-relaxed">{c.desc}</p>
                <a
                  href="https://github.com/your-org/leadecho"
                  target="_blank"
                  rel="noreferrer"
                  className="mt-3 inline-flex items-center gap-1 text-sm text-primary-ink font-medium hover:underline"
                >
                  {c.cta} <ArrowRight className="h-3.5 w-3.5" />
                </a>
              </StaggerItem>
            );
          })}
        </StaggerGroup>
      </div>
    </section>
  );
}

export function FinalCta() {
  return (
    <section className="relative py-28 overflow-hidden border-t border-border bg-card">
      <svg
        aria-hidden="true"
        viewBox="0 0 800 800"
        className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[900px] h-[900px] text-primary/10"
        fill="none"
        stroke="currentColor"
      >
        {[80, 160, 240, 320].map((r) => (
          <circle key={r} cx="400" cy="400" r={r} />
        ))}
        <circle cx="400" cy="400" r="380" strokeDasharray="3 8" />
      </svg>
      <div className="relative max-w-3xl mx-auto px-6 text-center">
        <Reveal>
          <div className="inline-flex items-center gap-1.5 rounded-full bg-accent-soft text-primary-ink text-xs font-medium px-3 py-1">
            The right conversation is happening right now
          </div>
          <h2 className="mt-5 font-[family-name:var(--font-head)] text-4xl lg:text-5xl font-medium tracking-tight leading-[1.05]">
            Someone&apos;s describing
            <br />
            your problem.{" "}
            <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">In public.</em>
          </h2>
          <p className="mt-5 text-xl text-foreground-soft leading-relaxed">
            Someone will answer them today. With the right context and the right reply, it should be you.
          </p>
          <div className="mt-8 flex flex-wrap justify-center gap-3">
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
          </div>
          <p className="mt-6 text-xs text-muted-foreground font-[family-name:var(--font-mono)] tracking-wide">
            Open source · self-hostable · BYOK
          </p>
        </Reveal>
      </div>
    </section>
  );
}

export function MarketingFooter() {
  return (
    <footer className="py-14 border-t border-border">
      <div className="max-w-6xl mx-auto px-6">
        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-10">
          <div>
            <Logo />
            <p className="mt-3 text-sm text-muted-foreground max-w-[220px]">
              Find real buying intent across the communities your customers live in.
            </p>
          </div>
          <div>
            <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-3">Product</h5>
            <ul className="space-y-2 text-sm">
              <li><a href="#how" className="text-foreground-soft hover:text-foreground transition-colors">How it works</a></li>
              <li><a href="#person360" className="text-foreground-soft hover:text-foreground transition-colors">Person360</a></li>
              <li><a href="#pricing" className="text-foreground-soft hover:text-foreground transition-colors">Pricing</a></li>
            </ul>
          </div>
          <div>
            <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-3">Resources</h5>
            <ul className="space-y-2 text-sm">
              <li><a href="#docs" className="text-foreground-soft hover:text-foreground transition-colors">Docs</a></li>
              <li><a href="#faq" className="text-foreground-soft hover:text-foreground transition-colors">FAQ</a></li>
              <li><Link to="/privacy" className="text-foreground-soft hover:text-foreground transition-colors">Privacy</Link></li>
            </ul>
          </div>
          <div>
            <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-3">Social</h5>
            <ul className="space-y-2 text-sm">
              <li><a href="https://github.com/your-org/leadecho" target="_blank" rel="noreferrer" className="text-foreground-soft hover:text-foreground transition-colors">GitHub</a></li>
            </ul>
          </div>
        </div>
        <div className="mt-10 pt-6 border-t border-border flex flex-wrap items-center justify-between gap-4 text-xs text-muted-foreground">
          <span>© 2026 LeadEcho · MIT License</span>
          <motion.div whileHover={{ scale: 1.02 }} className="flex items-center gap-3">
            <span className="rounded bg-muted px-2 py-1">MIT</span>
            <span className="rounded bg-muted px-2 py-1">Open Source</span>
          </motion.div>
        </div>
      </div>
    </footer>
  );
}
