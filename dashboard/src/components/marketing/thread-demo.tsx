import { useState, type ComponentType, type ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Section, Eyebrow, H2 } from "./section";
import { Reveal } from "./fx";
import { RedditIcon, LinkedInIcon, HackerNewsIcon } from "./platform-logos";

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

function ThreadCard({ t }: { t: Thread }) {
  return (
    <figure className="overflow-hidden rounded-[18px] border border-border bg-card shadow-[0_1px_2px_rgba(24,24,27,0.04),0_24px_48px_-28px_rgba(24,24,27,0.3)]">
      <div className="flex min-w-0 items-center gap-2 border-b border-border px-5 py-3 text-[13px] text-muted-foreground">
        <t.Icon className="size-4 shrink-0" />
        <span className="shrink-0 font-medium text-foreground">{t.where}</span>
        <span className="truncate">{t.title}</span>
      </div>
      <div className="space-y-5 px-5 py-5">
        {t.replies.map((r) =>
          r.buyer ? (
            <div key={r.user} className="-mx-3 rounded-[12px] border border-foreground/60 px-3 py-3">
              <ReplyBody r={r} />
              <div className="mkt-score mt-3 flex items-center gap-3 border-t border-border pt-3 text-[13px]">
                <span className="rounded-full bg-chip-bg px-2.5 py-0.5 text-[14px] font-semibold text-chip-fg">{t.score}</span>
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

export function ThreadDemo() {
  const [i, setI] = useState(0);
  return (
    <Section id="product" className="border-y border-border bg-card">
      <div className="grid gap-12 lg:grid-cols-[0.85fr_1.15fr] lg:items-center lg:gap-16 [&>*]:min-w-0">
        <Reveal>
          <Eyebrow>Reads every reply</Eyebrow>
          <H2 className="mt-4">Most of a thread is noise. LeadEcho finds the reply that isn't.</H2>
          <p className="mt-5 max-w-md text-foreground-soft">
            It reads the whole conversation, marks the sentence where someone describes your problem, and scores how
            ready they are to buy. Pick a community to see it work.
          </p>
          <div role="tablist" aria-label="Sample threads" className="mt-8 inline-flex rounded-full border border-border bg-background p-1">
            {threads.map((t, n) => (
              <button
                key={t.tab}
                role="tab"
                type="button"
                aria-selected={i === n}
                onClick={() => setI(n)}
                className={cn(
                  "inline-flex cursor-pointer items-center gap-2 rounded-full px-3.5 py-1.5 text-[14px] font-medium transition-colors",
                  i === n ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
                )}
              >
                <t.Icon className="size-3.5" />
                {t.tab}
              </button>
            ))}
          </div>
        </Reveal>
        <Reveal delay={0.1}>
          {/* key remounts the card so the highlight animation replays per tab */}
          <ThreadCard key={i} t={threads[i]} />
        </Reveal>
      </div>
    </Section>
  );
}
