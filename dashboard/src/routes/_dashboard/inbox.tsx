import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useInfiniteQuery, useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { LeadList } from "@/components/inbox/lead-list";
import { LeadDetail } from "@/components/inbox/lead-detail";
import {
  listMentions,
  listProposals,
  mentionPlatformCounts,
  mentionQueueCounts,
  proposalCounts,
  updateMentionStatus,
  updateProposalStatus,
  listKeywords,
  getLLMConfig,
} from "@/lib/api";
import { INBOX_QUEUES, QUERY_KEYS } from "@/lib/constants";
import type { InboxQueueCountsResponse } from "@/lib/types";

const PAGE_SIZE = 30;

interface InboxSearch {
  q?: string;
  queue?: string;
  action_kind?: string;
  platform?: string;
  status?: string;
  id?: string;
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
    id: typeof search.id === "string" ? search.id : undefined,
  }),
  component: InboxPage,
});

function InboxPage() {
  const search = Route.useSearch();
  const { q = "", platform = "", status = "", id } = search;
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
    ],
    queryFn: ({ pageParam }) =>
      listMentions({
        queue: queue || undefined,
        action_kind: isActionView && actionKind ? actionKind : undefined,
        platform: isBrowseView && platform ? platform : undefined,
        status: isBrowseView && status ? status : undefined,
        search: isBrowseView && q ? q : undefined,
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
    queryKey: [QUERY_KEYS.mentionPlatformCounts, queue, status, q],
    queryFn: () =>
      mentionPlatformCounts({
        queue,
        status: status || undefined,
        search: q || undefined,
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

  const archiveMutation = useMutation({
    mutationFn: (mentionId: string) => updateMentionStatus(mentionId, "archived"),
    onSuccess: invalidateInbox,
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
  const selected = isProposalsView
    ? null
    : (mentions.find((m) => m.id === id) ?? null);
  const selectedIndex = selected ? mentions.findIndex((m) => m.id === selected.id) : -1;

  const selectMention = (mentionId: string) => setSearch({ id: mentionId });
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
      q: browse ? q : undefined,
    });
  };

  return (
    <div className="flex -m-6 min-h-0 overflow-hidden h-[calc(100vh-var(--header-height))]">
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
      />
      {isProposalsView ? (
        <section className="flex-1 flex items-center justify-center text-sm text-muted-foreground p-8">
          Select a proposal to review community opportunities from the Discovery agent.
        </section>
      ) : (
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
        />
      )}
    </div>
  );
}
