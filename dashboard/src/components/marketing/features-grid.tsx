import {
  ArrowRight,
  Search,
  Users,
  MessageSquare,
  Inbox,
  MonitorSmartphone,
  Link2,
  BookOpen,
  Zap,
} from "lucide-react";
import { Reveal, StaggerGroup, StaggerItem } from "./reveal";

const features = [
  { icon: ArrowRight, title: "Zero-config onboarding", desc: "Paste your product URL. The Discovery agent scrapes it, extracts pain points, sets up the pipeline." },
  { icon: Search, title: "Semantic pain matching", desc: "Voyage embeddings match by meaning, not keywords. Catches buyers describing problems in their own words." },
  { icon: Users, title: "Person360 enrichment", desc: "Stitches Reddit + GitHub + LinkedIn + X into one Person record. Account-level ICP fit, automatically." },
  { icon: MessageSquare, title: "Conversation agent", desc: "Drafts initial replies and follow-ups. Per-community playbooks. Escalates when intent crosses your threshold." },
  { icon: Inbox, title: "Three-queue inbox", desc: "Auto-flowing drafts · Escalations · Proposals. Approve in one click; everything else stays out of your way." },
  { icon: MonitorSmartphone, title: "Side-panel reply coach", desc: "Chrome extension posts via your real browser session — your account, your click. No headless infra." },
  { icon: Link2, title: "UTM → CRM attribution", desc: "Every reply gets a unique shortlink. Conversions flow into HubSpot or Attio, automatically." },
  { icon: BookOpen, title: "Knowledge base RAG", desc: "Drop in your docs, FAQs, positioning. Drafts are grounded in your real product — not hallucinated specs." },
  { icon: Zap, title: "BYOK LLM router", desc: "Sonnet · Haiku · GPT · Voyage · Groq · any OpenAI-compatible endpoint. Our infra never sees AI spend." },
];

export function FeaturesGrid() {
  return (
    <section id="features" className="py-24">
      <div className="max-w-6xl mx-auto px-6">
        <div className="grid lg:grid-cols-2 gap-10 items-end mb-14">
          <Reveal>
            <div className="text-xs uppercase tracking-[0.12em] text-muted-foreground font-[family-name:var(--font-mono)]">
              What you get
            </div>
            <h2 className="mt-3 font-[family-name:var(--font-head)] text-3xl lg:text-4xl font-medium tracking-tight">
              Everything in one{" "}
              <em className="font-[family-name:var(--font-serif)] italic font-normal text-primary-ink">
                open-source
              </em>{" "}
              stack.
            </h2>
          </Reveal>
          <Reveal delay={0.1}>
            <p className="text-lg text-foreground-soft leading-relaxed">
              No stitching together signal tools, enrichment tools, reply tools and attribution tools. The full GTM
              loop runs in one codebase you self-host.
            </p>
          </Reveal>
        </div>

        <StaggerGroup className="grid sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {features.map((f) => {
            const Icon = f.icon;
            return (
              <StaggerItem key={f.title}>
                <div className="h-full rounded-xl border border-border bg-card p-5 hover:shadow-sm transition-shadow">
                  <div className="h-9 w-9 rounded-lg bg-accent-soft text-primary-ink flex items-center justify-center">
                    <Icon className="h-4.5 w-4.5" />
                  </div>
                  <h4 className="mt-3.5 font-[family-name:var(--font-head)] font-medium">{f.title}</h4>
                  <p className="mt-1.5 text-sm text-foreground-soft leading-relaxed">{f.desc}</p>
                </div>
              </StaggerItem>
            );
          })}
        </StaggerGroup>
      </div>
    </section>
  );
}
