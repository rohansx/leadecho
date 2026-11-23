import { motion } from "motion/react";
import { stripMarkdown } from "@/lib/markdown";
import { RefreshCw, Search } from "lucide-react";
import { useEffect, useRef } from "react";
import type { HumanProposal, InboxQueueCountsResponse, Mention } from "@/lib/types";
import { ACTION_KINDS, INBOX_QUEUES, INTENT_TAGS, MENTION_STATUSES, SENTIMENTS } from "@/lib/constants";
import { TagChips, tagLabel } from "@/components/inbox/intent-tags";
import { hiringText } from "@/lib/lead-age";
import { PlatformLogo } from "@/components/ui/platform-logo";
import { ExportCsvButton } from "@/components/sources/export-csv-button"; // sources
import { SetupChecklist } from "@/components/onboarding/setup-checklist";
import type { SetupStatus } from "@/lib/setup-status";

const actionLabel: Record<string, string> = {
  ready_to_send: "Review & send",
  needs_draft: "Draft reply",
  needs_review: "Needs review",
};

// Gold is reserved for the top band, so a gold tile means "look here first".
function scoreClass(score: number | null) {
  if (score == null || score < 7) return "border border-border text-muted-foreground";
  if (score >= 8.5) return "bg-primary/15 text-primary-ink";
  return "bg-secondary text-foreground";
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / (1000 * 60));
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

function queueSubtitle(
  queue: string,
  listTotal: number,
  actions?: InboxQueueCountsResponse["actions"],
  actionKind?: string,
): string | null {
  if (queue === INBOX_QUEUES.ACTION_REQUIRED && actionKind === ACTION_KINDS.OLDER) {
    return `${listTotal} older · posts over 7 days old, company leads over 10`;
  }
  if (queue === INBOX_QUEUES.ACTION_REQUIRED && actions) {
    const parts: string[] = [];
    if (actions.ready_to_send > 0) parts.push(`${actions.ready_to_send} ready to send`);
    if (actions.needs_draft > 0) parts.push(`${actions.needs_draft} need draft`);
    if (actions.needs_review > 0) parts.push(`${actions.needs_review} need review`);
    return parts.length > 0 ? parts.join(" · ") : `${listTotal} need attention`;
  }
  if (queue === INBOX_QUEUES.ALL) {
    return `${listTotal.toLocaleString()} mentions`;
  }
  return null;
}

function emptyMessage(queue: string, actionKind?: string): string {
  if (queue === INBOX_QUEUES.ACTION_REQUIRED) {
    if (actionKind === ACTION_KINDS.READY_TO_SEND) {
      return "Nothing ready to send. Draft replies from high-intent mentions first.";
    }
    if (actionKind === ACTION_KINDS.NEEDS_DRAFT) {
      return "All caught up — no mentions waiting for a draft.";
    }
    if (actionKind === ACTION_KINDS.NEEDS_REVIEW) {
      return "No mentions flagged for human review.";
    }
    if (actionKind === ACTION_KINDS.OLDER) {
      return "No older leads.";
    }
    return "Inbox zero. New high-intent mentions will appear here.";
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
  actionKind,
  onActionKindChange,
  proposalPendingCount,
  platformFilter,
  onPlatformChange,
  platformOptions,
  statusFilter,
  onStatusChange,
  tagFilter = "",
  onTagChange,
  sentimentFilter = "",
  onSentimentChange,
  search,
  onSearchChange,
  onRefresh,
  onProposalAction,
  proposalActionPending,
  hasMore,
  onLoadMore,
  isLoadingMore,
  agentActive,
  agentNeedsAI,
  activeKeywordCount,
  totalMentions,
  setup,
}: {
  mentions: Mention[];
  listTotal?: number;
  proposals?: HumanProposal[];
  isLoading: boolean;
  selectedId: string | null;
  /** tab: open the lead on that tab (clicking the author opens Person360). */
  onSelect: (id: string, tab?: "person360") => void;
  queueFilter: string;
  onQueueChange: (queue: string) => void;
  queueCounts?: InboxQueueCountsResponse;
  actionKind?: string;
  onActionKindChange?: (kind: string) => void;
  proposalPendingCount?: number;
  platformFilter: string;
  onPlatformChange: (platform: string) => void;
  platformOptions: { platform: string; count: number }[];
  statusFilter: string;
  onStatusChange: (status: string) => void;
  tagFilter?: string;
  onTagChange?: (tag: string) => void;
  sentimentFilter?: string;
  onSentimentChange?: (sentiment: string) => void;
  search: string;
  onSearchChange: (v: string) => void;
  onRefresh: () => void;
  onProposalAction?: (id: string, status: "accepted" | "dismissed") => void;
  proposalActionPending?: boolean;
  hasMore?: boolean;
  onLoadMore?: () => void;
  isLoadingMore?: boolean;
  agentActive?: boolean;
  agentNeedsAI?: boolean;
  activeKeywordCount?: number;
  totalMentions?: number;
  /** Set while the workspace may have no lead yet; the checklist shows until it has one. */
  setup?: SetupStatus;
}) {
  const isProposalsView = queueFilter === INBOX_QUEUES.PROPOSALS;
  const isActionView = queueFilter === INBOX_QUEUES.ACTION_REQUIRED;
  const isBrowseView = queueFilter === INBOX_QUEUES.ALL;
  const waitingForFirstLead = Boolean(setup && !setup.has_lead);
  const scrollRef = useRef<HTMLDivElement>(null);
  const loadMoreRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!onLoadMore || !hasMore || !isBrowseView) return;
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
  }, [onLoadMore, hasMore, isLoadingMore, isBrowseView, mentions.length]);

  const queueTabs = [
    { key: INBOX_QUEUES.ACTION_REQUIRED, label: "Leads" },
    { key: INBOX_QUEUES.ALL, label: "All conversations" },
    ...(proposalPendingCount && proposalPendingCount > 0
      ? [{ key: INBOX_QUEUES.PROPOSALS, label: "Proposals" as const }]
      : []),
  ];

  const getQueueCount = (q: string) =>
    queueCounts?.queues.find((c) => c.queue === q)?.count ??
    (q === INBOX_QUEUES.PROPOSALS ? (proposalPendingCount ?? 0) : 0);

  const platformTotal = platformOptions.reduce((sum, p) => sum + p.count, 0);
  const subtitle =
    !isProposalsView && listTotal != null
      ? queueSubtitle(queueFilter, listTotal, queueCounts?.actions, actionKind)
      : null;

  const actionFilters = [
    { key: "", label: "All", count: getQueueCount(INBOX_QUEUES.ACTION_REQUIRED) },
    {
      key: ACTION_KINDS.READY_TO_SEND,
      label: "Ready to send",
      count: queueCounts?.actions.ready_to_send ?? 0,
    },
    {
      key: ACTION_KINDS.NEEDS_DRAFT,
      label: "Needs draft",
      count: queueCounts?.actions.needs_draft ?? 0,
    },
    {
      key: ACTION_KINDS.NEEDS_REVIEW,
      label: "Needs review",
      count: queueCounts?.actions.needs_review ?? 0,
    },
    { key: ACTION_KINDS.OLDER, label: "Older", count: queueCounts?.actions.older ?? 0 },
  ];

  return (
    <section className="w-full md:w-[380px] shrink-0 border-r border-border flex flex-col min-h-0 h-full bg-background">
      <div className="p-4 border-b border-border space-y-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <h2 className="font-[family-name:var(--font-head)] font-medium">Inbox</h2>
            {subtitle && (
              <p className="text-[11px] text-muted-foreground mt-0.5">{subtitle}</p>
            )}
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            {/* sources: CSV export of the current view */}
            {queueFilter !== INBOX_QUEUES.PROPOSALS && (
              <ExportCsvButton
                kind="mentions"
                params={
                  queueFilter === INBOX_QUEUES.ALL
                    ? { queue: queueFilter, platform: platformFilter, status: statusFilter, search }
                    : { queue: queueFilter, action_kind: queueFilter === INBOX_QUEUES.ACTION_REQUIRED ? actionKind : undefined }
                }
              />
            )}
            <button
              type="button"
              onClick={onRefresh}
              className="w-7 h-7 rounded-md border border-border hover:bg-accent flex items-center justify-center cursor-pointer shrink-0"
              aria-label="Refresh"
            >
              <RefreshCw className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>

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
                    ? "bg-foreground text-background"
                    : "bg-muted text-muted-foreground hover:bg-accent"
                }`}
              >
                {t.label}
                <span className="ml-1 opacity-70">{count}</span>
              </button>
            );
          })}
        </div>

        {isActionView && onActionKindChange && queueCounts?.actions && (
          <div className="flex gap-1.5 flex-wrap">
            {actionFilters.map((t) => {
              const active = (actionKind ?? "") === t.key;
              return (
                <button
                  key={t.key || "all"}
                  type="button"
                  onClick={() => onActionKindChange(t.key)}
                  className={`px-2 py-0.5 rounded-md text-[11px] font-medium cursor-pointer transition-colors border ${
                    active
                      ? "border-foreground bg-foreground text-background"
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

        {isBrowseView && (
          <>
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
              <input
                value={search}
                onChange={(e) => onSearchChange(e.target.value)}
                placeholder="Search mentions…"
                aria-label="Search mentions"
                className="w-full rounded-lg border border-border bg-card pl-8 pr-3 py-1.5 text-sm focus:outline-hidden focus:ring-2 focus:ring-ring/30"
              />
            </div>
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
            {onTagChange && onSentimentChange && (
              <div className="flex gap-1.5">
                <select
                  value={tagFilter}
                  onChange={(e) => onTagChange(e.target.value)}
                  aria-label="Filter by intent tag"
                  className="flex-1 min-w-0 rounded-lg border border-border bg-card px-2.5 py-1.5 text-xs font-[family-name:var(--font-sans)]"
                >
                  <option value="">All intent tags</option>
                  {INTENT_TAGS.map((t) => (
                    <option key={t} value={t}>
                      {tagLabel(t)}
                    </option>
                  ))}
                </select>
                <select
                  value={sentimentFilter}
                  onChange={(e) => onSentimentChange(e.target.value)}
                  aria-label="Filter by sentiment"
                  className="flex-1 min-w-0 rounded-lg border border-border bg-card px-2.5 py-1.5 text-xs font-[family-name:var(--font-sans)]"
                >
                  <option value="">Any sentiment</option>
                  {SENTIMENTS.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
              </div>
            )}
          </>
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

        {/* No lead yet: what the workspace is waiting for, instead of an empty inbox. */}
        {!isLoading && !isProposalsView && waitingForFirstLead && setup && (
          <div className="m-3 rounded-lg border border-border p-4">
            <p className="mb-2 text-sm font-medium">{setup.finished_empty ? "No leads yet" : "Finding your first leads"}</p>
            <SetupChecklist status={setup} />
            {agentNeedsAI && (
              <p className="mt-2 text-xs text-orange-600 dark:text-orange-400">
                No AI key configured: posts are not scored. Add one in Settings → AI Router.
              </p>
            )}
          </div>
        )}

        {!isLoading && !isProposalsView && mentions.length === 0 && !waitingForFirstLead && (
          <div className="p-6 text-center text-sm text-muted-foreground space-y-4">
            {/* Agent status indicator — shown when no mentions yet but agent is active */}
            {agentActive && totalMentions === 0 && (
              <div className="space-y-3 py-4">
                <div className="flex items-center justify-center gap-2">
                  <span className="relative flex h-3 w-3">
                    <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-muted-foreground opacity-60"></span>
                    <span className="relative inline-flex h-3 w-3 rounded-full bg-muted-foreground"></span>
                  </span>
                  <span className="text-foreground font-medium text-sm">Agent is live & scanning</span>
                </div>
                <div className="space-y-1 text-xs text-muted-foreground">
                  <p>
                    Monitoring <span className="text-foreground-soft font-medium">{activeKeywordCount}</span>{" "}
                    {activeKeywordCount === 1 ? "keyword" : "keywords"} across platforms
                  </p>
                  <p>Scans every 5 minutes · Next results will appear automatically</p>
                  {agentNeedsAI && (
                    <p className="text-orange-600 dark:text-orange-400 mt-2">
                      ⚠ No AI key configured — mentions won't be scored.
                      Add one in Settings → AI Router.
                    </p>
                  )}
                </div>
              </div>
            )}
            {/* Agent active but all mentions are filtered out in this view */}
            {agentActive && totalMentions !== undefined && totalMentions > 0 && (
              <div className="flex items-center justify-center gap-2 py-2">
                <span className="relative flex h-2.5 w-2.5">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-muted-foreground opacity-40"></span>
                  <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-muted-foreground"></span>
                </span>
                <span className="text-xs text-muted-foreground">
                  Agent active · {totalMentions.toLocaleString()} mention{totalMentions === 1 ? "" : "s"} found
                </span>
              </div>
            )}
            {/* No keywords configured — agent not running */}
            {!agentActive && totalMentions === 0 && (
              <div className="space-y-2 py-4">
                <p className="text-muted-foreground">No keywords configured.</p>
                <p className="text-xs">Go to Settings → Keywords to start monitoring.</p>
              </div>
            )}
            {emptyMessage(queueFilter, actionKind)}
          </div>
        )}

        {!isProposalsView &&
          mentions.map((m) => {
            // A filter that already names the action would repeat it on every row.
            const badge = m.next_action && !(isActionView && actionKind) ? actionLabel[m.next_action] : null;
            return (
              <motion.button
                key={m.id}
                onClick={(e) =>
                  onSelect(m.id, (e.target as HTMLElement).closest("[data-open-person]") ? "person360" : undefined)
                }
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ duration: 0.15 }}
                className={`w-full text-left px-4 py-3.5 border-b border-border flex gap-3 transition-colors cursor-pointer ${
                  selectedId === m.id ? "bg-accent" : "hover:bg-accent/50"
                }`}
              >
                <div
                  className={`shrink-0 h-9 w-9 rounded-lg flex flex-col items-center justify-center text-[13px] font-[family-name:var(--font-head)] font-medium tabular-nums ${scoreClass(m.relevance_score)}`}
                >
                  {m.relevance_score != null ? m.relevance_score.toFixed(1) : "–"}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1 flex-wrap">
                    {badge && (
                      <span className="px-1.5 py-0.5 rounded-md border border-border bg-secondary text-foreground-soft text-[10px] font-medium">
                        {badge}
                      </span>
                    )}
                    <PlatformLogo platform={m.platform} />
                    {m.intent && (
                      <>
                        <span>·</span>
                        <span>{m.intent.replace("_", " ")}</span>
                      </>
                    )}
                    <span className="ml-auto shrink-0">
                      {timeAgo(m.platform_created_at ?? m.created_at)}
                    </span>
                  </div>
                  <p className="text-sm text-foreground line-clamp-2 leading-snug">
                    {m.platform_metadata?.signal === "hiring" ? hiringText(m.content, m, new Date()) : stripMarkdown(m.content)}
                  </p>
                  {m.person ? (
                    <div className="mt-1 text-xs text-foreground truncate" title={m.person.bio}>
                      <span data-open-person title="Open Person360" className="font-medium hover:underline underline-offset-2">
                        {m.person.name || m.author_username}
                      </span>
                      {[m.person.title || m.person.bio, [m.person.company, m.person.employees && `(${m.person.employees})`].filter(Boolean).join(" ")]
                        .filter(Boolean)
                        .map((part, i) => (
                          <span key={`${i}-${part}`} className="text-muted-foreground"> · {part}</span>
                        ))}
                      {m.person.has_email && <span className="text-muted-foreground"> · email found</span>}
                    </div>
                  ) : (
                    <div className="mt-1 text-xs text-muted-foreground truncate">
                      <span data-open-person title="Open Person360" className="hover:underline underline-offset-2">
                        @{m.author_username ?? "unknown"}
                      </span>
                    </div>
                  )}
                  <div className="mt-1.5">
                    <TagChips tags={m.intent_tags} sentiment={m.sentiment} />
                  </div>
                </div>
              </motion.button>
            );
          })}

        {isBrowseView && hasMore && (
          <div ref={loadMoreRef} className="py-4 text-center text-xs text-muted-foreground">
            {isLoadingMore ? "Loading more…" : "Scroll for more"}
          </div>
        )}

        {isBrowseView && !hasMore && mentions.length > 0 && (
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
                selectedId === p.id ? "bg-accent" : ""
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
                    className="px-2 py-1 text-xs rounded-md border border-border bg-card text-foreground hover:bg-accent cursor-pointer disabled:opacity-50"
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
