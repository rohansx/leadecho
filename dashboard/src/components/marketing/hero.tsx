import { useState, type ComponentType, type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { GITHUB_URL } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { btnPrimary, btnSecondary } from "./buttons";
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

interface Reply {
  user: string;
  age: string;
  meta: string;
  text: ReactNode;
  buyer?: boolean;
}

interface Thread {
  tab: string;
  Icon: ComponentType<{ className?: string }>;
  where: string;
  title: string;
  replies: Reply[];
  score: string;
  why: string;
  read: number;
}

const m = (t: string) => <mark className="mkt-mark">{t}</mark>;

// Sample threads. They show what LeadEcho flags; they are not real posts.
const threads: Thread[] = [
  {
    tab: "Reddit",
    Icon: RedditIcon,
    where: "r/SaaS",
    title: "What are you all using to find customers on Reddit?",
    read: 214,
    score: "9.2",
    why: "Ready to buy. Matches your pain point “Reddit monitoring hits rate limits”.",
    replies: [
      { user: "u/growthmarco", age: "3h", meta: "41 points", text: "Honestly just search the sub every morning with coffee. Works fine at our size." },
      {
        user: "u/build_in_public",
        age: "2h",
        meta: "12 points",
        buyer: true,
        text: <>We tried that and kept missing threads. We need something that {m("watches Reddit for people describing our problem")}, not just our brand name, and it can't keep hitting the API limits. {m("Budget is $50–100/mo")}.</>,
      },
      { user: "u/no_ads_please", age: "2h", meta: "33 points", text: "Please, no more replies that are just a referral link." },
    ],
  },
  {
    tab: "Hacker News",
    Icon: HackerNewsIcon,
    where: "Ask HN",
    title: "How did you get your first 100 users for a dev tool?",
    read: 187,
    score: "8.7",
    why: "Asking for a tool. Matches “finding buyers in developer communities”.",
    replies: [
      { user: "pg_fan_2009", age: "5h", meta: "18 replies", text: "Write things people want to read. Distribution follows good writing." },
      {
        user: "flakyci",
        age: "4h",
        meta: "6 replies",
        buyer: true,
        text: <>Two of us, no marketing person. I'd {m("happily pay for something that surfaces threads where people complain about flaky CI")}, so I can answer them while they're still deciding.</>,
      },
      { user: "throwaway_4421", age: "3h", meta: "2 replies", text: "Launch HN worked for us, but only once." },
    ],
  },
  {
    tab: "LinkedIn",
    Icon: LinkedInIcon,
    where: "LinkedIn post",
    title: "Priya N., Head of Growth at a Series A devtools company",
    read: 96,
    score: "9.0",
    why: "Evaluating vendors. Matches “manual social prospecting takes hours”.",
    replies: [
      {
        user: "Priya N.",
        age: "1d",
        meta: "Author",
        buyer: true,
        text: <>My team spends {m("about five hours a week scrolling Reddit for leads")}. Does anyone use a tool that {m("flags buying intent instead of brand mentions")}? Open to recommendations.</>,
      },
      { user: "Marcus T.", age: "1d", meta: "Comment", text: "Great question, following for the answers." },
      { user: "Dana K.", age: "20h", meta: "Comment", text: "We hired an SDR for this. It didn't scale." },
    ],
  },
];

function ThreadCard({ t }: { t: Thread }) {
  return (
    <figure className="overflow-hidden rounded-[14px] border border-border bg-card shadow-[0_1px_2px_rgba(28,26,20,0.04),0_18px_40px_-20px_rgba(28,26,20,0.28)]">
      <div className="flex min-w-0 items-center gap-2 border-b border-border px-5 py-3 text-[13px] text-muted-foreground">
        <t.Icon className="size-4 shrink-0" />
        <span className="shrink-0 font-medium text-foreground">{t.where}</span>
        <span className="truncate">{t.title}</span>
      </div>
      <div className="space-y-5 px-5 py-5">
        {t.replies.map((r) =>
          r.buyer ? (
            <div key={r.user} className="-mx-3 rounded-[10px] border border-foreground/70 px-3 py-3">
              <ReplyBody r={r} />
              <div className="mkt-score mt-3 flex items-center gap-3 border-t border-border pt-3 text-[13px]">
                <span className="rounded-full bg-yellow px-2.5 py-0.5 text-[15px] font-semibold text-[#1c1a14]">{t.score}</span>
                <span className="text-foreground-soft">{t.why}</span>
              </div>
            </div>
          ) : (
            <div key={r.user} className="mkt-noise">
              <ReplyBody r={r} />
            </div>
          ),
        )}
      </div>
      <figcaption className="border-t border-border px-5 py-3 text-[13px] text-muted-foreground">
        LeadEcho read all {t.read} replies. One person is looking to buy.
      </figcaption>
    </figure>
  );
}

function ReplyBody({ r }: { r: Reply }) {
  return (
    <>
      <div className="flex items-baseline gap-2 text-[13px] text-muted-foreground">
        <span className="font-medium text-foreground-soft">{r.user}</span>
        <span>{r.age}</span>
        <span className="ml-auto">{r.meta}</span>
      </div>
      <p className="mt-1 text-[15px] leading-relaxed">{r.text}</p>
    </>
  );
}

function Demo() {
  const [i, setI] = useState(0);
  return (
    <div className="rounded-[20px] border border-border p-3 sm:p-5" style={{ background: "var(--demo)" }}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="tablist" aria-label="Sample threads" className="inline-flex rounded-full bg-card/60 p-1 backdrop-blur">
          {threads.map((t, n) => (
            <button
              key={t.tab}
              role="tab"
              type="button"
              aria-selected={i === n}
              onClick={() => setI(n)}
              className={cn(
                "cursor-pointer rounded-full px-3.5 py-1.5 text-[13px] font-medium transition-colors",
                i === n ? "bg-card text-foreground shadow-sm" : "text-foreground-soft hover:text-foreground",
              )}
            >
              {t.tab}
            </button>
          ))}
        </div>
        <span className="hidden rounded-full bg-card/60 px-3.5 py-1.5 text-[13px] text-foreground-soft backdrop-blur sm:inline">
          Sample threads, scored the way LeadEcho scores them
        </span>
      </div>
      <div className="mx-auto mt-5 max-w-2xl pb-2 sm:mt-8 sm:pb-6">
        {/* key remounts the card so the highlight animation replays per tab */}
        <ThreadCard key={i} t={threads[i]} />
      </div>
    </div>
  );
}

export function Hero() {
  return (
    <section className="px-5 pt-14 pb-10 sm:px-10 lg:pt-20">
      <div className="grid gap-10 [&>*]:min-w-0 lg:grid-cols-[1.15fr_1fr] lg:items-end lg:gap-16">
        <div>
          <p className="flex items-center gap-2 text-[14px] text-foreground-soft">
            <span className="size-2 rounded-full bg-yellow ring-2 ring-yellow/30" />
            Open source, self-hostable
          </p>
          <h1 className="mt-5 text-[clamp(2.75rem,6vw,4.4rem)] leading-[1.02] font-light tracking-[-0.04em] text-balance">
            Someone just asked for what you sell.{" "}
            <span className="text-muted-head">Be the one who answers.</span>
          </h1>
        </div>
        <div className="lg:pb-2">
          <p className="text-[17px] leading-relaxed text-foreground-soft">
            LeadEcho reads Reddit, Hacker News, X, LinkedIn and developer communities around the clock. It finds the
            people describing the problem your product solves, tells you who they are, and drafts a reply for you to
            approve.
          </p>
          <div className="mt-7 flex flex-wrap gap-3">
            <Link to="/register" className={btnPrimary}>
              Start free
            </Link>
            <a href={GITHUB_URL} target="_blank" rel="noreferrer" className={btnSecondary}>
              Self-host from GitHub
            </a>
          </div>
          <p className="mt-4 text-[13px] text-muted-foreground">Free plan with 300 credits a month. MIT licensed.</p>
        </div>
      </div>

      <div className="mt-14">
        <Demo />
      </div>

      <div className="mt-10 flex flex-wrap items-center gap-x-7 gap-y-3 text-[14px] text-muted-foreground">
        <span className="font-[family-name:var(--font-plexmono)] text-[12px] uppercase tracking-[0.08em]">Watches</span>
        {platforms.map(({ name, Icon }) => (
          <span key={name} className="inline-flex items-center gap-2">
            <Icon className="size-4 shrink-0" />
            {name}
          </span>
        ))}
      </div>
    </section>
  );
}
