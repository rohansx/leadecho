import { request } from "@/lib/api";

// automations (P2 #17): scheduled playbooks + prompt library.

export type AutomationKind = "daily_triage" | "lead_sweep" | "attribution_review" | "custom";

export interface AutomationSchedule {
  frequency: "daily" | "weekly";
  time: string;
  weekday: string;
  timezone: string;
}

export interface AutomationConfig {
  top_n?: number;
  mark_noise?: boolean;
  noise_below?: number;
  min_score?: number;
  max_leads?: number;
  push_to_crm?: boolean;
  days?: number;
  min_mentions?: number;
  email?: boolean;
  steps?: { tool: string; args?: Record<string, unknown> }[];
}

export interface Automation {
  id: string;
  name: string;
  kind: AutomationKind;
  enabled: boolean;
  schedule: AutomationSchedule;
  config: AutomationConfig;
  last_run_at: string | null;
  created_at: string;
}

export type AutomationInput = Pick<Automation, "name" | "kind" | "enabled" | "schedule" | "config">;

export interface AutomationItem {
  mention_id?: string;
  lead_id?: string;
  title: string;
  url?: string;
  platform?: string;
  score?: number;
  intent?: string;
  note?: string;
}

export interface AutomationOutput {
  summary?: string;
  items?: AutomationItem[];
  suggestions?: string[];
  notes?: string[];
}

export interface AutomationRun {
  id: string;
  trigger: "schedule" | "manual";
  status: "running" | "succeeded" | "failed";
  output: AutomationOutput;
  error: string;
  started_at: string;
  finished_at: string | null;
}

export interface Skill {
  name: string;
  title: string;
  summary: string;
  tools: string[];
  schedulable?: string;
  prompt: string;
  content: string;
}

export function listAutomations(): Promise<Automation[]> {
  return request<Automation[]>("/automations");
}

export function createAutomation(input: AutomationInput): Promise<Automation> {
  return request<Automation>("/automations", { method: "POST", body: JSON.stringify(input) });
}

export function updateAutomation(id: string, input: AutomationInput): Promise<Automation> {
  return request<Automation>(`/automations/${id}`, { method: "PUT", body: JSON.stringify(input) });
}

export function deleteAutomation(id: string): Promise<{ status: string }> {
  return request<{ status: string }>(`/automations/${id}`, { method: "DELETE" });
}

export function runAutomation(id: string): Promise<AutomationRun> {
  return request<AutomationRun>(`/automations/${id}/run`, { method: "POST" });
}

export function listAutomationRuns(id: string): Promise<AutomationRun[]> {
  return request<AutomationRun[]>(`/automations/${id}/runs?limit=10`);
}

export function listSkills(): Promise<Skill[]> {
  return request<Skill[]>("/skills");
}
