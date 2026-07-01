import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import {
  ArchiveIcon,
  ArrowLeft,
  ArrowRight,
  Brain,
  Check,
  Copy,
  ExternalLink,
  Loader2,
  MessageSquareText,
  Sparkles,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { classifyMention, draftReply, listReplies, updateMentionStatus } from "@/lib/api";
import type { Mention, Reply } from "@/lib/types";

const awarenessLabels: Record<string, string> = {
  problem_aware: "Problem aware",
  solution_aware: "Solution aware",
  product_aware: "Product aware",
  purchase_ready: "Purchase ready",
};

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const hours = Math.floor(diff / (1000 * 60 * 60));
  if (hours < 1) return "just now";
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function SignalRow({ label, value }: { label: string; value: React.ReactNode }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex items-start justify-between gap-4 py-2 text-sm border-b border-border last:border-0">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className="text-right text-foreground-soft">{value}</span>
    </div>
  );
}

function OverviewTab({ mention }: { mention: Mention }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Awaited<ReturnType<typeof draftReply>> | null>(null);
  const [copied, setCopied] = useState(false);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["mentions"] });
    queryClient.invalidateQueries({ queryKey: ["mentionCounts"] });
    queryClient.invalidateQueries({ queryKey: ["mentionTierCounts"] });
  };

  const classifyMutation = useMutation({
    mutationFn: () => classifyMention(mention.id),
    onSuccess: invalidate,
  });

  const draftMutation = useMutation({
    mutationFn: () => draftReply(mention.id),
    onSuccess: (data) => {
      setDraft(data);
      invalidate();
    },
  });

  const markRepliedMutation = useMutation({
    mutationFn: () => updateMentionStatus(mention.id, "replied"),
    onSuccess: invalidate,
  });

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1800);
  };

  return (
    <div className="space-y-5">
      <article className="rounded-xl border border-border bg-card p-5">
        <div className="flex items-center gap-2 text-xs text-muted-foreground mb-3">
          <span className="font-medium text-foreground-soft">{mention.platform}</span>
          <span>·</span>
          <span>@{mention.author_username ?? "unknown"}</span>
          <a
            href={mention.url}
            target="_blank"
            rel="noreferrer"
            className="ml-auto inline-flex items-center gap-1 text-primary-ink hover:underline"
          >
            View original <ExternalLink className="h-3 w-3" />
          </a>
        </div>
        {mention.title && (
          <h3 className="font-[family-name:var(--font-head)] text-lg font-medium mb-2">{mention.title}</h3>
        )}
        <p className="text-sm text-foreground-soft leading-relaxed whitespace-pre-wrap">{mention.content}</p>
      </article>

      <div className="flex flex-wrap gap-2">
        {!mention.intent && (
          <Button size="sm" variant="outline" onClick={() => classifyMutation.mutate()} disabled={classifyMutation.isPending}>
            {classifyMutation.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Brain className="h-3.5 w-3.5" />
            )}
            Classify
          </Button>
        )}
        <Button size="sm" onClick={() => draftMutation.mutate()} disabled={draftMutation.isPending}>
          {draftMutation.isPending ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Sparkles className="h-3.5 w-3.5" />
          )}
          Draft reply
        </Button>
        {mention.status !== "replied" && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => markRepliedMutation.mutate()}
            disabled={markRepliedMutation.isPending}
          >
            <Check className="h-3.5 w-3.5" />
            Mark as replied
          </Button>
        )}
      </div>

      <AnimatePresence>
        {draft && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.25 }}
            className="overflow-hidden"
          >
            <div className="rounded-xl border border-primary/30 bg-accent-soft/40 p-4">
              {!draft.should_reply ? (
                <>
                  <div className="text-sm font-medium mb-1">Not worth replying</div>
                  <p className="text-sm text-foreground-soft">{draft.reason}</p>
                </>
              ) : draft.reply ? (
                <>
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-1.5 flex-wrap text-xs">
                      <span className="font-medium text-sm mr-1">AI draft</span>
                      {draft.tone && <span className="rounded-full bg-muted px-2 py-0.5">{draft.tone}</span>}
                      {draft.template_style && (
                        <span className="rounded-full bg-muted px-2 py-0.5">
                          {draft.template_style.replace("_", " ")}
                        </span>
                      )}
                      {draft.awareness_level && (
                        <span className="rounded-full bg-accent-soft text-primary-ink px-2 py-0.5">
                          {awarenessLabels[draft.awareness_level] ?? draft.awareness_level}
                        </span>
                      )}
                    </div>
                    <Button size="sm" variant="ghost" onClick={() => handleCopy(draft.reply!.content)}>
                      {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                      {copied ? "Copied" : "Copy"}
                    </Button>
                  </div>
                  <p className="text-sm leading-relaxed whitespace-pre-wrap">{draft.reply.content}</p>
                </>
              ) : null}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function ThreadTab({ mentionId }: { mentionId: string }) {
  const { data: replies, isLoading } = useQuery({
    queryKey: ["replies", mentionId],
    queryFn: () => listReplies(mentionId),
  });

  if (isLoading) return <div className="text-sm text-muted-foreground py-8 text-center">Loading thread…</div>;

  if (!replies || replies.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
        <MessageSquareText className="h-5 w-5 mx-auto mb-2 opacity-50" />
        No replies logged yet. Draft one from the Overview tab — once you post it, it'll show up here.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {replies.map((r: Reply) => (
        <div key={r.id} className="rounded-xl border border-border bg-card p-4">
          <div className="flex items-center gap-2 text-xs text-muted-foreground mb-1.5">
            <span className="rounded-full bg-muted px-2 py-0.5 capitalize">{r.status}</span>
            {r.template_style && <span>{r.template_style.replace("_", " ")}</span>}
            <span className="ml-auto">{timeAgo(r.created_at)}</span>
          </div>
          <p className="text-sm leading-relaxed whitespace-pre-wrap">{r.edited_content ?? r.content}</p>
        </div>
      ))}
    </div>
  );
}

function SignalsPanel({ mention }: { mention: Mention }) {
  const meta = mention.platform_metadata as Record<string, unknown>;
  const subreddit = typeof meta?.subreddit === "string" ? meta.subreddit : null;

  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-2">Signals</h5>
      <SignalRow label="Platform" value={<span className="capitalize">{mention.platform}</span>} />
      {subreddit && <SignalRow label="Community" value={subreddit} />}
      <SignalRow label="Author" value={mention.author_username ? `@${mention.author_username}` : null} />
      <SignalRow label="Karma" value={mention.author_karma} />
      <SignalRow
        label="Account age"
        value={mention.author_account_age_days != null ? `${mention.author_account_age_days}d` : null}
      />
      <SignalRow
        label="Intent"
        value={mention.intent ? mention.intent.replace("_", " ") : null}
      />
      <SignalRow
        label="Awareness"
        value={mention.awareness_level ? awarenessLabels[mention.awareness_level] : null}
      />
      <SignalRow
        label="Relevance"
        value={mention.relevance_score != null ? `${mention.relevance_score.toFixed(1)} / 10` : null}
      />
      <SignalRow
        label="Conversion prob."
        value={mention.conversion_probability != null ? `${Math.round(mention.conversion_probability * 100)}%` : null}
      />
      {mention.keyword_matches?.length > 0 && (
        <div className="pt-2">
          <div className="text-xs text-muted-foreground mb-1.5">Keyword matches</div>
          <div className="flex flex-wrap gap-1">
            {mention.keyword_matches.map((k) => (
              <span key={k} className="text-xs rounded-full bg-muted px-2 py-0.5">
                {k}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export function LeadDetail({
  mention,
  onPrev,
  onNext,
  hasPrev,
  hasNext,
  onArchive,
  archiving,
}: {
  mention: Mention | null;
  onPrev: () => void;
  onNext: () => void;
  hasPrev: boolean;
  hasNext: boolean;
  onArchive: () => void;
  archiving: boolean;
}) {
  const [tab, setTab] = useState<"overview" | "thread">("overview");

  if (!mention) {
    return (
      <section className="flex-1 flex items-center justify-center text-sm text-muted-foreground">
        Select a mention to see the details.
      </section>
    );
  }

  return (
    <section className="flex-1 flex flex-col h-full overflow-hidden">
      <div className="flex items-center gap-3 px-6 h-14 border-b border-border shrink-0">
        <div className="text-sm text-muted-foreground truncate">
          Inbox / <b className="text-foreground">{mention.platform}</b> · {timeAgo(mention.created_at)}
        </div>
        <div className="ml-auto flex items-center gap-1.5">
          <button
            onClick={onPrev}
            disabled={!hasPrev}
            className="h-7 w-7 rounded-md border border-border hover:bg-accent disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center cursor-pointer"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={onNext}
            disabled={!hasNext}
            className="h-7 w-7 rounded-md border border-border hover:bg-accent disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center cursor-pointer"
          >
            <ArrowRight className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={onArchive}
            disabled={archiving}
            className="h-7 w-7 rounded-md border border-border hover:bg-accent flex items-center justify-center cursor-pointer"
            aria-label="Archive"
          >
            <ArchiveIcon className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <div className="flex gap-1 px-6 pt-3 border-b border-border shrink-0">
        {(["overview", "thread"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`relative px-3 pb-3 text-sm font-medium capitalize cursor-pointer transition-colors ${
              tab === t ? "text-foreground" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t}
            {tab === t && (
              <motion.div layoutId="detail-tab" className="absolute left-0 right-0 -bottom-px h-0.5 bg-primary" />
            )}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-6 grid lg:grid-cols-[1fr_280px] gap-6 items-start">
        <AnimatePresence mode="wait">
          <motion.div
            key={`${mention.id}-${tab}`}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
          >
            {tab === "overview" ? <OverviewTab mention={mention} /> : <ThreadTab mentionId={mention.id} />}
          </motion.div>
        </AnimatePresence>
        <SignalsPanel mention={mention} />
      </div>
    </section>
  );
}
