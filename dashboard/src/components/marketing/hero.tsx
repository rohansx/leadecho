import { useEffect, useState, type ComponentType } from "react";
import { Link } from "@tanstack/react-router";
import { useReducedMotion } from "motion/react";
import { Dish } from "@lucasmarkes/hairline/react";
import { GITHUB_URL } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { btnPrimary, btnSecondary, container } from "./buttons";
import {
  RedditIcon,
  XIcon,
  LinkedInIcon,
  HackerNewsIcon,
  DevtoIcon,
  LobstersIcon,
  IndieHackersIcon,
} from "./platform-logos";

const platforms = [
  { name: "Reddit", Icon: RedditIcon },
  { name: "Hacker News", Icon: HackerNewsIcon },
  { name: "X", Icon: XIcon },
  { name: "LinkedIn", Icon: LinkedInIcon },
  { name: "Dev.to", Icon: DevtoIcon },
  { name: "Lobsters", Icon: LobstersIcon },
  { name: "Indie Hackers", Icon: IndieHackersIcon },
];

interface Post {
  Icon: ComponentType<{ className?: string }>;
  where: string;
  text: string;
  score?: string;
}

// Sample posts for the feed; most are noise, a few are buyers.
const posts: Post[] = [
  { Icon: RedditIcon, where: "r/SaaS", text: "Budget is $50–100/mo, we just need something that watches Reddit for our problem", score: "9.2" },
  { Icon: HackerNewsIcon, where: "Hacker News", text: "Show HN: I built a tiny static site generator in Rust" },
  { Icon: XIcon, where: "X", text: "anyone have a tool that flags buying intent instead of brand mentions?", score: "8.8" },
  { Icon: DevtoIcon, where: "Dev.to", text: "10 VS Code extensions I can't live without" },
  { Icon: LinkedInIcon, where: "LinkedIn", text: "My team spends five hours a week scrolling Reddit for leads. Open to tools.", score: "9.0" },
  { Icon: RedditIcon, where: "r/webdev", text: "Is it just me or is CSS getting harder every year" },
  { Icon: IndieHackersIcon, where: "Indie Hackers", text: "Paying for social listening and it's 90% noise. What do you all use?", score: "8.4" },
  { Icon: LobstersIcon, where: "Lobsters", text: "A deep dive into Postgres vacuum internals" },
];

/** A feed where a new post lands every few seconds; buyers get a score. */
function LiveFeed() {
  const reduce = useReducedMotion();
  const [head, setHead] = useState(0);

  useEffect(() => {
    if (reduce) return;
    const id = setInterval(() => setHead((h) => (h + 1) % posts.length), 2600);
    return () => clearInterval(id);
  }, [reduce]);

  const visible = [0, 1, 2, 3].map((k) => ({ ...posts[(head - k + posts.length * 2) % posts.length], k }));

  return (
    <ul aria-label="Sample feed of scored posts" className="divide-y divide-border">
      {visible.map((p) => (
        <li key={`${head}-${p.k}`} className={cn("px-5 py-3", p.k === 0 && !reduce && "mkt-feed-in")}>
          {/* Dimming lives on this wrapper: the slide-in animation owns the li's opacity. */}
          <div className={cn("flex items-center gap-3", !p.score && "opacity-45")}>
            <p.Icon className="size-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <div className="text-[12px] text-muted-foreground">{p.where}</div>
              <div className="truncate text-[14px]">{p.text}</div>
            </div>
            {p.score ? (
              <span className={cn("shrink-0 rounded-full px-2 py-0.5 text-[13px] font-semibold", Number(p.score) >= 8.5 ? "bg-chip-bg text-chip-fg" : "border border-border text-foreground-soft")}>{p.score}</span>
            ) : (
              <span className="shrink-0 text-[12px] text-muted-foreground">skipped</span>
            )}
          </div>
        </li>
      ))}
    </ul>
  );
}

/** The product video in a hairline browser frame. Sized by width/height so nothing shifts while it loads. */
function HeroVideo() {
  const reduce = useReducedMotion();
  return (
    <div className={cn(container, "pb-16")}>
      <div className="mkt-glass overflow-hidden rounded-[22px] border border-border p-1.5">
        <video
          className="block h-auto w-full rounded-[16px] bg-[#0b0b0c]"
          width={1600}
          height={1000}
          autoPlay={!reduce}
          controls={!!reduce}
          muted
          loop
          playsInline
          preload="metadata"
          poster="/hero-demo-poster.jpg"
          aria-label="Animated walkthrough of LeadEcho with sample data: posts are filtered, a buyer is scored and researched, and a drafted reply is approved"
        >
          <source src="/hero-demo.webm" type="video/webm" />
          <source src="/hero-demo.mp4" type="video/mp4" />
        </video>
      </div>
    </div>
  );
}

function ProofStrip() {
  return (
    <div className="border-y border-border">
      <ul className={cn(container, "flex flex-wrap items-center justify-between gap-x-10 gap-y-2 py-4 text-[14px] text-foreground-soft")}>
        <li>
          <a href={GITHUB_URL} target="_blank" rel="noreferrer" className="underline decoration-border underline-offset-4 hover:text-foreground">
            MIT licensed, open source
          </a>
        </li>
        <li>Reddit, HN, X, LinkedIn and more</li>
        <li>Self-hostable, bring your own AI keys</li>
      </ul>
    </div>
  );
}

export function Hero() {
  return (
    <section className="relative overflow-hidden">
      <div className={cn(container, "grid gap-14 pt-12 pb-12 lg:grid-cols-[1.05fr_1fr] lg:items-center lg:gap-12 lg:pt-14 lg:pb-14 [&>*]:min-w-0")}>
        <div>
          <p className="inline-flex items-center gap-2 mkt-glass rounded-full border border-border px-3 py-1 text-[13px] text-foreground-soft">
            <span className="mkt-live size-2 rounded-full bg-foreground" aria-hidden="true" />
            Listening on Reddit, HN, X and more, open source
          </p>
          <h1 className="mt-7 text-[clamp(2.9rem,6.2vw,5rem)] leading-[0.96] font-semibold tracking-[-0.045em] text-balance [font-stretch:90%]">
            Find the people already asking for what you sell.
          </h1>
          <p className="mt-6 max-w-[33rem] text-[18px] leading-relaxed text-foreground-soft">
            LeadEcho reads Reddit, Hacker News, X, LinkedIn and developer communities on a schedule, keeps the
            posts that look like someone who could buy, explains why, and drafts a reply you approve before it goes out.
          </p>
          <div className="mt-9 flex flex-wrap gap-3">
            <Link to="/register" className={btnPrimary}>
              Start free
            </Link>
            <a href={GITHUB_URL} target="_blank" rel="noreferrer" className={btnSecondary}>
              Self-host from GitHub
            </a>
          </div>
          <div className="mt-12 flex flex-wrap items-center gap-x-5 gap-y-3 text-muted-foreground">
            {platforms.map(({ name, Icon }) => (
              <span key={name} title={name}>
                <Icon className="size-5" />
                <span className="sr-only">{name}</span>
              </span>
            ))}
            <span className="text-[13px]">and more</span>
          </div>
        </div>

        <div className="relative">
          <Dish className="on-page mx-auto w-full max-w-[460px]" play intensity={0.7} label="A dish antenna turning to listen" />
          <div className="relative -mt-16 mkt-glass overflow-hidden rounded-[20px] border border-border sm:mx-6">
            <div className="flex items-center justify-between border-b border-border px-5 py-3 text-[13px]">
              <span className="inline-flex items-center gap-2 font-medium">
                <span className="mkt-live size-1.5 rounded-full bg-foreground" aria-hidden="true" />
                Incoming
              </span>
              <span className="text-muted-foreground">sample data</span>
            </div>
            <LiveFeed />
          </div>
        </div>
      </div>
      <HeroVideo />
      <ProofStrip />
    </section>
  );
}
