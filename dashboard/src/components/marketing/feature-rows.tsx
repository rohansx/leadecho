import type { ReactNode } from "react";
import { Sieve, Hub, Keyboard, Plot } from "@lucasmarkes/hairline/react";
import { cn } from "@/lib/utils";
import { Section, Eyebrow, H2 } from "./section";
import { Reveal, CountUp, Bar } from "./fx";
import { useTypewriter } from "./use-typewriter";

function Row({
  figure,
  eyebrow,
  title,
  body,
  flip,
  children,
}: {
  figure: ReactNode;
  eyebrow: string;
  title: string;
  body: string;
  flip?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="grid gap-10 lg:grid-cols-2 lg:items-center lg:gap-20 [&>*]:min-w-0">
      <Reveal className={cn(flip && "lg:order-2")}>
        <div className="rounded-[24px] border border-border bg-card px-8 py-10 sm:px-14">{figure}</div>
      </Reveal>
      <Reveal delay={0.08}>
        <Eyebrow>{eyebrow}</Eyebrow>
        <H2 className="mt-4">{title}</H2>
        <p className="mt-5 max-w-lg text-foreground-soft">{body}</p>
        <div className="mt-8">{children}</div>
      </Reveal>
    </div>
  );
}

// Example workspace, 30 days. Bar width uses sqrt so the small stages stay visible.
const stages = [
  { name: "Collected", count: 2184 },
  { name: "Not spam", count: 966 },
  { name: "Matches your problem", count: 459 },
  { name: "Ready to buy", count: 73 },
];

function Funnel() {
  const max = Math.sqrt(stages[0].count);
  return (
    <ul className="space-y-3.5">
      {stages.map((s, i) => {
        const last = i === stages.length - 1;
        return (
          <li key={s.name} className="grid grid-cols-[9.5rem_1fr_3.5rem] items-center gap-4 text-[15px] sm:grid-cols-[11rem_1fr_4rem]">
            <span className={last ? "font-semibold" : "text-foreground-soft"}>{s.name}</span>
            <Bar pct={(Math.sqrt(s.count) / max) * 100} className={cn("h-full rounded-full", last ? "bg-dot" : "bg-foreground/80")} />
            <span className={cn("text-right font-[family-name:var(--font-mono)] text-[14px]", last && "font-semibold")}>
              <CountUp to={s.count} />
            </span>
          </li>
        );
      })}
    </ul>
  );
}

function Profile() {
  const rows = [
    ["Role", "Founder and engineer at Cursive Labs"],
    ["Company", "cursivelabs.dev, from the GitHub profile"],
    ["Stage", "Comparing vendors, has a budget"],
  ];
  return (
    <div className="rounded-[18px] border border-border bg-card p-5">
      <div className="flex items-center gap-3">
        <span className="flex size-10 items-center justify-center rounded-full bg-muted text-[14px] font-semibold">DR</span>
        <div className="min-w-0 flex-1">
          <div className="font-semibold">Daniel Reyes</div>
          <div className="text-[13px] text-muted-foreground">u/build_in_public on Reddit</div>
        </div>
        <span className="rounded-full bg-chip-bg px-2.5 py-0.5 text-[13px] font-semibold text-chip-fg">88% fit</span>
      </div>
      <dl className="mt-4 border-t border-border">
        {rows.map(([k, v]) => (
          <div key={k} className="grid grid-cols-[5.5rem_1fr] gap-3 border-b border-border py-2.5 text-[14px] last:border-0">
            <dt className="text-muted-foreground">{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

const DRAFT =
  "Hey, I built LeadEcho partly because I kept hitting the same API limits. It reads threads for meaning, not keywords, and runs on docker compose, so the only cost is your own AI keys.";

function Draft() {
  const { ref, typed, done } = useTypewriter(DRAFT);
  return (
    <div ref={ref} className="rounded-[18px] border border-border bg-card p-5">
      <div className="flex items-center justify-between text-[13px] text-muted-foreground">
        <span>Reply to u/build_in_public</span>
        <span>Grounded in 3 docs</span>
      </div>
      <p className="mt-3 min-h-[6.5rem] text-[15px] leading-relaxed">
        <span className="sr-only">{DRAFT}</span>
        <span aria-hidden="true">
          {typed}
          {!done && <span className="mkt-caret ml-0.5 inline-block h-4 w-[2px] translate-y-0.5 bg-foreground" />}
        </span>
      </p>
      <div className="mt-4 flex gap-2 text-[13px] font-semibold">
        <span className="rounded-full bg-primary-bg px-3.5 py-1.5 text-primary-fg">Approve and post</span>
        <span className="rounded-full border border-border px-3.5 py-1.5 text-foreground-soft">Edit</span>
      </div>
    </div>
  );
}

const results = [
  { label: "Replies posted", to: 61 },
  { label: "Signed up", to: 14 },
  { label: "Revenue added", to: 4872, prefix: "$" },
];

function Results() {
  return (
    <div className="rounded-[18px] border border-border bg-card">
      <dl className="grid grid-cols-3 divide-x divide-border">
        {results.map((r) => (
          <div key={r.label} className="p-4 sm:p-5">
            <dt className="text-[13px] text-muted-foreground">{r.label}</dt>
            <dd className="mt-1 text-[26px] font-semibold tracking-[-0.03em]">
              <CountUp to={r.to} prefix={r.prefix} />
            </dd>
          </div>
        ))}
      </dl>
      <div className="flex items-center gap-4 border-t border-border p-4 text-[14px] sm:px-5">
        <span className="w-40 shrink-0 text-foreground-soft">Best style: story</span>
        <Bar pct={78} className="h-full rounded-full bg-dot" />
        <span className="font-[family-name:var(--font-mono)] text-[13px]">31%</span>
      </div>
    </div>
  );
}

export function FeatureRows() {
  return (
    <Section className="space-y-20 sm:space-y-24">
      <Row
        figure={<Sieve play className="mx-auto max-w-[380px]" intensity={0.7} label="Stacked sieves sorting what passes through" />}
        eyebrow="Filters"
        title="Thousands of posts in. Only buyers out."
        body="Four passes drop spam, match posts to the problems you described by meaning, and score buying intent. Cheap models go first, so AI costs stay at a few dollars a month."
      >
        <Funnel />
        <p className="mt-4 text-[13px] text-muted-foreground">One example workspace, last 30 days.</p>
      </Row>
      <Row
        flip
        figure={<Hub play className="mx-auto max-w-[380px]" intensity={0.7} label="A hub linking several sources together" />}
        eyebrow="Researches"
        title="Know who is asking before you answer."
        body="Each lead is linked to a GitHub profile and a company, then scored against your ideal customer, so you can tell a founder with a budget from someone just curious."
      >
        <Profile />
      </Row>
      <Row
        figure={<Keyboard play className="mx-auto max-w-[400px]" intensity={0.6} label="A keyboard whose keys sink under the pointer" />}
        eyebrow="Drafts"
        title="A reply drafted from your docs and the thread."
        body="Drafts read the full thread and quote your real product. Nothing posts until you approve it, and it posts from your own account through the Chrome extension."
      >
        <Draft />
      </Row>
      <Row
        flip
        figure={<Plot play className="mx-auto max-w-[380px]" intensity={0.7} label="A bar chart whose bars lift under the pointer" />}
        eyebrow="Attributes"
        title="See which replies turn into customers."
        body="Every reply carries its own link. Signups and revenue flow back to the post, the community and the reply style that earned them, or on to HubSpot and Attio."
      >
        <Results />
      </Row>
    </Section>
  );
}
