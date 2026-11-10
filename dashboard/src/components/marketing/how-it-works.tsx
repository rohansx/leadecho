import { Section, SectionHead } from "./section";

// Example workspace, 30 days. Bar width uses sqrt so the small stages stay visible.
const stages = [
  { name: "Collect", desc: "New posts and comments from 7 communities, every 5 minutes.", count: 2184 },
  { name: "Drop spam", desc: "Bots, link drops and self-promotion are removed by a fast, cheap model.", count: 966 },
  { name: "Match your problem", desc: "Embeddings compare each post to the pain points you described, by meaning rather than keywords.", count: 459 },
  { name: "Score buying intent", desc: "A stronger model decides who is asking for a solution, complaining, or just chatting.", count: 73 },
];
const max = Math.sqrt(stages[0].count);

export function HowItWorks() {
  return (
    <Section id="how">
      <SectionHead title="From 2,184 posts to the 73 people worth answering">
        Keyword alerts send you everything. LeadEcho filters in four passes, and only the posts that survive
        reach your inbox. These numbers are from one example workspace over 30 days.
      </SectionHead>

      <ol className="mt-14 border-t border-border">
        {stages.map((s, i) => {
          const last = i === stages.length - 1;
          return (
            <li
              key={s.name}
              className="grid grid-cols-[2rem_1fr] gap-x-4 gap-y-2 border-b border-border py-6 md:gap-x-8 md:grid-cols-[2.5rem_14rem_1fr_minmax(10rem,22rem)] md:items-center"
            >
              <span className="text-[15px] text-muted-foreground">{i + 1}</span>
              <h3 className="text-xl font-semibold tracking-[-0.01em]">{s.name}</h3>
              <p className="col-start-2 text-[15px] text-foreground-soft md:col-start-auto">{s.desc}</p>
              <div className="col-start-2 flex items-center gap-4 md:col-start-auto">
                <div className="h-2 flex-1 rounded-full bg-muted">
                  <div
                    className={`h-full rounded-full ${last ? "bg-signal" : "bg-foreground"}`}
                    style={{ width: `${(Math.sqrt(s.count) / max) * 100}%` }}
                  />
                </div>
                <span className={`w-16 text-right text-xl font-semibold ${last ? "text-signal" : ""}`}>
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
