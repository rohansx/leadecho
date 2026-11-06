// workflows: one workflow as a readable sentence, with on/off, edit, delete and run history.
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ChevronDown, Pencil, Trash2, Zap } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Text } from "@/components/ui/text";
import { cn } from "@/lib/utils";
import { ACTION_INFO, describeActions, describeTrigger } from "@/lib/workflow-templates";
import {
  WORKFLOWS_KEY,
  deleteWorkflow,
  listWorkflowRuns,
  toDefinition,
  updateWorkflow,
  type Workflow,
  type WorkflowRun,
} from "@/lib/workflows-api";

function Switch({ on, label, disabled, onChange }: { on: boolean; label: string; disabled?: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button" role="switch" aria-checked={on} aria-label={label} disabled={disabled}
      onClick={() => onChange(!on)}
      className={cn(
        "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors disabled:opacity-50",
        on ? "border-primary bg-primary" : "border-border bg-muted",
      )}
    >
      <span className={cn("inline-block h-3.5 w-3.5 rounded-full bg-background shadow transition-transform", on ? "translate-x-4" : "translate-x-0.5")} />
    </button>
  );
}

const STEP_TONE: Record<string, string> = {
  completed: "text-emerald-600 dark:text-emerald-400",
  skipped: "text-muted-foreground",
  failed: "text-destructive",
};

function RunRow({ run }: { run: WorkflowRun }) {
  return (
    <li className="space-y-1 border-b border-border/60 py-2 last:border-0">
      <div className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
        <span className="tabular-nums">{new Date(run.created_at).toLocaleString()}</span>
        {run.platform && <span>· {run.platform}</span>}
        {run.score != null && <span>· score {run.score.toFixed(1)}</span>}
        {run.mention_id && <Link to="/inbox" search={{ id: run.mention_id }} className="text-primary-ink hover:underline">Open</Link>}
      </div>
      {run.title && <p className="line-clamp-1 text-[13px]">{run.title}</p>}
      <ul className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs">
        {run.steps.map((s) => (
          <li key={s.step} className={STEP_TONE[s.status]} title={s.detail}>
            {ACTION_INFO[s.type]?.label ?? s.type}: {s.status === "completed" ? "done" : s.status}
            {s.status !== "completed" && s.detail ? ` — ${s.detail}` : ""}
          </li>
        ))}
      </ul>
    </li>
  );
}

export function WorkflowCard({ workflow, onEdit }: { workflow: Workflow; onEdit: () => void }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const runs = useQuery({ queryKey: [...WORKFLOWS_KEY, workflow.id, "runs"], queryFn: () => listWorkflowRuns(workflow.id), enabled: open });
  const refresh = () => qc.invalidateQueries({ queryKey: WORKFLOWS_KEY });
  const toggle = useMutation({ mutationFn: (on: boolean) => updateWorkflow(workflow.id, { ...toDefinition(workflow), enabled: on }), onSettled: refresh });
  const remove = useMutation({ mutationFn: () => deleteWorkflow(workflow.id), onSuccess: refresh });

  return (
    <Card className={cn(!workflow.enabled && "opacity-70")}>
      <CardContent className="space-y-2 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <Zap className={cn("h-4 w-4 shrink-0", workflow.enabled ? "text-primary-ink" : "text-muted-foreground")} />
              <Text as="p" className="font-medium">{workflow.name}</Text>
              {!workflow.enabled && <Badge size="sm" variant="outline">off</Badge>}
            </div>
            <p className="text-[13px] text-muted-foreground">
              <span className="font-medium text-foreground/80">When</span> {describeTrigger(workflow.trigger)}
            </p>
            <p className="text-[13px] text-muted-foreground">
              <span className="font-medium text-foreground/80">Then</span> {describeActions(workflow.actions)}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Switch on={workflow.enabled} label={`Turn ${workflow.name} on or off`} disabled={toggle.isPending} onChange={(v) => toggle.mutate(v)} />
            <Button variant="ghost" size="icon" aria-label="Edit workflow" onClick={onEdit}><Pencil className="h-4 w-4" /></Button>
            {confirmDelete ? (
              <Button variant="destructive" size="sm" disabled={remove.isPending} onClick={() => remove.mutate()} onBlur={() => setConfirmDelete(false)} autoFocus>
                Delete?
              </Button>
            ) : (
              <Button variant="ghost" size="icon" aria-label="Delete workflow" onClick={() => setConfirmDelete(true)}>
                <Trash2 className="h-4 w-4 text-muted-foreground" />
              </Button>
            )}
          </div>
        </div>
        {(toggle.error || remove.error) && <Text as="p" className="text-xs text-destructive">{(toggle.error || remove.error)?.message}</Text>}
        <button type="button" onClick={() => setOpen(!open)} className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <ChevronDown className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-180")} />
          {workflow.run_count > 0
            ? `Ran ${workflow.run_count} time${workflow.run_count === 1 ? "" : "s"}${workflow.last_run_at ? ` · last ${new Date(workflow.last_run_at).toLocaleString()}` : ""}`
            : "Hasn't run yet"}
        </button>
        {open && (
          <div className="rounded-md bg-muted/40 px-3">
            {runs.isLoading && <p className="py-2 text-xs text-muted-foreground">Loading…</p>}
            {runs.data?.length === 0 && <p className="py-2 text-xs text-muted-foreground">No runs yet. It runs as new posts are scored.</p>}
            <ul>{runs.data?.map((r) => <RunRow key={r.id} run={r} />)}</ul>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
