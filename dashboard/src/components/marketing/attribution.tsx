import { Section, SectionHead } from "./section";

const funnel = [
  { label: "Replies posted", num: "61" },
  { label: "Clicked through", num: "38" },
  { label: "Signed up", num: "14" },
  { label: "Revenue added", num: "$4,872", hi: true },
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
    <Section id="attribution">
      <SectionHead label="What converts" title="Every reply gets its own link." muted="So you know which ones paid off.">
        Signups and deals flow back to the post, the community and the reply style that earned them, or on to
        HubSpot and Attio.
      </SectionHead>

      <div className="mt-10 overflow-hidden rounded-[18px] border border-border bg-card">
        <dl className="grid grid-cols-2 md:grid-cols-4">
          {funnel.map((f, i) => (
            <div
              key={f.label}
              className={`border-border p-5 sm:p-6 ${i % 2 ? "border-l" : ""} ${i > 1 ? "border-t md:border-t-0" : ""} ${i === 2 ? "md:border-l" : ""}`}
            >
              <dt className="text-[14px] text-muted-foreground">{f.label}</dt>
              <dd className="mt-2 flex items-center gap-2 text-[34px] leading-none font-light tracking-[-0.03em]">
                {f.num}
                {f.hi && <span className="size-2 rounded-full bg-yellow" aria-hidden="true" />}
              </dd>
            </div>
          ))}
        </dl>
        <div className="border-t border-border p-5 sm:p-6">
          <h3 className="text-[15px] font-medium">Signup rate by reply style</h3>
          <ul className="mt-4 space-y-3">
            {styles.map((s, i) => (
              <li key={s.label} className="grid grid-cols-[8.5rem_1fr_3rem] items-center gap-4 text-[15px] sm:grid-cols-[11rem_1fr_3.5rem]">
                <span className="text-foreground-soft">{s.label}</span>
                <div className="h-2 rounded-full bg-muted">
                  <div
                    className={`h-full rounded-full ${i === 0 ? "bg-yellow" : "bg-foreground/80"}`}
                    style={{ width: `${(s.pct / 31.4) * 100}%` }}
                  />
                </div>
                <span className="text-right font-[family-name:var(--font-plexmono)] text-[14px]">{s.pct}%</span>
              </li>
            ))}
          </ul>
          <p className="mt-5 text-[13px] text-muted-foreground">Example workspace, last 30 days.</p>
        </div>
      </div>
    </Section>
  );
}
