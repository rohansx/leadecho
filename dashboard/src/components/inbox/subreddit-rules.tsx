import { ExternalLink, ShieldAlert } from "lucide-react";
import type { CommunityRules, RuleWarning } from "@/lib/types";

const PROMO_LABEL: Record<CommunityRules["self_promo"], string> = {
  allowed: "Allowed",
  limited: "Limited",
  banned: "Not allowed",
  unknown: "Not stated",
};

/** ban-safe: the subreddit's rules and the draft's warnings, shown before copy/approve. */
export function SubredditRulesBox({ rules, warnings }: { rules: CommunityRules; warnings?: RuleWarning[] }) {
  const facts = [
    rules.available && ["Self-promotion", PROMO_LABEL[rules.self_promo]],
    rules.links_allowed === false && ["Links", "Not allowed"],
    rules.min_karma ? ["Min karma", String(rules.min_karma)] : null,
    rules.min_account_age && ["Min account age", rules.min_account_age],
    rules.promo_thread && ["Promo thread", rules.promo_thread],
  ].filter(Boolean) as [string, string][];

  return (
    <div className="space-y-2">
      {warnings?.map((w) => (
        <p
          key={w.code}
          role="alert"
          className="flex gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-foreground"
        >
          <ShieldAlert className="h-3.5 w-3.5 shrink-0 mt-0.5 text-amber-600 dark:text-amber-400" />
          {w.message}
        </p>
      ))}
      <div className="rounded-lg border border-border px-3 py-2 text-xs space-y-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="font-medium">r/{rules.subreddit} rules</span>
          <a
            href={rules.rules_url}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground underline-offset-2 hover:underline"
          >
            Full rules <ExternalLink className="h-3 w-3" />
          </a>
        </div>
        {!rules.available && (
          <p className="text-muted-foreground">Could not load this subreddit's rules. Read them before posting.</p>
        )}
        {facts.length > 0 && (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
            {facts.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-muted-foreground">{k}</dt>
                <dd>{v}</dd>
              </div>
            ))}
          </dl>
        )}
        {rules.quotes.length > 0 && (
          <ul className="space-y-0.5 text-foreground-soft">
            {rules.quotes.map((q) => (
              <li key={q} className="italic">"{q}"</li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
