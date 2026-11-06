import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { listLeads, updateLeadStage, leadCounts } from "@/lib/api";
import type { LeadStage } from "@/lib/types";
import { sortLeads } from "@/lib/pipeline";
import { ExportCsvButton } from "@/components/sources/export-csv-button"; // sources
import { LeadRow } from "@/components/pipeline/lead-row";
import { StageTabs } from "@/components/pipeline/stage-tabs";

export const Route = createFileRoute("/_dashboard/pipeline")({
  component: PipelinePage,
});

const EMPTY: Record<LeadStage, string> = {
  prospect: "Leads appear here when someone asks for a tool like yours. Reply, then move them along.",
  qualified: "People you've replied to land here. Move them on when they answer.",
  engaged: "Leads you're in a back-and-forth with. Mark them Won when they sign up.",
  converted: "Leads that became customers. Nothing here yet.",
  lost: "Leads that weren't a fit. Nothing here yet.",
};

function PipelinePage() {
  const queryClient = useQueryClient();
  const [stage, setStage] = useState<LeadStage>("prospect");
  const [platform, setPlatform] = useState("");

  const { data, isLoading, isError } = useQuery({
    queryKey: ["leads", stage],
    // ponytail: first 100 per stage (API max); add paging when a stage outgrows it.
    queryFn: () => listLeads({ stage, limit: 100 }),
  });

  const { data: counts } = useQuery({
    queryKey: ["leadCounts"],
    queryFn: leadCounts,
  });

  const moveMutation = useMutation({
    mutationFn: ({ id, stage: next }: { id: string; stage: LeadStage }) => updateLeadStage(id, next),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["leads"] });
      queryClient.invalidateQueries({ queryKey: ["leadCounts"] });
    },
  });

  const all = data?.data ?? [];
  const platforms = [...new Set(all.map((l) => l.platform).filter((p): p is NonNullable<typeof p> => !!p))];
  const leads = sortLeads(platform ? all.filter((l) => l.platform === platform) : all);
  const countMap = new Map(counts?.map((c) => [c.status, c.count]) ?? []);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <Text as="h2">Pipeline</Text>
          <p className="text-sm text-muted-foreground">Who to reply to, and what's next.</p>
        </div>
        <ExportCsvButton kind="leads" label="Export CSV" /> {/* sources */}
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <StageTabs
          value={stage}
          counts={countMap}
          onChange={(s) => {
            setStage(s);
            setPlatform("");
          }}
        />
        {platforms.length > 1 && (
          <select
            aria-label="Filter by platform"
            className="h-8 rounded-lg border border-border bg-card px-2 text-[13px] text-foreground"
            value={platform}
            onChange={(e) => setPlatform(e.target.value)}
          >
            <option value="">All platforms</option>
            {platforms.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        )}
      </div>

      <div className="rounded-xl border border-border bg-card">
        {isLoading ? (
          <p className="p-8 text-center text-sm text-muted-foreground">Loading leads…</p>
        ) : isError ? (
          <p className="p-8 text-center text-sm text-destructive">Couldn't load leads. Try refreshing.</p>
        ) : leads.length === 0 ? (
          <p className="p-8 text-center text-sm text-muted-foreground">{EMPTY[stage]}</p>
        ) : (
          <ul className="divide-y divide-border">
            {leads.map((lead) => (
              <LeadRow
                key={lead.id}
                lead={lead}
                pending={moveMutation.isPending && moveMutation.variables?.id === lead.id}
                onStage={(id, next) => moveMutation.mutate({ id, stage: next })}
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
