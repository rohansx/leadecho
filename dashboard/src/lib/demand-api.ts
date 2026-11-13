// demand: API client.
import { request } from "./api";
import type { DemandResponse } from "./demand";

export const DEMAND_KEY = ["demand"];

export const getDemand = () => request<DemandResponse>("/demand");

export const refreshDemand = () => request<{ placed: number }>("/demand/refresh", { method: "POST" });

export const shipTheme = (id: string, note: string, drafts: boolean) =>
  request<{ theme_id: string; status: string; drafted: number }>(`/demand/themes/${id}/ship`, {
    method: "POST",
    body: JSON.stringify({ note, drafts }),
  });
