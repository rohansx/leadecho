// workflows: real-time rules (templates + When/If/Then form) and scheduled automations.
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { cn } from "@/lib/utils";
import { WorkflowCard } from "@/components/workflows/workflow-card";
import { WorkflowEditor } from "@/components/workflows/workflow-editor";
import { ScheduledAutomations } from "@/components/automations/scheduled-panel";
import { TEMPLATES, blankDefinition, describeActions, type WorkflowDefinition } from "@/lib/workflow-templates";
import { WORKFLOWS_KEY, createWorkflow, listWorkflows, toDefinition, updateWorkflow } from "@/lib/workflows-api";

type Tab = "realtime" | "scheduled";

export const Route = createFileRoute("/_dashboard/workflows")({
  validateSearch: (s: Record<string, unknown>): { tab?: Tab } => ({
    tab: s.tab === "scheduled" ? "scheduled" : undefined,
  }),
  component: WorkflowsPage,
});

// What the editor is working on: a new workflow (from a template or blank) or an existing one.
type Editing = { id?: string; def: WorkflowDefinition } | null;

function Templates({ onPick }: { onPick: (d: WorkflowDefinition) => void }) {
  return (
    <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
      {TEMPLATES.map((t) => (
        <button
          key={t.id}
          type="button"
          onClick={() => onPick(t.def)}
          className="rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-primary/60 hover:bg-accent"
        >
          <span className="block text-sm font-medium">{t.title}</span>
          <span className="mt-0.5 block text-xs text-muted-foreground">{t.why}</span>
          <span className="mt-2 block text-[11px] text-primary-ink">{describeActions(t.def.actions)}</span>
        </button>
      ))}
    </div>
  );
}

function RealTime() {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<Editing>(null);
  const [showTemplates, setShowTemplates] = useState(false);
  const { data, isLoading, isError } = useQuery({ queryKey: WORKFLOWS_KEY, queryFn: listWorkflows });
  const save = useMutation({
    mutationFn: (e: { id?: string; def: WorkflowDefinition }) => (e.id ? updateWorkflow(e.id, e.def) : createWorkflow(e.def)),
    onSuccess: () => {
      setEditing(null);
      setShowTemplates(false);
      qc.invalidateQueries({ queryKey: WORKFLOWS_KEY });
    },
  });
  const start = (def: WorkflowDefinition) => {
    save.reset();
    setEditing({ def: structuredClone(def) });
  };
  const empty = !isLoading && (data?.length ?? 0) === 0;

  if (editing) {
    return (
      <WorkflowEditor
        key={editing.id ?? "new"}
        initial={editing.def}
        saving={save.isPending}
        error={save.error?.message}
        onCancel={() => setEditing(null)}
        onSave={(def) => save.mutate({ id: editing.id, def })}
      />
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Text as="p" className="max-w-2xl text-sm text-muted-foreground">
          Runs the moment a new post is scored: get pinged, have a reply drafted, or send it to your pipeline, CRM or
          any webhook. Nothing is ever posted without you.
        </Text>
        {!empty && (
          <div className="flex gap-2">
            <Button size="sm" variant="outline" onClick={() => setShowTemplates(!showTemplates)}>
              <Sparkles className="h-3.5 w-3.5" /> Templates
            </Button>
            <Button size="sm" variant="accent" onClick={() => start(blankDefinition())}>
              <Plus className="h-3.5 w-3.5" /> New workflow
            </Button>
          </div>
        )}
      </div>

      {isError && <Text as="p" className="text-sm text-destructive">Could not load workflows.</Text>}

      {(empty || showTemplates) && (
        <div className="space-y-2">
          {empty && <Text as="p" className="text-sm font-medium">Start from a template</Text>}
          <Templates onPick={start} />
          {empty && (
            <button type="button" className="text-xs text-muted-foreground hover:text-foreground" onClick={() => start(blankDefinition())}>
              or build one from scratch →
            </button>
          )}
        </div>
      )}

      <div className="space-y-3">
        {data?.map((w) => (
          <WorkflowCard key={w.id} workflow={w} onEdit={() => setEditing({ id: w.id, def: toDefinition(w) })} />
        ))}
      </div>
    </div>
  );
}

function WorkflowsPage() {
  const { tab = "realtime" } = Route.useSearch();
  const navigate = useNavigate({ from: "/workflows" });
  const tabs: { id: Tab; label: string }[] = [
    { id: "realtime", label: "Real-time" },
    { id: "scheduled", label: "Scheduled" },
  ];
  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <Text as="h2">Workflows</Text>
      <div role="tablist" className="flex gap-1 border-b border-border">
        {tabs.map((t) => (
          <button
            key={t.id}
            role="tab"
            type="button"
            aria-selected={tab === t.id}
            onClick={() => navigate({ search: t.id === "realtime" ? {} : { tab: t.id } })}
            className={cn(
              "-mb-px border-b-2 px-3 py-2 text-sm transition-colors",
              tab === t.id ? "border-primary font-medium text-foreground" : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === "scheduled" ? <ScheduledAutomations /> : <RealTime />}
    </div>
  );
}
