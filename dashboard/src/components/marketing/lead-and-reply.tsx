import { Section, SectionHead } from "./section";

const profile = [
  { k: "Role", v: "Founder and engineer at Cursive Labs" },
  { k: "GitHub", v: "danielreyes, 42 repos, 310 followers" },
  { k: "Company", v: "cursivelabs.dev, found from the GitHub profile" },
  { k: "Stage", v: "Comparing vendors, has a budget" },
  { k: "Asked about", v: "Reddit monitoring that hits rate limits, a 0.82 match to your pain point" },
  { k: "Reply style", v: "Founder to founder. Name the rate-limit problem, link the install docs." },
];

const timeline = [
  { when: "Mon 09:14", who: "Draft", body: "Hey, I built LeadEcho partly because I kept hitting the same API limits. It's MIT and runs on docker compose, so the only cost is your own AI keys.", draft: true },
  { when: "Mon 09:18", who: "You", body: "Approved and posted from your own browser with the Chrome extension." },
  { when: "Mon 14:32", who: "u/build_in_public", body: "Does the scoring work with Anthropic? We already pay for Claude." },
  { when: "Wed 10:14", who: "Signed up", body: "Starter, yearly. Tracked through the reply's link." },
];

export function LeadAndReply() {
  return (
    <Section>
      <SectionHead title="Know who is asking before you answer">
        Each lead comes with the person behind the username and a reply drafted from your own docs. Nothing
        posts until you approve it, and it posts from your account.
      </SectionHead>

      <div className="mt-14 grid overflow-hidden rounded-[10px] border border-border bg-card lg:grid-cols-2">
        <div className="p-6 sm:p-8">
          <div className="flex items-baseline justify-between gap-4">
            <div>
              <div className="text-2xl font-semibold tracking-[-0.01em]">Daniel Reyes</div>
              <div className="text-[15px] text-muted-foreground">u/build_in_public on Reddit</div>
            </div>
            <div className="text-right">
              <div className="text-2xl font-bold text-signal">88%</div>
              <div className="text-[13px] text-muted-foreground">fits your customer profile</div>
            </div>
          </div>
          <dl className="mt-6 border-t border-border">
            {profile.map((r) => (
              <div key={r.k} className="grid grid-cols-[7rem_1fr] gap-4 border-b border-border py-3 text-[15px]">
                <dt className="text-muted-foreground">{r.k}</dt>
                <dd>{r.v}</dd>
              </div>
            ))}
          </dl>
        </div>

        <ol className="border-t border-border bg-background p-6 sm:p-8 lg:border-t-0 lg:border-l">
          {timeline.map((t) => (
            <li key={t.when} className="grid grid-cols-[5.5rem_1fr] gap-4 pb-5 last:pb-0">
              <span className="pt-0.5 text-[13px] text-muted-foreground">{t.when}</span>
              <div>
                <div className="text-[13px] font-semibold">{t.who}</div>
                <p
                  className={`mt-1 text-[15px] leading-relaxed ${
                    t.draft ? "rounded-lg border border-border bg-card p-3" : "text-foreground-soft"
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
