import { CreditsBanner } from "@/components/billing/credits-banner";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useInfiniteQuery, useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { ChevronLeft, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { LeadList } from "@/components/inbox/lead-list";
import { LeadDetail, type DetailTab } from "@/components/inbox/lead-detail";
import { leadScope } from "@/components/inbox/intent-tags";
import { voteKey, type Vote } from "@/lib/lead-vote";
import { LoosenedBanner } from "@/components/onboarding/loosened-banner"; // onboarding
import {
  getMention,
  listMentions,
  listProposals,
  sendMentionFeedback,
  mentionPlatformCounts,
  mentionQueueCounts,
  proposalCounts,
  updateMentionStatus,
  updateProposalStatus,
  listKeywords,
  getLLMConfig,
  getSetupStatus,
} from "@/lib/api";
import { INBOX_QUEUES, QUERY_KEYS } from "@/lib/constants";
import { setupPollMs } from "@/lib/setup-status";
import { isPostedRange, postedSince } from "@/lib/posted-range";
import type { InboxQueueCountsResponse } from "@/lib/types";

const PAGE_SIZE = 30;

interface InboxSearch {
  q?: string;
  queue?: string;
  action_kind?: string;
  platform?: string;
  status?: string;
  tag?: string; // scoring
  sentiment?: string; // scoring
  intent?: string; // set by Analytics' "Mentions by intent" links
  posted?: string; // "24h" | "7d" | "30d" | "90d"
  id?: string;
  tab?: string; // person360: the lead's open tab
  loosened?: string; // onboarding: set when first value fell back below high fit
}

function pickDefaultQueue(counts: InboxQueueCountsResponse | undefined): string {
  const actionCount =
    counts?.queues.find((q) => q.queue === INBOX_QUEUES.ACTION_REQUIRED)?.count ?? 0;
  if (actionCount > 0) return INBOX_QUEUES.ACTION_REQUIRED;
  return INBOX_QUEUES.ALL;
}

export const Route = createFileRoute("/_dashboard/inbox")({
  validateSearch: (search: Record<string, unknown>): InboxSearch => ({
    q: typeof search.q === "string" ? search.q : undefined,
    queue: typeof search.queue === "string" ? search.queue : undefined,
    action_kind: typeof search.action_kind === "string" ? search.action_kind : undefined,
    platform: typeof search.platform === "string" ? search.platform : undefined,
    status: typeof search.status === "string" ? search.status : undefined,
    tag: typeof search.tag === "string" ? search.tag : undefined,
    sentiment: typeof search.sentiment === "string" ? search.sentiment : undefined,
    intent: typeof search.intent === "string" ? search.intent : undefined,
    posted: isPostedRange(search.posted) ? search.posted : undefined,
    id: typeof search.id === "string" ? search.id : undefined,
    tab: search.tab === "person360" || search.tab === "thread" ? search.tab : undefined,
    loosened: typeof search.loosened === "string" ? search.loosened : undefined, // onboarding
  }),
  component: InboxPage,
});

function InboxPage() {
  const search = Route.useSearch();
  const { q = "", platform = "", status = "", tag = "", sentiment = "", intent = "", posted = "", id } = search;
  const navigate = useNavigate({ from: Route.fullPath });
  const queryClient = useQueryClient();

  const { data: queueCountsData } = useQuery({
    queryKey: [QUERY_KEYS.mentionQueueCounts],
    queryFn: mentionQueueCounts,
    refetchInterval: 30000,
  });

  // Fetch keywords to determine if the agent is active and how many monitors are running
  const { data: keywords } = useQuery({
    queryKey: ["keywords"],
    queryFn: listKeywords,
    refetchInterval: 30000,
  });

  // Fetch LLM config to check if AI scoring is configured
  const { data: llmConfig } = useQuery({
    queryKey: ["llm-config"],
    queryFn: getLLMConfig,
    retry: false,
  });

  // A workspace with nothing to act on may not have a lead yet: ask what it
  // is waiting for. Workspaces with leads in the queue never make this call,
  // and it stops repeating once a lead exists.
  const nothingToActOn =
    queueCountsData !== undefined &&
    (queueCountsData.queues.find((c) => c.queue === INBOX_QUEUES.ACTION_REQUIRED)?.count ?? 0) === 0;
  const { data: setup } = useQuery({
    queryKey: ["setup-status"],
    queryFn: getSetupStatus,
    enabled: nothingToActOn,
    staleTime: 5 * 60_000,
    refetchInterval: (query) => setupPollMs(query.state.data, 5000),
  });

  const activeKeywords = (keywords ?? []).filter((k) => k.is_active);
  const aiConfigured = llmConfig?.health?.chat_ok ?? false;
  const totalMentions =
    queueCountsData?.queues.find((q) => q.queue === INBOX_QUEUES.ALL)?.count ?? 0;
  const agentActive = activeKeywords.length > 0;
  const agentNeedsAI = agentActive && !aiConfigured;

  const queue = search.queue ?? pickDefaultQueue(queueCountsData);
  const isProposalsView = queue === INBOX_QUEUES.PROPOSALS;
  const isActionView = queue === INBOX_QUEUES.ACTION_REQUIRED;
  const isBrowseView = queue === INBOX_QUEUES.ALL;
  const actionKind = search.action_kind;

  useEffect(() => {
    if (!queueCountsData || search.queue) return;
    navigate({
      search: (prev) => ({ ...prev, queue: pickDefaultQueue(queueCountsData) }),
      replace: true,
    });
  }, [search.queue, queueCountsData, navigate]);

  const setSearch = (patch: Partial<InboxSearch>) =>
    navigate({ search: (prev) => ({ ...prev, ...patch }) });

  const {
    data,
    isLoading,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: [
      QUERY_KEYS.mentions,
      queue,
      actionKind,
      isBrowseView ? platform : "",
      isBrowseView ? status : "",
      isBrowseView ? q : "",
      isBrowseView ? tag : "",
      isBrowseView ? sentiment : "",
      isBrowseView ? intent : "",
      isBrowseView ? posted : "",
    ],
    queryFn: ({ pageParam }) =>
      listMentions({
        queue: queue || undefined,
        action_kind: isActionView && actionKind ? actionKind : undefined,
        platform: isBrowseView && platform ? platform : undefined,
        status: isBrowseView && status ? status : undefined,
        search: isBrowseView && q ? q : undefined,
        tag: isBrowseView && tag ? tag : undefined,
        sentiment: isBrowseView && sentiment ? sentiment : undefined,
        intent: isBrowseView && intent ? intent : undefined,
        since: isBrowseView ? postedSince(posted) : undefined,
        limit: PAGE_SIZE,
        offset: pageParam,
      }),
    initialPageParam: 0,
    getNextPageParam: (lastPage) => {
      const next = lastPage.offset + lastPage.limit;
      return next < lastPage.total ? next : undefined;
    },
    enabled: !isProposalsView,
    refetchInterval: 30000,
  });

  const { data: proposals, isLoading: proposalsLoading, refetch: refetchProposals } = useQuery({
    queryKey: [QUERY_KEYS.proposals],
    queryFn: () => listProposals({ status: "pending", limit: 30 }),
    enabled: isProposalsView,
  });

  const { data: proposalCountRows } = useQuery({
    queryKey: [QUERY_KEYS.proposalCounts],
    queryFn: proposalCounts,
  });

  const { data: platformCounts } = useQuery({
    queryKey: [QUERY_KEYS.mentionPlatformCounts, queue, status, q, tag, sentiment, intent, posted],
    queryFn: () =>
      mentionPlatformCounts({
        queue,
        status: status || undefined,
        search: q || undefined,
        tag: tag || undefined,
        sentiment: sentiment || undefined,
        intent: intent || undefined,
        since: postedSince(posted),
      }),
    enabled: isBrowseView,
  });

  const proposalPendingCount =
    proposalCountRows?.find((c) => c.status === "pending")?.count ?? 0;

  const invalidateInbox = () => {
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentions] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionCounts] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionQueueCounts] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionPlatformCounts] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.scoringPrecision] });
  };

  // Errors from actions that already moved on to the next lead (archive, spam, y / n).
  const [inboxError, setInboxError] = useState<string | null>(null);
  const failed = (what: string) => (err: Error) =>
    setInboxError(`Couldn't ${what} (${err.message || "network error"}).`);

  const archiveMutation = useMutation({
    mutationFn: (mentionId: string) => updateMentionStatus(mentionId, "archived"),
    onSuccess: invalidateInbox,
    onError: failed("archive that lead"),
  });

  const feedbackMutation = useMutation({
    mutationFn: ({
      mentionId,
      status: nextStatus,
      reason,
    }: {
      mentionId: string;
      status: "spam" | "archived";
      reason?: string;
    }) => updateMentionStatus(mentionId, nextStatus, reason),
    onSuccess: invalidateInbox,
    onError: (err, { status: nextStatus }) => failed(nextStatus === "spam" ? "mark that lead as spam" : "archive that lead")(err),
  });

  const proposalMutation = useMutation({
    mutationFn: ({ id: proposalId, status: nextStatus }: { id: string; status: "accepted" | "dismissed" }) =>
      updateProposalStatus(proposalId, nextStatus),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.proposals] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.proposalCounts] });
    },
  });

  const mentions = data?.pages.flatMap((p) => p.data) ?? [];
  const listTotal = data?.pages[0]?.total ?? 0;
  const inList = mentions.find((m) => m.id === id);
  // Deep links (e.g. "Draft reply" from Leads) may target a mention outside the loaded page.
  const { data: linked } = useQuery({
    queryKey: [QUERY_KEYS.mentions, "one", id],
    queryFn: () => getMention(id!),
    enabled: !!id && !isProposalsView && !isLoading && !inList,
  });
  const selected = isProposalsView ? null : (inList ?? (linked && linked.id === id ? linked : null));
  const selectedIndex = selected ? mentions.findIndex((m) => m.id === selected.id) : -1;

  const selectMention = (mentionId: string, tab?: DetailTab) =>
    setSearch({ id: mentionId, tab: tab === "overview" ? undefined : tab });
  const goPrev = () => selectedIndex > 0 && selectMention(mentions[selectedIndex - 1].id);
  const goNext = () => {
    if (selectedIndex < 0) return;
    if (selectedIndex < mentions.length - 1) {
      selectMention(mentions[selectedIndex + 1].id);
      return;
    }
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
  };

  const advanceSelection = () => {
    if (!selected) return;
    const nextId = mentions[selectedIndex + 1]?.id ?? mentions[selectedIndex - 1]?.id;
    setSearch({ id: nextId });
  };

  const handleArchive = () => {
    if (!selected) return;
    advanceSelection();
    archiveMutation.mutate(selected.id);
  };

  const handleFeedback = (nextStatus: "spam" | "archived", reason?: string) => {
    if (!selected) return;
    advanceSelection();
    feedbackMutation.mutate({ mentionId: selected.id, status: nextStatus, reason });
  };

  // Vote "Not a lead" with a reason, then archive with it: one sequence, so
  // leaving the lead mid-flight neither skips the archive nor moves you on.
  const notAFitMutation = useMutation({
    scope: leadScope(selected?.id),
    mutationFn: async ({ mentionId, note }: { mentionId: string; note: string }) => {
      await sendMentionFeedback(mentionId, "down", note);
      await updateMentionStatus(mentionId, "archived", note);
    },
    onSuccess: (_, { mentionId }) => {
      if (selected?.id === mentionId) advanceSelection();
      invalidateInbox();
    },
    onError: invalidateInbox, // the vote may have landed without the archive
  });

  // y = Lead, n = Not a lead on the open lead, then on to the next one. A
  // failed vote reopens its lead and says so.
  const keyVote = useMutation({
    scope: leadScope(selected?.id),
    mutationFn: ({ id: mentionId, label }: { id: string; label: Vote }) => sendMentionFeedback(mentionId, label),
    onSuccess: invalidateInbox,
    onError: (err, { id: mentionId }) => {
      setInboxError(`Couldn't save your vote (${err.message || "network error"}). That lead is open again.`);
      setSearch({ id: mentionId });
    },
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const label = voteKey(e, document.querySelector('[aria-modal="true"]') != null);
      if (!label || !selected) return;
      e.preventDefault();
      setInboxError(null);
      keyVote.mutate({ id: selected.id, label });
      advanceSelection();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }); // ponytail: re-subscribes each render so the handler never sees a stale selection

  const handleRefresh = () => {
    if (isProposalsView) {
      void refetchProposals();
    } else {
      void refetch();
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionQueueCounts] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionPlatformCounts] });
    }
  };

  const handleQueueChange = (nextQueue: string) => {
    const browse = nextQueue === INBOX_QUEUES.ALL;
    setSearch({
      queue: nextQueue,
      id: undefined,
      action_kind: undefined,
      platform: undefined,
      status: undefined,
      tag: undefined,
      sentiment: undefined,
      intent: undefined,
      posted: browse ? posted || undefined : undefined,
      q: browse ? q : undefined,
    });
  };

  return (
    <div className="relative flex h-[calc(100dvh-var(--header-height))] min-h-0 overflow-hidden">
      {inboxError && (
        <div
          role="alert"
          className="absolute bottom-3 left-3 z-30 flex max-w-[calc(100%-1.5rem)] items-start gap-2 rounded-lg border border-destructive/30 bg-card px-3 py-2 text-xs text-destructive shadow-lg md:max-w-sm"
        >
          <span className="min-w-0 flex-1">{inboxError}</span>
          <button
            type="button"
            onClick={() => setInboxError(null)}
            aria-label="Dismiss"
            className="shrink-0 rounded text-muted-foreground hover:text-foreground focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring/40 cursor-pointer"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
      {search.loosened && <LoosenedBanner /> /* onboarding */}
      <CreditsBanner /* billing */ />
      {/* Phones: one pane at a time, the list, or the open post with a way back. */}
      <div className={cn("h-full min-h-0 w-full md:flex md:w-auto", id ? "hidden" : "flex")}>
      <LeadList
        mentions={mentions}
        listTotal={listTotal}
        proposals={proposals}
        isLoading={isProposalsView ? proposalsLoading : isLoading}
        isLoadingMore={isFetchingNextPage}
        hasMore={Boolean(hasNextPage) && isBrowseView}
        onLoadMore={() => {
          if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
        }}
        selectedId={id ?? null}
        onSelect={selectMention}
        queueFilter={queue}
        onQueueChange={handleQueueChange}
        queueCounts={queueCountsData}
        actionKind={actionKind}
        onActionKindChange={(kind) =>
          setSearch({ action_kind: kind || undefined, id: undefined })
        }
        proposalPendingCount={proposalPendingCount}
        platformFilter={platform}
        onPlatformChange={(p) => setSearch({ platform: p || undefined })}
        platformOptions={platformCounts ?? []}
        statusFilter={status}
        onStatusChange={(s) => setSearch({ status: s || undefined })}
        tagFilter={tag}
        onTagChange={(t) => setSearch({ tag: t || undefined })}
        sentimentFilter={sentiment}
        onSentimentChange={(s) => setSearch({ sentiment: s || undefined })}
        intentFilter={intent}
        onClearIntent={() => setSearch({ intent: undefined })}
        postedFilter={posted}
        onPostedChange={(r) => setSearch({ posted: r || undefined })}
        search={q}
        onSearchChange={(v) => setSearch({ q: v || undefined })}
        onRefresh={handleRefresh}
        onProposalAction={(proposalId, nextStatus) =>
          proposalMutation.mutate({ id: proposalId, status: nextStatus })
        }
        proposalActionPending={proposalMutation.isPending}
        agentActive={agentActive}
        agentNeedsAI={agentNeedsAI}
        activeKeywordCount={activeKeywords.length}
        totalMentions={totalMentions}
        setup={nothingToActOn ? setup : undefined}
      />
      </div>
      {isProposalsView ? (
        <section className="hidden flex-1 items-center justify-center p-8 text-sm text-muted-foreground md:flex">
          Select a proposal to review community opportunities from the Discovery agent.
        </section>
      ) : (
        <div className={cn("h-full min-w-0 flex-1 flex-col md:flex", id ? "flex" : "hidden")}>
        <button
          type="button"
          onClick={() => setSearch({ id: undefined })}
          className="flex h-10 shrink-0 items-center gap-1.5 border-b border-border px-4 text-sm text-muted-foreground md:hidden"
        >
          <ChevronLeft className="h-4 w-4" /> Back to inbox
        </button>
        <LeadDetail
          mention={selected}
          onPrev={goPrev}
          onNext={goNext}
          hasPrev={selectedIndex > 0}
          hasNext={
            selectedIndex >= 0 &&
            (selectedIndex < mentions.length - 1 || Boolean(hasNextPage))
          }
          onArchive={handleArchive}
          archiving={archiveMutation.isPending}
          onFeedback={handleFeedback}
          feedbackPending={feedbackMutation.isPending}
          notAFit={notAFitMutation}
          tab={(search.tab as DetailTab | undefined) ?? "overview"}
          onTabChange={(t) => setSearch({ tab: t === "overview" ? undefined : t })}
        />
        </div>
      )}
    </div>
  );
}
