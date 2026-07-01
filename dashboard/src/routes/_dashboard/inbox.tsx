import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { LeadList } from "@/components/inbox/lead-list";
import { LeadDetail } from "@/components/inbox/lead-detail";
import {
  listMentions,
  mentionCounts,
  mentionTierCounts,
  mentionsPerPlatform,
  updateMentionStatus,
} from "@/lib/api";

interface InboxSearch {
  q?: string;
  tier?: string;
  platform?: string;
  id?: string;
}

export const Route = createFileRoute("/_dashboard/inbox")({
  validateSearch: (search: Record<string, unknown>): InboxSearch => ({
    q: typeof search.q === "string" ? search.q : undefined,
    tier: typeof search.tier === "string" ? search.tier : undefined,
    platform: typeof search.platform === "string" ? search.platform : undefined,
    id: typeof search.id === "string" ? search.id : undefined,
  }),
  component: InboxPage,
});

function InboxPage() {
  const { q = "", tier = "", platform = "", id } = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const queryClient = useQueryClient();

  const setSearch = (patch: Partial<InboxSearch>) =>
    navigate({ search: (prev) => ({ ...prev, ...patch }) });

  const { data, isLoading, refetch } = useQuery({
    queryKey: ["mentions", tier, platform, q],
    queryFn: () =>
      listMentions({
        tier: tier || undefined,
        platform: platform || undefined,
        search: q || undefined,
        limit: 30,
      }),
  });

  const { data: tierCounts } = useQuery({
    queryKey: ["mentionTierCounts"],
    queryFn: mentionTierCounts,
  });

  useQuery({
    queryKey: ["mentionCounts"],
    queryFn: mentionCounts,
  });

  const { data: platformCounts } = useQuery({
    queryKey: ["mentionsPerPlatform"],
    queryFn: mentionsPerPlatform,
  });

  const archiveMutation = useMutation({
    mutationFn: (mentionId: string) => updateMentionStatus(mentionId, "archived"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mentions"] });
      queryClient.invalidateQueries({ queryKey: ["mentionCounts"] });
      queryClient.invalidateQueries({ queryKey: ["mentionTierCounts"] });
    },
  });

  const mentions = data?.data ?? [];
  const selected = mentions.find((m) => m.id === id) ?? null;
  const selectedIndex = selected ? mentions.findIndex((m) => m.id === selected.id) : -1;

  const selectMention = (mentionId: string) => setSearch({ id: mentionId });
  const goPrev = () => selectedIndex > 0 && selectMention(mentions[selectedIndex - 1].id);
  const goNext = () =>
    selectedIndex >= 0 && selectedIndex < mentions.length - 1 && selectMention(mentions[selectedIndex + 1].id);

  const handleArchive = () => {
    if (!selected) return;
    const nextId = mentions[selectedIndex + 1]?.id ?? mentions[selectedIndex - 1]?.id;
    archiveMutation.mutate(selected.id);
    setSearch({ id: nextId });
  };

  return (
    <div className="flex h-[calc(100vh-var(--header-height))] -m-6">
      <LeadList
        mentions={mentions}
        isLoading={isLoading}
        selectedId={id ?? null}
        onSelect={selectMention}
        tierFilter={tier}
        onTierChange={(t) => setSearch({ tier: t || undefined })}
        tierCounts={tierCounts}
        platformFilter={platform}
        onPlatformChange={(p) => setSearch({ platform: p || undefined })}
        platformOptions={platformCounts ?? []}
        search={q}
        onSearchChange={(v) => setSearch({ q: v || undefined })}
        onRefresh={() => refetch()}
      />
      <LeadDetail
        mention={selected}
        onPrev={goPrev}
        onNext={goNext}
        hasPrev={selectedIndex > 0}
        hasNext={selectedIndex >= 0 && selectedIndex < mentions.length - 1}
        onArchive={handleArchive}
        archiving={archiveMutation.isPending}
      />
    </div>
  );
}
