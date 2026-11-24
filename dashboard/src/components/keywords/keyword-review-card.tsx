import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Gavel, Sparkles, X } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Text } from "@/components/ui/text";
import { createKeyword, updateKeyword } from "@/lib/api";
import type { Keyword, Platform } from "@/lib/types";
import {
  decideSuggestion,
  getKeywordReview,
  requestKeywordReview,
  type KeywordSuggestion,
} from "@/lib/keyword-judge-api";
import { topUpCount } from "@/lib/keyword-top-up";
import { GenerateKeywordsDialog } from "./generate-keywords-dialog";
import { ScoringRules } from "./scoring-rules";

const KIND_LABEL: Record<KeywordSuggestion["kind"], string> = {
  add: "Add",
  pause: "Pause",
  platforms: "Move",
  keep: "Keep",
};

const AUTO_LABEL: Partial<Record<KeywordSuggestion["kind"], string>> = { add: "Added", pause: "Paused", platforms: "Narrowed" };

interface KeywordReviewCardProps {
  keywords: Keyword[];
}

/**
 * The keyword judge's report: what it found working and not, and one-click
 * changes. Applying or dismissing is remembered, and the next weekly review
 * sees how the applied changes did.
 */
export function KeywordReviewCard({ keywords }: KeywordReviewCardProps) {
  const qc = useQueryClient();
  const [generating, setGenerating] = useState(false);
  const { data } = useQuery({
    queryKey: ["keyword-review"],
    queryFn: getKeywordReview,
    // While a review runs, look again every 15 seconds.
    refetchInterval: (q) => (q.state.data?.scan && q.state.data.scan.state !== "done" ? 15_000 : false),
  });

  const run = useMutation({
    mutationFn: requestKeywordReview,
    onSettled: () => qc.invalidateQueries({ queryKey: ["keyword-review"] }),
  });

  const act = useMutation({
    mutationFn: async ({ s, decision }: { s: KeywordSuggestion; decision: "applied" | "dismissed" }) => {
      if (decision === "applied") {
        const kw = keywords.find((k) => k.term.toLowerCase() === s.term.toLowerCase());
        if (s.kind === "add") {
          if (!kw) await createKeyword({ term: s.term, platforms: s.platforms, match_type: "broad" });
          else if (!kw.is_active) await updateKeyword(kw.id, { is_active: true }); // a paused copy: switch it back on
        } else if (!kw) throw new Error(`"${s.term}" is no longer one of your keywords`);
        else if (s.kind === "pause") await updateKeyword(kw.id, { is_active: false });
        else if (s.kind === "platforms") {
          if (!s.platforms?.length) throw new Error("This suggestion names no platforms; dismiss it instead");
          await updateKeyword(kw.id, { platforms: s.platforms as Platform[] });
        }
      }
      await decideSuggestion(s.id, decision);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["keyword-review"] });
      qc.invalidateQueries({ queryKey: ["keywords"] });
    },
  });

  if (!data) return null;
  const review = data.review;
  const running = data.scan != null && data.scan.state !== "done";
  const open = (review?.suggestions ?? []).filter((s) => s.kind !== "keep" && !review?.decisions?.[s.id]);
  const kept = (review?.suggestions ?? []).filter((s) => s.kind === "keep");
  const auto = (review?.suggestions ?? []).filter((s) => review?.decisions?.[s.id] === "auto");
  const count = topUpCount(data);

  return (
    <Card className="min-w-0">
      <CardHeader>
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-secondary">
            <Gavel className="h-4 w-4 text-foreground-soft" />
          </div>
          {/* basis 12rem: on a phone the buttons wrap to their own line instead of squeezing the text */}
          <div className="min-w-0 flex-1 basis-48">
            <CardTitle>Keyword review</CardTitle>
            <CardDescription>
              {review
                ? `Read ${review.posts} posts and ${review.leads} leads from the last ${review.days} days · ${new Date(review.generated_at).toLocaleDateString()}${review.by === "weekly" ? " (weekly)" : ""}`
                : "Every week an AI judge reads what your keywords found, as you would. Once a keyword has had a week of posts, the judge pauses or narrows it if it found nothing worth replying to, and adds keywords that match how your buyers write."}
            </CardDescription>
          </div>
          <div className="flex gap-2">
            {data.allowed && count > 0 && (
              <Button size="sm" variant="outline" onClick={() => setGenerating(true)}>
                <Sparkles className="mr-1 h-3.5 w-3.5" /> Generate keywords
              </Button>
            )}
            <Button size="sm" variant="accent" disabled={!data.can_review || running || run.isPending} onClick={() => run.mutate()}
              title={data.can_review ? "Takes a minute or two" : "A review is running, or was made in the last 6 hours"}>
              {running ? "Reviewing…" : "Review now"}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {run.isError && <Text as="p" className="text-sm text-destructive">{run.error.message}</Text>}
        {running && <Text as="p" className="text-sm text-muted-foreground">The judge is reading your posts and leads. This page updates when it is done.</Text>}
        {data.scan?.state === "done" && data.scan.note?.startsWith("the keyword review failed") && (
          <Text as="p" className="text-sm text-destructive">The last review failed. Try again in a few minutes.</Text>
        )}
        {review && (
          <>
            <div className="space-y-2 rounded-lg border border-border bg-muted/40 p-3">
              <Text as="p" className="text-sm">{review.summary}</Text>
              {review.lead_quality && <Text as="p" className="text-xs text-muted-foreground">Leads: {review.lead_quality}</Text>}
            </div>

            {auto.length > 0 && (
              <div className="space-y-1.5 rounded-lg border border-border p-3">
                <Text as="p" className="text-sm font-medium">Changed by the judge</Text>
                <ul className="space-y-1">
                  {auto.map((s) => (
                    <li key={s.id} className="text-xs text-muted-foreground">
                      <span className="font-medium text-foreground">{AUTO_LABEL[s.kind] ?? s.kind}</span> “{s.term}”
                      {s.kind !== "pause" && s.platforms && s.platforms.length > 0 ? ` (${s.platforms.join(" · ")})` : ""}: {s.why}
                    </li>
                  ))}
                </ul>
                <Text as="p" className="text-xs text-muted-foreground">Disagree? Turn a keyword back on or edit it in the list below.</Text>
              </div>
            )}

            {open.length > 0 ? (
              <div className="space-y-2">
                {open.map((s) => (
                  <div key={s.id} className="flex items-start gap-3 rounded-lg border border-border p-3">
                    <Badge variant="outline" size="sm" className="mt-0.5 shrink-0">{KIND_LABEL[s.kind]}</Badge>
                    <div className="min-w-0 flex-1">
                      <Text as="p" className="text-sm font-medium">{s.term}</Text>
                      <Text as="p" className="text-xs text-muted-foreground">{s.why}</Text>
                      {s.platforms && s.platforms.length > 0 && (
                        <Text as="p" className="mt-0.5 text-xs text-muted-foreground">
                          {s.kind === "platforms" ? "Search only on: " : ""}{s.platforms.join(" · ")}
                        </Text>
                      )}
                    </div>
                    <div className="flex shrink-0 gap-1">
                      <Button size="sm" variant="outline" disabled={act.isPending} onClick={() => act.mutate({ s, decision: "applied" })} title="Apply this change">
                        <Check className="h-3.5 w-3.5" />
                      </Button>
                      <Button size="sm" variant="ghost" disabled={act.isPending} onClick={() => act.mutate({ s, decision: "dismissed" })} title="Dismiss">
                        <X className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                ))}
                {act.isError && <Text as="p" className="text-sm text-destructive">{act.error.message}</Text>}
              </div>
            ) : (
              <Text as="p" className="text-sm text-muted-foreground">No open suggestions. The next review will check how your changes did.</Text>
            )}

            <ScoringRules rules={data.rules ?? { rules: [], removed: [] }} />

            {kept.length > 0 && (
              <Text as="p" className="text-xs text-muted-foreground">
                Working, keep: {kept.map((s) => s.term).join(", ")}
              </Text>
            )}
            {review.scoring.length > 0 && (
              <details className="text-xs text-muted-foreground">
                <summary className="cursor-pointer">Posts the judge thinks were scored wrongly ({review.scoring.length})</summary>
                <ul className="mt-2 list-disc space-y-1 pl-5">
                  {review.scoring.map((n, i) => (
                    <li key={i}><span className="italic">“{n.post}”</span>: {n.note}</li>
                  ))}
                </ul>
              </details>
            )}
          </>
        )}
      </CardContent>
      {generating && <GenerateKeywordsDialog count={count} onClose={() => setGenerating(false)} />}
    </Card>
  );
}
