import { Link } from "@tanstack/react-router";
import { GITHUB_URL } from "@/lib/constants";
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

function Comment({
  user,
  age,
  votes,
  noise,
  children,
}: {
  user: string;
  age: string;
  votes: number;
  noise?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className={noise ? "mkt-noise" : undefined}>
      <div className="flex items-baseline gap-2 text-[13px] text-muted-foreground">
        <span className="font-semibold text-foreground-soft">u/{user}</span>
        <span>{age}</span>
        <span className="ml-auto">{votes} points</span>
      </div>
      <p className="mt-1 text-[15px] leading-relaxed">{children}</p>
    </div>
  );
}

/** A thread as LeadEcho sees it: most of it is noise, one comment is a buyer. */
function Thread() {
  return (
    <figure className="rounded-[10px] border border-border bg-card shadow-[0_1px_0_rgba(14,23,38,0.04),0_12px_32px_-12px_rgba(14,23,38,0.18)]">
      <div className="flex items-center gap-2 border-b border-border px-5 py-3 text-[13px] text-muted-foreground">
        <RedditIcon className="size-4" />
        <span className="font-semibold text-foreground">r/SaaS</span>
        <span className="truncate">What are you all using to find customers on Reddit?</span>
      </div>
      <div className="space-y-5 px-5 py-5">
        <Comment user="growthmarco" age="3h" votes={41} noise>
          Honestly just search the sub every morning with coffee. Works fine at our size.
        </Comment>
        <div className="relative -mx-3 rounded-lg border border-foreground/80 px-3 py-3">
          <Comment user="build_in_public" age="2h" votes={12}>
            We tried that and kept missing threads. We need something that{" "}
            <mark className="mkt-mark">watches Reddit for people describing our problem</mark>, not just our
            brand name, and it can't keep hitting the API limits.{" "}
            <mark className="mkt-mark">Budget is $50–100/mo</mark>.
          </Comment>
          <div className="mkt-score mt-3 flex items-center gap-3 border-t border-border pt-3 text-[13px]">
            <span className="text-2xl font-bold tracking-tight text-signal">9.2</span>
            <span className="text-foreground-soft">
              Ready to buy. Matches your pain point “Reddit monitoring hits rate limits”.
            </span>
          </div>
        </div>
        <Comment user="no_ads_please" age="2h" votes={33} noise>
          Please, no more replies that are just a referral link.
        </Comment>
        <Comment user="kernel_panic" age="1h" votes={8} noise>
          F5Bot is free if you only need keyword alerts.
        </Comment>
      </div>
      <figcaption className="border-t border-border px-5 py-3 text-[13px] text-muted-foreground">
        LeadEcho read all 214 comments in this thread. One person is looking to buy.
      </figcaption>
    </figure>
  );
}

export function Hero() {
  return (
    <section className="mx-auto max-w-6xl px-4 pt-16 pb-20 sm:px-6 lg:pt-24">
      <div className="grid gap-14 [&>*]:min-w-0 lg:grid-cols-[1fr_1.05fr] lg:gap-16 lg:items-center">
        <div>
          <h1 className="text-[clamp(2.75rem,6vw,4.5rem)] leading-[0.98] font-bold tracking-[-0.035em] text-balance">
            Someone just asked for what you sell.
          </h1>
          <p className="mt-6 max-w-[34rem] text-lg leading-relaxed text-foreground-soft">
            LeadEcho reads Reddit, Hacker News, X, LinkedIn and developer communities around the clock. It finds
            the people describing the problem your product solves, tells you who they are, and drafts a reply for
            you to approve.
          </p>
          <div className="mt-9 flex flex-wrap gap-3">
            <Link to="/register" className={btnPrimary}>
              Start free
            </Link>
            <a href={GITHUB_URL} target="_blank" rel="noreferrer" className={btnSecondary}>
              Self-host from GitHub
            </a>
          </div>
          <p className="mt-4 text-[13px] text-muted-foreground">
            Free plan with 300 credits a month. Open source under MIT.
          </p>
        </div>
        <Thread />
      </div>

      <div className="mt-20 flex flex-wrap items-center gap-x-7 gap-y-3 text-[15px] text-muted-foreground">
        <span className="text-foreground-soft">Watches</span>
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
