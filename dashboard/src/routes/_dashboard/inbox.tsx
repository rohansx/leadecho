import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { LeadList } from "@/components/inbox/lead-list";
import { LeadDetail } from "@/components/inbox/lead-detail";
import {
  listMentions,
  listProposals,
  mentionQueueCounts,
  mentionsPerPlatform,
  proposalCounts,
  updateMentionStatus,
  updateProposalStatus,
} from "@/lib/api";
import { INBOX_QUEUES, QUERY_KEYS } from "@/lib/constants";

interface InboxSearch {
  q?: string;
  queue?: string;
  platform?: string;
  status?: string;
  id?: string;
}

export const Route = createFileRoute("/_dashboard/inbox")({
  validateSearch: (search: Record<string, unknown>): InboxSearch => ({
    q: typeof search.q === "string" ? search.q : undefined,
    queue:
      typeof search.queue === "string"
        ? search.queue
        : INBOX_QUEUES.AUTO_FLOWING,
    platform: typeof search.platform === "string" ? search.platform : undefined,
    status: typeof search.status === "string" ? search.status : undefined,
    id: typeof search.id === "string" ? search.id : undefined,
  }),
  component: InboxPage,
});

function InboxPage() {
  const {
    q = "",
    queue = INBOX_QUEUES.AUTO_FLOWING,
    platform = "",
    status = "",
    id,
  } = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const queryClient = useQueryClient();

  const isProposalsView = queue === INBOX_QUEUES.PROPOSALS;

  const setSearch = (patch: Partial<InboxSearch>) =>
    navigate({ search: (prev) => ({ ...prev, ...patch }) });

  const { data, isLoading, refetch } = useQuery({
    queryKey: [QUERY_KEYS.mentions, queue, platform, status, q],
    queryFn: () =>
      listMentions({
        queue: queue || undefined,
        platform: platform || undefined,
        status: status || undefined,
        search: q || undefined,
        limit: 30,
      }),
    enabled: !isProposalsView,
  });

  const { data: proposals, isLoading: proposalsLoading, refetch: refetchProposals } = useQuery({
    queryKey: [QUERY_KEYS.proposals],
    queryFn: () => listProposals({ status: "pending", limit: 30 }),
    enabled: isProposalsView,
  });

  const { data: queueCounts } = useQuery({
    queryKey: [QUERY_KEYS.mentionQueueCounts],
    queryFn: mentionQueueCounts,
  });

  const { data: proposalCountRows } = useQuery({
    queryKey: [QUERY_KEYS.proposalCounts],
    queryFn: proposalCounts,
  });

  const { data: platformCounts } = useQuery({
    queryKey: [QUERY_KEYS.mentionsPerPlatform],
    queryFn: mentionsPerPlatform,
  });

  const proposalPendingCount =
    proposalCountRows?.find((c) => c.status === "pending")?.count ?? 0;

  const archiveMutation = useMutation({
    mutationFn: (mentionId: string) => updateMentionStatus(mentionId, "archived"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentions] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionCounts] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionQueueCounts] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.scoringPrecision] });
    },
  });

  const feedbackMutation = useMutation({
    mutationFn: ({
      mentionId,
      status,
      reason,
    }: {
      mentionId: string;
      status: "spam" | "archived";
      reason?: string;
    }) => updateMentionStatus(mentionId, status, reason),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentions] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionCounts] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionQueueCounts] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.scoringPrecision] });
    },
  });

  const proposalMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: "accepted" | "dismissed" }) =>
      updateProposalStatus(id, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.proposals] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.proposalCounts] });
    },
  });

  const mentions = data?.data ?? [];
  const selected = isProposalsView
    ? null
    : (mentions.find((m) => m.id === id) ?? null);
  const selectedIndex = selected ? mentions.findIndex((m) => m.id === selected.id) : -1;

  const selectMention = (mentionId: string) => setSearch({ id: mentionId });
  const goPrev = () => selectedIndex > 0 && selectMention(mentions[selectedIndex - 1].id);
  const goNext = () =>
    selectedIndex >= 0 &&
    selectedIndex < mentions.length - 1 &&
    selectMention(mentions[selectedIndex + 1].id);

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

  const handleFeedback = (status: "spam" | "archived", reason?: string) => {
    if (!selected) return;
    advanceSelection();
    feedbackMutation.mutate({ mentionId: selected.id, status, reason });
  };

  const handleRefresh = () => {
    if (isProposalsView) {
      void refetchProposals();
    } else {
      void refetch();
    }
  };

  return (
    <div className="flex h-[calc(100vh-var(--header-height))] -m-6">
      <LeadList
        mentions={mentions}
        proposals={proposals}
        isLoading={isProposalsView ? proposalsLoading : isLoading}
        selectedId={id ?? null}
        onSelect={selectMention}
        queueFilter={queue}
        onQueueChange={(q) => setSearch({ queue: q, id: undefined })}
        queueCounts={queueCounts}
        proposalPendingCount={proposalPendingCount}
        platformFilter={platform}
        onPlatformChange={(p) => setSearch({ platform: p || undefined })}
        platformOptions={platformCounts ?? []}
        statusFilter={status}
        onStatusChange={(s) => setSearch({ status: s || undefined })}
        search={q}
        onSearchChange={(v) => setSearch({ q: v || undefined })}
        onRefresh={handleRefresh}
        onProposalAction={(proposalId, status) =>
          proposalMutation.mutate({ id: proposalId, status })
        }
        proposalActionPending={proposalMutation.isPending}
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
          hasNext={selectedIndex >= 0 && selectedIndex < mentions.length - 1}
          onArchive={handleArchive}
          archiving={archiveMutation.isPending}
          onFeedback={handleFeedback}
          feedbackPending={feedbackMutation.isPending}
        />
      )}
    </div>
  );
}
