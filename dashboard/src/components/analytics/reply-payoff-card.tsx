import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Trophy } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Text } from "@/components/ui/text";
import { replyPayoff, type PayoffTotal } from "@/lib/api";
import { QUERY_KEYS } from "@/lib/constants";
import { safeHttpUrl } from "@/lib/outreach";
import { timeAgo } from "@/lib/pipeline";
import { platformTone } from "@/lib/tones";

const extLink = "text-muted-foreground underline underline-offset-2 hover:text-foreground";

// "Replies that paid off": every reply the customer posted (90 days) with the
// clicks on its tracked link and the sign-ups their site reported for it.
export function ReplyPayoffCard() {
  const { data } = useQuery({ queryKey: [QUERY_KEYS.replyPayoff], queryFn: replyPayoff });
  const rows = data?.replies ?? [];

  return (
    <Card id="reply-payoff">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Trophy className="h-4 w-4" /> Replies that paid off (90d)
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {data && (
          <Text as="p" className="text-sm tabular-nums">
            {data.totals.replies} posted · {data.totals.clicks} clicks · {data.totals.signups} sign-ups
          </Text>
        )}
        {data && rows.length === 0 && (
          <Text as="p" className="text-xs text-muted-foreground">
            No posted replies yet. After you post a reply, press "I posted it" on the lead. Clicks on its tracked link are
            counted here; report sign-ups from your server with POST /api/public/v1/conversions (an API key with write
            access, and the utm_content your landing page received).
          </Text>
        )}

        {rows.length > 0 && (
          <div className="divide-y divide-border">
            {rows.map((r) => {
              const thread = safeHttpUrl(r.thread_url);
              const comment = safeHttpUrl(r.posted_url);
              return (
                <div key={r.reply_id} className="flex items-center gap-3 py-2">
                  <div className="min-w-0 flex-1">
                    <Link
                      to="/inbox"
                      search={{ queue: "all", id: r.mention_id }}
                      className="block truncate text-sm font-medium hover:underline"
                    >
                      {r.title || r.thread_url}
                    </Link>
                    <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                      <Badge className={platformTone(r.platform)} size="sm">{r.platform}</Badge>
                      {r.keyword && <span>{r.keyword}</span>}
                      <span>{r.posted_at ? `posted ${timeAgo(r.posted_at)}` : "clicked, not marked posted"}</span>
                      {thread && <a href={thread} target="_blank" rel="noopener noreferrer" className={extLink}>thread</a>}
                      {comment && <a href={comment} target="_blank" rel="noopener noreferrer" className={extLink}>your comment</a>}
                    </div>
                  </div>
                  <div className="shrink-0 text-right text-xs tabular-nums">
                    <div>{r.clicks} clicks</div>
                    <div className={r.signups > 0 ? "font-semibold text-emerald-600" : "text-muted-foreground"}>
                      {r.signups} sign-ups
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {rows.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Totals title="By platform" items={data?.by_platform ?? []} />
            <Totals title="By keyword" items={data?.by_keyword ?? []} />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function Totals({ title, items }: { title: string; items: PayoffTotal[] }) {
  return (
    <div>
      <h5 className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</h5>
      {items.slice(0, 6).map((t) => (
        <div key={t.key} className="flex items-center justify-between py-0.5 text-sm">
          <span className="truncate">{t.key}</span>
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {t.replies} replies · {t.clicks} clicks · {t.signups} sign-ups
          </span>
        </div>
      ))}
    </div>
  );
}
