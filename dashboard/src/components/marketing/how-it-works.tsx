import { Section, SectionHead } from "./section";

// Example workspace, 30 days. Bar width uses sqrt so the small stages stay visible.
const stages = [
  { name: "Collect", desc: "New posts and comments from 7 communities, every 5 minutes.", count: 2184 },
  { name: "Drop spam", desc: "Bots, link drops and self-promotion go first, through a fast, cheap model.", count: 966 },
  { name: "Match your problem", desc: "Embeddings compare each post to the pain points you described, by meaning rather than keywords.", count: 459 },
  { name: "Score buying intent", desc: "A stronger model decides who is asking for a solution, complaining, or just chatting.", count: 73 },
];
const max = Math.sqrt(stages[0].count);

export function HowItWorks() {
  return (
    <Section id="how">
      <SectionHead label="How it filters" title="From 2,184 posts to 73 people." muted="Only the ones worth answering.">
        Keyword alerts send you everything. LeadEcho filters in four passes, and only the posts that survive reach
        your inbox. These numbers are from one example workspace over 30 days.
      </SectionHead>

      <ol className="mt-10 overflow-hidden rounded-[18px] border border-border bg-card">
        {stages.map((s, i) => {
          const last = i === stages.length - 1;
          return (
            <li
              key={s.name}
              className="grid gap-x-8 gap-y-3 border-t border-border p-5 first:border-t-0 sm:p-6 md:grid-cols-[1fr_15rem] md:items-center"
            >
              <div className="flex gap-4">
                <span className="pt-0.5 font-[family-name:var(--font-plexmono)] text-[13px] text-muted-foreground">{i + 1}</span>
                <div>
                  <h3 className="text-[19px] font-medium tracking-[-0.01em]">{s.name}</h3>
                  <p className="mt-1 text-[15px] text-foreground-soft">{s.desc}</p>
                </div>
              </div>
              <div className="flex items-center gap-4 pl-8 md:pl-0">
                <div className="h-2 flex-1 rounded-full bg-muted">
                  <div
                    className={`h-full rounded-full ${last ? "bg-yellow" : "bg-foreground/80"}`}
                    style={{ width: `${(Math.sqrt(s.count) / max) * 100}%` }}
                  />
                </div>
                <span className="w-16 text-right font-[family-name:var(--font-plexmono)] text-[17px]">
                  {s.count.toLocaleString()}
                </span>
              </div>
            </li>
          );
        })}
      </ol>
      <p className="mt-5 text-[15px] text-muted-foreground">
        Spam is dropped before any expensive model runs, which keeps AI cost to a few dollars a month.
      </p>
    </Section>
  );
}
