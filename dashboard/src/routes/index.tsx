import { createFileRoute, Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Text } from "@/components/ui/text";
import { Badge } from "@/components/ui/badge";
import {
  Inbox,
  GitBranch,
  BarChart3,
  BookOpen,
  Zap,
  Link2,
  Radar,
  MessageSquare,
  ArrowRight,
  Moon,
  Sun,
} from "lucide-react";
import { useTheme } from "@/providers/theme-provider";

export const Route = createFileRoute("/")({
  component: LandingPage,
});

const features = [
  {
    icon: Radar,
    title: "Signal Engine",
    desc: "Monitor Reddit, HN, Twitter & LinkedIn for buying signals in real-time. AI-scored intent detection catches the conversations that matter.",
  },
  {
    icon: MessageSquare,
    title: "AI Reply Drafts",
    desc: "Generate context-aware, non-spammy replies in three tones: value-only, technical, or soft-sell. Trained on your knowledge base.",
  },
  {
    icon: GitBranch,
    title: "Lead Pipeline",
    desc: "Kanban board tracks leads from Prospect to Converted. Every mention, reply, and click attributed to revenue.",
  },
  {
    icon: BookOpen,
    title: "Knowledge Base",
    desc: "Upload docs, FAQs, and positioning guides. RAG-powered retrieval ensures every AI reply reflects your actual product.",
  },
  {
    icon: Link2,
    title: "Safe Link Tracking",
    desc: "Unique UTM links per reply. Track clicks, signups, and revenue without triggering platform spam filters.",
  },
  {
    icon: Zap,
    title: "Workflow Automation",
    desc: "If-this-then-that rules for auto-drafting, notifications, and lead creation. Set it and let the engine work.",
  },
];

const platforms = [
  { name: "Reddit", color: "bg-orange-200 text-orange-900" },
  { name: "Hacker News", color: "bg-amber-200 text-amber-900" },
  { name: "Twitter / X", color: "bg-sky-200 text-sky-900" },
  { name: "LinkedIn", color: "bg-blue-200 text-blue-900" },
];

function LandingPage() {
  const { theme, toggleTheme } = useTheme();

  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* Nav */}
      <nav className="border-b-2 border-border bg-card">
        <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="w-9 h-9 bg-primary border-2 border-border shadow-xs rounded flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-sm font-bold">
              LE
            </div>
            <span className="font-[family-name:var(--font-head)] text-xl">
              LeadEcho
            </span>
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={toggleTheme}
              className="w-9 h-9 rounded border-2 border-border bg-background shadow-xs hover:shadow-none transition-all flex items-center justify-center cursor-pointer"
            >
              {theme === "dark" ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Moon className="h-4 w-4" />
              )}
            </button>
            <Link to="/login">
              <Button variant="outline" size="sm">
                Log In
              </Button>
            </Link>
            <Link to="/inbox">
              <Button size="sm">
                Dashboard
                <ArrowRight className="h-3.5 w-3.5 ml-1.5" />
              </Button>
            </Link>
          </div>
        </div>
      </nav>

      {/* Hero */}
      <section className="max-w-6xl mx-auto px-6 pt-20 pb-16">
        <div className="max-w-3xl">
          <Badge variant="surface" size="lg" className="mb-6">
            Social Intent Orchestrator
          </Badge>
          <Text as="h1" className="text-5xl lg:text-6xl leading-tight mb-6">
            Turn Social Conversations Into Revenue
          </Text>
          <Text
            as="p"
            className="text-xl text-muted-foreground leading-relaxed mb-10 max-w-2xl"
          >
            Monitor Reddit, Hacker News, Twitter &amp; LinkedIn for buying
            signals. Draft AI-powered replies. Convert conversations into
            paying customers — without being spammy.
          </Text>
          <div className="flex flex-wrap gap-4">
            <Link to="/inbox">
              <Button size="lg">
                Open Dashboard
                <ArrowRight className="h-4 w-4 ml-2" />
              </Button>
            </Link>
            <a href="#features">
              <Button variant="outline" size="lg">
                See How It Works
              </Button>
            </a>
          </div>
        </div>
      </section>

      {/* Platforms */}
      <section className="max-w-6xl mx-auto px-6 pb-16">
        <div className="flex flex-wrap gap-3 items-center">
          <Text as="span" className="text-sm text-muted-foreground mr-2">
            Monitors:
          </Text>
          {platforms.map((p) => (
            <Badge
              key={p.name}
              className={`${p.color} border-2 border-border shadow-xs`}
              size="md"
            >
              {p.name}
            </Badge>
          ))}
        </div>
      </section>

      {/* Features */}
      <section id="features" className="max-w-6xl mx-auto px-6 pb-20">
        <Text as="h2" className="mb-10">
          Everything you need to sell through social
        </Text>
        <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
          {features.map((f) => {
            const Icon = f.icon;
            return (
              <Card key={f.title} className="hover:shadow-sm">
                <CardContent className="p-6">
                  <div className="w-10 h-10 bg-primary border-2 border-border shadow-xs rounded flex items-center justify-center mb-4">
                    <Icon className="h-5 w-5 text-primary-foreground" />
                  </div>
                  <Text as="h4" className="mb-2">
                    {f.title}
                  </Text>
                  <Text
                    as="p"
                    className="text-sm text-muted-foreground leading-relaxed"
                  >
                    {f.desc}
                  </Text>
                </CardContent>
              </Card>
            );
          })}
        </div>
      </section>

      {/* How it works */}
      <section className="border-t-2 border-border bg-card">
        <div className="max-w-6xl mx-auto px-6 py-20">
          <Text as="h2" className="mb-12 text-center">
            From mention to money in 4 steps
          </Text>
          <div className="grid md:grid-cols-4 gap-8">
            {[
              {
                step: "01",
                icon: Radar,
                title: "Detect",
                desc: "Signal engine scans platforms for keyword matches and buying intent.",
              },
              {
                step: "02",
                icon: Inbox,
                title: "Triage",
                desc: "AI scores and classifies mentions. High-intent signals surface first.",
              },
              {
                step: "03",
                icon: MessageSquare,
                title: "Reply",
                desc: "Draft context-aware replies trained on your knowledge base. Review and post.",
              },
              {
                step: "04",
                icon: BarChart3,
                title: "Convert",
                desc: "Track clicks, signups, and revenue through safe UTM links.",
              },
            ].map((s) => {
              const Icon = s.icon;
              return (
                <div key={s.step} className="text-center">
                  <div className="w-14 h-14 bg-primary border-2 border-border shadow-md rounded mx-auto mb-4 flex items-center justify-center">
                    <Icon className="h-6 w-6 text-primary-foreground" />
                  </div>
                  <Badge variant="outline" size="sm" className="mb-3">
                    Step {s.step}
                  </Badge>
                  <Text as="h4" className="mb-2">
                    {s.title}
                  </Text>
                  <Text as="p" className="text-sm text-muted-foreground">
                    {s.desc}
                  </Text>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="max-w-6xl mx-auto px-6 py-20 text-center">
        <Text as="h2" className="mb-4">
          Ready to catch every buying signal?
        </Text>
        <Text
          as="p"
          className="text-muted-foreground text-lg mb-8 max-w-xl mx-auto"
        >
          Stop missing conversations where people are asking for exactly what
          you sell.
        </Text>
        <Link to="/inbox">
          <Button size="lg">
            Get Started Free
            <ArrowRight className="h-4 w-4 ml-2" />
          </Button>
        </Link>
      </section>

      {/* Footer */}
      <footer className="border-t-2 border-border bg-card py-8">
        <div className="max-w-6xl mx-auto px-6 flex items-center gap-6 flex-wrap">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 bg-primary border-2 border-border rounded flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-xs font-bold">
              LE
            </div>
            <span className="font-[family-name:var(--font-head)] text-sm">LeadEcho</span>
          </div>
          <div className="flex items-center gap-5">
            <a href="https://github.com/your-org/leadecho" target="_blank" rel="noreferrer" className="text-xs text-muted-foreground hover:text-foreground transition-colors">GitHub</a>
            <Link to="/privacy" className="text-xs text-muted-foreground hover:text-foreground transition-colors">Privacy</Link>
            <a href="https://www.producthunt.com/posts/leadecho" target="_blank" rel="noreferrer" className="text-xs text-muted-foreground hover:text-foreground transition-colors">Product Hunt</a>
          </div>
          <Text as="span" className="text-xs text-muted-foreground ml-auto">
            MIT License · © 2025 LeadEcho
          </Text>
        </div>
      </footer>
    </div>
  );
}
