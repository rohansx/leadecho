import { Section, SectionHead } from "./section";

const funnel = [
  { label: "Replies posted", num: "61" },
  { label: "Clicked through", num: "38" },
  { label: "Signed up", num: "14" },
  { label: "Revenue added", num: "$4,872", signal: true },
];

const styles = [
  { label: "Story from experience", pct: 31.4 },
  { label: "Technical answer", pct: 22.1 },
  { label: "Value first", pct: 14.8 },
  { label: "Casual", pct: 8.6 },
  { label: "Contrarian", pct: 3.2 },
];

export function Attribution() {
  return (
    <Section>
      <SectionHead title="See which replies turn into customers">
        Every reply gets its own tracking link. Signups and deals flow back to the post, the community and the
        reply style that earned them, or on to HubSpot and Attio.
      </SectionHead>

      <div className="mt-14 grid gap-12 lg:grid-cols-[1fr_1fr]">
        <dl className="grid grid-cols-2 border-t border-l border-border">
          {funnel.map((f) => (
            <div key={f.label} className="border-r border-b border-border p-6">
              <dt className="text-[15px] text-muted-foreground">{f.label}</dt>
              <dd className={`mt-2 text-4xl font-bold tracking-[-0.02em] ${f.signal ? "text-signal" : ""}`}>
                {f.num}
              </dd>
            </div>
          ))}
        </dl>

        <div>
          <h3 className="text-[15px] font-semibold">Signup rate by reply style</h3>
          <ul className="mt-4 space-y-3">
            {styles.map((s) => (
              <li key={s.label} className="grid grid-cols-[8.5rem_1fr_3rem] sm:grid-cols-[11rem_1fr_3.5rem] items-center gap-4 text-[15px]">
                <span className="text-foreground-soft">{s.label}</span>
                <div className="h-2 rounded-full bg-muted">
                  <div className="h-full rounded-full bg-foreground" style={{ width: `${(s.pct / 31.4) * 100}%` }} />
                </div>
                <span className="text-right">{s.pct}%</span>
              </li>
            ))}
          </ul>
          <p className="mt-6 text-[13px] text-muted-foreground">Example workspace, last 30 days.</p>
        </div>
      </div>
    </Section>
  );
}
