import { Section, SectionHead } from "./section";

const profile = [
  { k: "Role", v: "Founder and engineer at Cursive Labs" },
  { k: "GitHub", v: "danielreyes, 42 repos, 310 followers" },
  { k: "Company", v: "cursivelabs.dev, found from the GitHub profile" },
  { k: "Stage", v: "Comparing vendors, has a budget" },
  { k: "Asked about", v: "Reddit monitoring that hits rate limits, a 0.82 match to your pain point" },
];

const timeline = [
  { when: "Mon 09:14", who: "Draft", body: "Hey, I built LeadEcho partly because I kept hitting the same API limits. It's MIT and runs on docker compose, so the only cost is your own AI keys.", draft: true },
  { when: "Mon 09:18", who: "You", body: "Approved and posted from your own browser with the Chrome extension." },
  { when: "Mon 14:32", who: "u/build_in_public", body: "Does the scoring work with Anthropic? We already pay for Claude." },
  { when: "Wed 10:14", who: "Signed up", body: "Starter, yearly. Tracked through the reply's link." },
];

export function LeadAndReply() {
  return (
    <Section id="who">
      <SectionHead label="Who is asking" title="Know the person behind the username." muted="Then answer like one.">
        Each lead comes with a profile and a reply drafted from your own docs. Nothing posts until you approve it,
        and it posts from your account.
      </SectionHead>

      <div className="mt-10 overflow-hidden rounded-[18px] border border-border bg-card">
        <div className="p-5 sm:p-7">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="text-[22px] font-medium tracking-[-0.01em]">Daniel Reyes</div>
              <div className="text-[15px] text-muted-foreground">u/build_in_public on Reddit</div>
            </div>
            <div className="flex items-center gap-2 text-[14px] text-foreground-soft">
              <span className="rounded-full bg-yellow px-2.5 py-0.5 text-[15px] font-semibold text-[#1c1a14]">88%</span>
              fits your customer profile
            </div>
          </div>
          <dl className="mt-6 border-t border-border">
            {profile.map((r) => (
              <div key={r.k} className="grid gap-1 border-b border-border py-3 text-[15px] sm:grid-cols-[8rem_1fr] sm:gap-4">
                <dt className="text-muted-foreground">{r.k}</dt>
                <dd>{r.v}</dd>
              </div>
            ))}
          </dl>
        </div>

        <ol className="border-t border-border bg-panel p-5 sm:p-7">
          {timeline.map((t) => (
            <li key={t.when} className="grid gap-1 pb-5 last:pb-0 sm:grid-cols-[6rem_1fr] sm:gap-4">
              <span className="pt-0.5 font-[family-name:var(--font-plexmono)] text-[12px] text-muted-foreground">{t.when}</span>
              <div>
                <div className="text-[13px] font-medium">{t.who}</div>
                <p
                  className={`mt-1 text-[15px] leading-relaxed ${
                    t.draft ? "rounded-[12px] border border-border bg-card p-3.5" : "text-foreground-soft"
                  }`}
                >
                  {t.body}
                </p>
              </div>
            </li>
          ))}
        </ol>
      </div>
    </Section>
  );
}
