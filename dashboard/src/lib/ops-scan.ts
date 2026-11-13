// ops: an operator's "scan now" request for one workspace and its outcome
// (workspaces.settings.manual_scan). Pure helpers for the console.

export type ScanKind = "posts" | "hiring" | "prospects" | "rescore";

const SCAN_NAMES: Record<ScanKind, string> = { posts: "Posts scan", hiring: "Hiring scan", prospects: "Prospect scan", rescore: "Re-score" };

export interface ManualScan {
  kind: ScanKind;
  state: "requested" | "running" | "done";
  by?: string;
  requested_at: string;
  started_at?: string;
  finished_at?: string;
  /** Why a run did nothing. */
  note?: string;
}

const STALE_MS = 60 * 60 * 1000; // matches the server: an abandoned run stops blocking after an hour

const ago = (iso: string | undefined, now: Date): string => {
  const min = Math.max(0, Math.round((now.getTime() - new Date(iso ?? now).getTime()) / 60_000));
  return min < 60 ? `${min} min` : `${Math.round(min / 60)} h`;
};

/** One line on what the workspace's last manual scan is doing. "" when there was none. */
export function manualScanStatus(scan: ManualScan | null | undefined, now: Date): string {
  if (!scan) return "";
  const name = SCAN_NAMES[scan.kind] ?? "Posts scan";
  if (scan.state === "requested") return `${name} requested. It starts within a minute.`;
  if (scan.state === "running") return `${name} running for ${ago(scan.started_at, now)}.`;
  if (scan.note) return `${name} ended ${ago(scan.finished_at, now)} ago: ${scan.note}.`;
  // "ran", not "found something": the result is in the workspace's inbox.
  return `${name} ran, finished ${ago(scan.finished_at, now)} ago.`;
}

/** Whether a new manual scan may be asked for (the server checks this too). */
export function canRequestScan(scan: ManualScan | null | undefined, now: Date): boolean {
  if (!scan || scan.state === "done") return true;
  const since = scan.state === "running" ? scan.started_at : scan.requested_at;
  return now.getTime() - new Date(since ?? scan.requested_at).getTime() > STALE_MS;
}

/** Whether any workspace has a manual scan waiting or running (the console then refreshes itself). */
export function scanPending(workspaces: { manual_scan: ManualScan | null }[]): boolean {
  return workspaces.some((w) => w.manual_scan != null && w.manual_scan.state !== "done");
}
