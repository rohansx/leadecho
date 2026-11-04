import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { AUTOMATIONS_KEY, AutomationCard, KIND_INFO } from "@/components/automations/automation-card";
import { PromptLibrary } from "@/components/automations/prompt-library";
import { createAutomation, listAutomations, type AutomationKind } from "@/lib/automations-api";

// automations (P2 #17): scheduled playbooks + prompt library.

export const Route = createFileRoute("/_dashboard/automations")({
  component: AutomationsPage,
});

const PRESETS: { kind: AutomationKind; frequency: "daily" | "weekly"; steps?: { tool: string }[] }[] = [
  { kind: "daily_triage", frequency: "daily" },
  { kind: "lead_sweep", frequency: "daily" },
  { kind: "attribution_review", frequency: "weekly" },
  { kind: "custom", frequency: "weekly", steps: [{ tool: "weekly_summary" }] },
];

const browserTZ = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

function AutomationsPage() {
  const queryClient = useQueryClient();
  const { data, isError } = useQuery({ queryKey: AUTOMATIONS_KEY, queryFn: listAutomations });
  const add = useMutation({
    mutationFn: (p: (typeof PRESETS)[number]) =>
      createAutomation({
        name: KIND_INFO[p.kind].label,
        kind: p.kind,
        enabled: false,
        schedule: { frequency: p.frequency, time: "09:30", weekday: "monday", timezone: browserTZ },
        config: p.steps ? { steps: p.steps } : {},
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: AUTOMATIONS_KEY }),
  });

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <Text as="h2">Automations</Text>
        <Text as="p" className="text-sm text-muted-foreground">
          Playbooks that run on a schedule. Results land in the run history and your notifications, and can be emailed.
        </Text>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {PRESETS.map((p) => (
          <Button key={p.kind} size="sm" variant="outline" onClick={() => add.mutate(p)} disabled={add.isPending}>
            <Plus className="h-3.5 w-3.5 mr-1" /> {KIND_INFO[p.kind].label}
          </Button>
        ))}
        {add.isError && <Text as="p" className="text-sm text-destructive">{add.error.message}</Text>}
      </div>

      {isError && <Text as="p" className="text-sm text-destructive">Could not load automations.</Text>}
      {data && data.length === 0 && (
        <Card>
          <CardContent className="p-8 text-center">
            <Text as="p" className="text-muted-foreground">
              No automations yet. Add one above; it stays off until you switch it on.
            </Text>
          </CardContent>
        </Card>
      )}
      {data?.map((a) => <AutomationCard key={a.id} automation={a} />)}

      <PromptLibrary />
    </div>
  );
}
