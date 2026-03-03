import { createFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import {
  DollarSign,
  User,
  ExternalLink,
  ChevronRight,
  RefreshCw,
} from "lucide-react";
import { listLeads, updateLeadStage, leadCounts } from "@/lib/api";
import type { Lead, LeadStage } from "@/lib/types";

export const Route = createFileRoute("/_dashboard/pipeline")({
  component: PipelinePage,
});

const stages: { key: LeadStage; label: string; color: string }[] = [
  { key: "prospect", label: "Prospect", color: "bg-slate-100 text-slate-700" },
  {
    key: "qualified",
    label: "Qualified",
    color: "bg-blue-100 text-blue-700",
  },
  { key: "engaged", label: "Engaged", color: "bg-purple-100 text-purple-700" },
  {
    key: "converted",
    label: "Converted",
    color: "bg-green-100 text-green-700",
  },
  { key: "lost", label: "Lost", color: "bg-red-100 text-red-700" },
];

const nextStage: Record<string, LeadStage | null> = {
  prospect: "qualified",
  qualified: "engaged",
  engaged: "converted",
  converted: null,
  lost: null,
};

const platformColors: Record<string, string> = {
  reddit: "bg-orange-100 text-orange-800",
  hackernews: "bg-amber-100 text-amber-800",
  twitter: "bg-sky-100 text-sky-800",
  linkedin: "bg-blue-100 text-blue-800",
};

function PipelinePage() {
  const queryClient = useQueryClient();

  const { data, isLoading, refetch } = useQuery({
    queryKey: ["leads"],
    queryFn: () => listLeads({ limit: 100 }),
  });

  const { data: counts } = useQuery({
    queryKey: ["leadCounts"],
    queryFn: leadCounts,
  });

  const moveMutation = useMutation({
    mutationFn: ({ id, stage }: { id: string; stage: string }) =>
      updateLeadStage(id, stage),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["leads"] });
      queryClient.invalidateQueries({ queryKey: ["leadCounts"] });
    },
  });

  const leads = data?.data ?? [];
  const countMap = new Map(counts?.map((c) => [c.status, c.count]) ?? []);

  const leadsByStage = (stage: string) =>
    leads.filter((l: Lead) => l.stage === stage);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <Text as="h2">Lead Pipeline</Text>
        <div className="flex gap-2 items-center">
          <Badge variant="surface">{leads.length} leads</Badge>
          <Button size="sm" variant="outline" onClick={() => refetch()}>
            <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
            Refresh
          </Button>
        </div>
      </div>

      {/* Loading state */}
      {isLoading && (
        <Card className="w-full">
          <CardContent className="p-8 text-center">
            <Text as="p" className="text-muted-foreground">
              Loading pipeline...
            </Text>
          </CardContent>
        </Card>
      )}

      {/* Kanban Board */}
      {!isLoading && (
        <div className="flex gap-4 overflow-x-auto pb-4">
          {stages.map((stage) => {
            const stageLeads = leadsByStage(stage.key);
            const count = countMap.get(stage.key) ?? stageLeads.length;
            return (
              <div
                key={stage.key}
                className="min-w-[280px] w-[280px] flex-shrink-0"
              >
                {/* Column Header */}
                <div className="flex items-center justify-between mb-3 px-1">
                  <div className="flex items-center gap-2">
                    <Badge
                      className={`${stage.color} border border-border`}
                      size="sm"
                    >
                      {stage.label}
                    </Badge>
                    <span className="text-sm text-muted-foreground font-[family-name:var(--font-sans)]">
                      {count}
                    </span>
                  </div>
                </div>

                {/* Lead Cards */}
                <div className="space-y-3">
                  {stageLeads.length === 0 && (
                    <div className="border-2 border-dashed border-border rounded p-6 text-center">
                      <Text
                        as="p"
                        className="text-xs text-muted-foreground"
                      >
                        No leads
                      </Text>
                    </div>
                  )}
                  {stageLeads.map((lead: Lead) => (
                    <LeadCard
                      key={lead.id}
                      lead={lead}
                      onMove={(id, newStage) =>
                        moveMutation.mutate({ id, stage: newStage })
                      }
                      isPending={moveMutation.isPending}
                    />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function LeadCard({
  lead,
  onMove,
  isPending,
}: {
  lead: Lead;
  onMove: (id: string, stage: string) => void;
  isPending: boolean;
}) {
  const next = nextStage[lead.stage];

  return (
    <Card className="w-full hover:shadow-sm">
      <CardContent className="p-3 space-y-2">
        {/* Name + Platform */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1.5">
            <User className="h-3.5 w-3.5 text-muted-foreground" />
            <Text as="span" className="text-sm font-medium truncate max-w-[140px]">
              {lead.contact_name ?? lead.username ?? "Unknown"}
            </Text>
          </div>
          {lead.platform && (
            <Badge
              className={`${platformColors[lead.platform] ?? ""} border border-border`}
              size="sm"
            >
              {lead.platform}
            </Badge>
          )}
        </div>

        {/* Company */}
        {lead.company && (
          <Text as="p" className="text-xs text-muted-foreground truncate">
            {lead.company}
          </Text>
        )}

        {/* Value */}
        {lead.estimated_value != null && lead.estimated_value > 0 && (
          <div className="flex items-center gap-1">
            <DollarSign className="h-3 w-3 text-green-600" />
            <Text as="span" className="text-xs font-medium text-green-700">
              ${lead.estimated_value.toLocaleString()}/yr
            </Text>
          </div>
        )}

        {/* Tags */}
        {lead.tags.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {lead.tags.slice(0, 3).map((tag) => (
              <Badge key={tag} variant="outline" size="sm" className="text-xs">
                {tag}
              </Badge>
            ))}
          </div>
        )}

        {/* Notes preview */}
        {lead.notes && (
          <Text as="p" className="text-xs text-muted-foreground line-clamp-2">
            {lead.notes}
          </Text>
        )}

        {/* Actions */}
        <div className="flex items-center gap-2 pt-1 border-t border-border/50">
          {lead.profile_url && (
            <a
              href={lead.profile_url}
              target="_blank"
              rel="noopener noreferrer"
            >
              <Button size="sm" variant="ghost" className="h-7 px-2">
                <ExternalLink className="h-3 w-3" />
              </Button>
            </a>
          )}
          {next && (
            <Button
              size="sm"
              variant="outline"
              className="ml-auto h-7 px-2 text-xs"
              onClick={() => onMove(lead.id, next)}
              disabled={isPending}
            >
              {next}
              <ChevronRight className="h-3 w-3 ml-1" />
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
