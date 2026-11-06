// workflows: real-time rules API client.
import { request } from "./api";
import type { WorkflowAction, WorkflowDefinition, WorkflowTrigger } from "./workflow-templates";

export interface Workflow extends Required<Omit<WorkflowDefinition, "max_per_hour">> {
  id: string;
  max_per_hour: number;
  run_count: number;
  last_run_at: string | null;
  created_at: string;
}

export interface WorkflowStep {
  step: number;
  type: WorkflowAction["type"];
  status: "completed" | "failed" | "skipped";
  detail?: string;
  lead_id?: string;
}

export interface WorkflowRun {
  id: string;
  status: string;
  steps: WorkflowStep[];
  error?: string;
  created_at: string;
  mention_id?: string;
  platform?: string;
  title?: string;
  url?: string;
  score?: number;
}

export interface PreviewLead {
  mention_id: string;
  platform: string;
  title?: string;
  content: string;
  url: string;
  author?: string;
  score: number;
}

export interface PreviewResult {
  checked: number;
  matched: number;
  samples: PreviewLead[];
}

export const WORKFLOWS_KEY = ["workflows"];

export const listWorkflows = () => request<{ data: Workflow[] }>("/workflows").then((r) => r.data);

export const createWorkflow = (d: WorkflowDefinition) =>
  request<Workflow>("/workflows", { method: "POST", body: JSON.stringify(d) });

export const updateWorkflow = (id: string, d: WorkflowDefinition) =>
  request<Workflow>(`/workflows/${id}`, { method: "PUT", body: JSON.stringify(d) });

export async function deleteWorkflow(id: string): Promise<void> {
  const res = await fetch(`/api/v1/workflows/${id}`, { method: "DELETE" });
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error || `Delete failed (${res.status})`);
  }
}

export const listWorkflowRuns = (id: string) =>
  request<{ data: WorkflowRun[] }>(`/workflows/${id}/runs`).then((r) => r.data);

export const previewWorkflow = (t: WorkflowTrigger) =>
  request<PreviewResult>("/workflows/preview", { method: "POST", body: JSON.stringify(t) });

export function toDefinition(w: Workflow): WorkflowDefinition {
  return { name: w.name, enabled: w.enabled, trigger: w.trigger, actions: w.actions, max_per_hour: w.max_per_hour };
}
