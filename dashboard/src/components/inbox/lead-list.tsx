import { motion } from "motion/react";
import { RefreshCw, Search } from "lucide-react";
import { useEffect, useRef } from "react";
import type { HumanProposal, InboxQueueCountsResponse, Mention } from "@/lib/types";
import { ESCALATION_KINDS, INBOX_QUEUES, MENTION_STATUSES } from "@/lib/constants";

const intentColors: Record<string, string> = {
  buy_signal: "text-primary-ink",
  recommendation_ask: "text-blue-600 dark:text-blue-400",
  comparison: "text-purple-600 dark:text-purple-400",
  complaint: "text-orange-600 dark:text-orange-400",
  general: "text-muted-foreground",
};

function scoreClass(score: number | null) {
  if (score == null) return "bg-muted text-muted-foreground";
  if (score >= 7) return "bg-primary/15 text-primary-ink";
  if (score >= 4) return "bg-blue-500/10 text-blue-700 dark:text-blue-400";
  return "bg-muted text-muted-foreground";
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / (1000 * 60));
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

function emptyMessage(queue: string, escalationKind?: string): string {
  if (queue === INBOX_QUEUES.AUTO_FLOWING) {
    return "No replies ready to review. Draft replies from Escalations to move items here.";
  }
  if (queue === INBOX_QUEUES.ESCALATIONS) {
    if (escalationKind === ESCALATION_KINDS.FLAGGED) {
      return "No mentions flagged for human review.";
    }
    if (escalationKind === ESCALATION_KINDS.NEEDS_DRAFT) {
      return "All high-intent mentions have drafts. Nice work.";
    }
    return "Escalation queue is clear.";
  }
  if (queue === INBOX_QUEUES.ALL) {
    return "No mentions match your filters.";
  }
  return "No mentions in this view.";
}

export function LeadList({
  mentions,
  listTotal,
  proposals,
  isLoading,
  selectedId,
  onSelect,
  queueFilter,
  onQueueChange,
  queueCounts,
  escalationKind,
  onEscalationKindChange,
  proposalPendingCount,
  platformFilter,
  onPlatformChange,
  platformOptions,
  statusFilter,
  onStatusChange,
  search,
  onSearchChange,
  onRefresh,
  onProposalAction,
  proposalActionPending,
  hasMore,
  onLoadMore,
  isLoadingMore,
}: {
  mentions: Mention[];
  listTotal?: number;
  proposals?: HumanProposal[];
  isLoading: boolean;
  selectedId: string | null;
  onSelect: (id: string) => void;
  queueFilter: string;
  onQueueChange: (queue: string) => void;
  queueCounts?: InboxQueueCountsResponse;
  escalationKind?: string;
  onEscalationKindChange?: (kind: string) => void;
  proposalPendingCount?: number;
  platformFilter: string;
  onPlatformChange: (platform: string) => void;
  platformOptions: { platform: string; count: number }[];
  statusFilter: string;
  onStatusChange: (status: string) => void;
  search: string;
  onSearchChange: (v: string) => void;
  onRefresh: () => void;
  onProposalAction?: (id: string, status: "accepted" | "dismissed") => void;
  proposalActionPending?: boolean;
  hasMore?: boolean;
  onLoadMore?: () => void;
  isLoadingMore?: boolean;
}) {
  const isProposalsView = queueFilter === INBOX_QUEUES.PROPOSALS;
  const isEscalationsView = queueFilter === INBOX_QUEUES.ESCALATIONS;
  const scrollRef = useRef<HTMLDivElement>(null);
  const loadMoreRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!onLoadMore || !hasMore || isProposalsView) return;
    const root = scrollRef.current;
    const target = loadMoreRef.current;
    if (!root || !target) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && !isLoadingMore) onLoadMore();
      },
      { root, rootMargin: "120px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [onLoadMore, hasMore, isLoadingMore, isProposalsView, mentions.length]);

  const queueTabs = [
    { key: INBOX_QUEUES.AUTO_FLOWING, label: "Auto-flowing" },
    { key: INBOX_QUEUES.ESCALATIONS, label: "Escalations" },
    { key: INBOX_QUEUES.ALL, label: "All mentions" },
    ...(proposalPendingCount && proposalPendingCount > 0
      ? [{ key: INBOX_QUEUES.PROPOSALS, label: "Proposals" as const }]
      : []),
  ];

  const getQueueCount = (queue: string) => {
    if (queue === INBOX_QUEUES.PROPOSALS) {
      return proposalPendingCount ?? 0;
    }
    return queueCounts?.queues.find((c) => c.queue === queue)?.count ?? 0;
  };

  const platformTotal = platformOptions.reduce((sum, p) => sum + p.count, 0);

  return (
    <section className="w-[380px] shrink-0 border-r border-border flex flex-col min-h-0 h-full bg-background">
      <div className="p-4 border-b border-border space-y-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <h2 className="font-[family-name:var(--font-head)] font-medium">Inbox</h2>
            {!isProposalsView && listTotal != null && (
              <p className="text-[11px] text-muted-foreground mt-0.5">
                {listTotal.toLocaleString()} in this view
              </p>
            )}
          </div>
          <button
            type="button"
            onClick={onRefresh}
            className="w-7 h-7 rounded-md border border-border hover:bg-accent flex items-center justify-center cursor-pointer shrink-0"
            aria-label="Refresh"
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>

        {!isProposalsView && (
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <input
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder="Search this view…"
              aria-label="Search mentions"
              className="w-full rounded-lg border border-border bg-card pl-8 pr-3 py-1.5 text-sm focus:outline-hidden focus:ring-2 focus:ring-ring/30"
            />
          </div>
        )}

        <div className="flex gap-1.5 flex-wrap">
          {queueTabs.map((t) => {
            const count = getQueueCount(t.key);
            const active = queueFilter === t.key;
            return (
              <button
                key={t.key}
                type="button"
                onClick={() => onQueueChange(t.key)}
                className={`px-2.5 py-1 rounded-full text-xs font-medium cursor-pointer transition-colors ${
                  active
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:bg-accent"
                }`}
              >
                {t.label}
                <span className="ml-1 opacity-70">{count}</span>
              </button>
            );
          })}
        </div>

        {isEscalationsView && onEscalationKindChange && queueCounts?.escalations && (
          <div className="flex gap-1.5 flex-wrap">
            {[
              {
                key: ESCALATION_KINDS.NEEDS_DRAFT,
                label: "Needs draft",
                count: queueCounts.escalations.needs_draft ?? 0,
              },
              {
                key: ESCALATION_KINDS.FLAGGED,
                label: "Needs review",
                count: queueCounts.escalations.flagged ?? 0,
              },
            ].map((t) => {
              const active = escalationKind === t.key;
              return (
                <button
                  key={t.key}
                  type="button"
                  onClick={() => onEscalationKindChange(t.key)}
                  className={`px-2 py-0.5 rounded-md text-[11px] font-medium cursor-pointer transition-colors border ${
                    active
                      ? "border-primary/40 bg-primary/10 text-primary-ink"
                      : "border-border text-muted-foreground hover:bg-accent"
                  }`}
                >
                  {t.label}
                  <span className="ml-1 opacity-70">{t.count}</span>
                </button>
              );
            })}
          </div>
        )}

        {!isProposalsView && (
          <div className="flex gap-1.5">
            {platformOptions.length > 0 && (
              <select
                value={platformFilter}
                onChange={(e) => onPlatformChange(e.target.value)}
                aria-label="Filter by platform"
                className="flex-1 min-w-0 rounded-lg border border-border bg-card px-2.5 py-1.5 text-xs font-[family-name:var(--font-sans)]"
              >
                <option value="">
                  All platforms ({platformTotal.toLocaleString()})
                </option>
                {platformOptions.map((p) => (
                  <option key={p.platform} value={p.platform}>
                    {p.platform} ({p.count.toLocaleString()})
                  </option>
                ))}
              </select>
            )}
            <select
              value={statusFilter}
              onChange={(e) => onStatusChange(e.target.value)}
              aria-label="Filter by status"
              className="flex-1 min-w-0 rounded-lg border border-border bg-card px-2.5 py-1.5 text-xs font-[family-name:var(--font-sans)]"
            >
              <option value="">All statuses</option>
              {MENTION_STATUSES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      <div ref={scrollRef} className="flex-1 min-h-0 overflow-y-auto overscroll-contain">
        {isLoading && (
          <div className="p-6 text-center text-sm text-muted-foreground">Loading…</div>
        )}

        {!isLoading && isProposalsView && (proposals?.length ?? 0) === 0 && (
          <div className="p-6 text-center text-sm text-muted-foreground">
            No pending proposals. Discovery agent suggestions will appear here.
          </div>
        )}

        {!isLoading && !isProposalsView && mentions.length === 0 && (
          <div className="p-6 text-center text-sm text-muted-foreground">
            {emptyMessage(queueFilter, escalationKind)}
          </div>
        )}

        {!isProposalsView &&
          mentions.map((m, i) => {
            const flagged = Boolean(m.scoring_metadata?.needs_escalation);
            return (
              <motion.button
                key={m.id}
                onClick={() => onSelect(m.id)}
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.25, delay: Math.min(i, 8) * 0.03 }}
                className={`w-full text-left px-4 py-3.5 border-b border-border flex gap-3 transition-colors cursor-pointer ${
                  selectedId === m.id ? "bg-accent-soft/60" : "hover:bg-accent/50"
                }`}
              >
                <div
                  className={`shrink-0 h-9 w-9 rounded-lg flex flex-col items-center justify-center text-[13px] font-[family-name:var(--font-head)] font-medium ${scoreClass(m.relevance_score)}`}
                >
                  {m.relevance_score != null ? m.relevance_score.toFixed(1) : "–"}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1 flex-wrap">
                    <span className="font-medium text-foreground-soft">{m.platform}</span>
                    {m.intent && (
                      <>
                        <span>·</span>
                        <span className={intentColors[m.intent] ?? ""}>
                          {m.intent.replace("_", " ")}
                        </span>
                      </>
                    )}
                    {flagged && (
                      <>
                        <span>·</span>
                        <span className="text-orange-600 dark:text-orange-400">needs review</span>
                      </>
                    )}
                    {isEscalationsView && !flagged && m.status === "new" && (
                      <>
                        <span>·</span>
                        <span className="text-blue-600 dark:text-blue-400">needs draft</span>
                      </>
                    )}
                    <span className="ml-auto shrink-0">
                      {timeAgo(m.platform_created_at ?? m.created_at)}
                    </span>
                  </div>
                  <p className="text-sm text-foreground line-clamp-2 leading-snug">{m.content}</p>
                  <div className="mt-1 text-xs text-muted-foreground truncate">
                    @{m.author_username ?? "unknown"}
                  </div>
                </div>
              </motion.button>
            );
          })}

        {!isProposalsView && hasMore && (
          <div ref={loadMoreRef} className="py-4 text-center text-xs text-muted-foreground">
            {isLoadingMore ? "Loading more…" : "Scroll for more"}
          </div>
        )}

        {!isProposalsView && !hasMore && mentions.length > 0 && listTotal != null && (
          <div className="py-4 text-center text-xs text-muted-foreground">
            End of list · {mentions.length.toLocaleString()} loaded
          </div>
        )}

        {isProposalsView &&
          proposals?.map((p, i) => (
            <motion.div
              key={p.id}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.25, delay: Math.min(i, 8) * 0.03 }}
              className={`w-full text-left px-4 py-3.5 border-b border-border ${
                selectedId === p.id ? "bg-accent-soft/60" : ""
              }`}
            >
              <button type="button" onClick={() => onSelect(p.id)} className="w-full text-left cursor-pointer">
                <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                  <span className="font-medium text-foreground-soft">{p.proposal_type}</span>
                  <span className="ml-auto shrink-0">{timeAgo(p.created_at)}</span>
                </div>
                <p className="text-sm font-medium text-foreground line-clamp-1">{p.title}</p>
                <p className="text-sm text-muted-foreground line-clamp-2 leading-snug mt-1">{p.body}</p>
              </button>
              {p.status === "pending" && onProposalAction && (
                <div className="mt-2 flex gap-2">
                  <button
                    type="button"
                    disabled={proposalActionPending}
                    onClick={() => onProposalAction(p.id, "accepted")}
                    className="px-2 py-1 text-xs rounded-md bg-primary text-primary-foreground cursor-pointer disabled:opacity-50"
                  >
                    Accept
                  </button>
                  <button
                    type="button"
                    disabled={proposalActionPending}
                    onClick={() => onProposalAction(p.id, "dismissed")}
                    className="px-2 py-1 text-xs rounded-md border border-border hover:bg-accent cursor-pointer disabled:opacity-50"
                  >
                    Dismiss
                  </button>
                </div>
              )}
            </motion.div>
          ))}
      </div>
    </section>
  );
}
