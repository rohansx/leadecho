// ops: operator console. Who may spend the operator's treg balance, comped
// plans, credit adjustments, pauses, spend, and an audit trail.
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ShieldCheck } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { canRequestScan, manualScanStatus, scanPending, type ScanKind } from "@/lib/ops-scan";
import { cn } from "@/lib/utils";
import {
  OPS_KEYS,
  adjustCredits,
  getOpsOverview,
  listOpsAudit,
  listOpsWorkspaces,
  scanNow,
  setPaidData,
  setPaidDataPaused,
  setScanning,
  setWorkspacePlan,
  usd,
  type OpsWorkspace,
} from "@/lib/ops-api";

// "Scan now": runs the same scan as the schedule, without waiting for it.
// Spend caps and the customer's credits apply as on any scan.
function ScanNowCell({ ws, busy, onScan }: { ws: OpsWorkspace; busy: boolean; onScan: (kind: ScanKind) => void }) {
  const now = new Date();
  const blocked = busy || !canRequestScan(ws.manual_scan, now);
  const status = manualScanStatus(ws.manual_scan, now);
  return (
    <div className="mt-1.5 space-y-1">
      <div className="flex gap-1">
        <Button size="sm" variant="outline" disabled={blocked} onClick={() => onScan("posts")} title="Crawl every keyword on every platform now">
          Scan posts
        </Button>
        <Button size="sm" variant="outline" disabled={blocked} onClick={() => onScan("hiring")} title="Run the hiring-signal scan now, without waiting for tomorrow">
          Scan hiring
        </Button>
      </div>
      {status && <p className="max-w-56 text-xs text-muted-foreground">{status}</p>}
    </div>
  );
}

export const Route = createFileRoute("/_dashboard/ops")({ component: OpsPage });

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <Card>
      <CardContent className="p-4">
        <Text as="p" className="text-xs text-muted-foreground">{label}</Text>
        <Text as="p" className="text-2xl font-semibold tabular-nums">{value}</Text>
        {sub && <Text as="p" className="text-xs text-muted-foreground">{sub}</Text>}
      </CardContent>
    </Card>
  );
}

function Toggle({ on, label, disabled, onChange }: { on: boolean; label: string; disabled?: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
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

function CreditsCell({ ws, onAdjust, busy }: { ws: OpsWorkspace; onAdjust: (delta: number) => void; busy: boolean }) {
  const [amount, setAmount] = useState("");
  const n = Number.parseInt(amount, 10);
  return (
    <div className="flex items-center gap-1.5">
      <span className="w-16 tabular-nums">{ws.credit_balance.toLocaleString()}</span>
      <Input className="h-7 w-20 px-2 text-xs" placeholder="±1000" inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value)} />
      <Button
        size="sm"
        variant="outline"
        className="h-7 px-2 text-xs"
        disabled={busy || !Number.isFinite(n) || n === 0}
        onClick={() => {
          onAdjust(n);
          setAmount("");
        }}
      >
        Apply
      </Button>
    </div>
  );
}

function OpsPage() {
  const qc = useQueryClient();
  const overview = useQuery({ queryKey: OPS_KEYS.overview, queryFn: getOpsOverview, retry: false });
  const workspaces = useQuery({
    queryKey: OPS_KEYS.workspaces,
    queryFn: listOpsWorkspaces,
    retry: false,
    // While a manual scan is waiting or running, keep its status current.
    refetchInterval: (query) => (scanPending(query.state.data?.data ?? []) ? 15_000 : false),
  });
  const audit = useQuery({ queryKey: OPS_KEYS.audit, queryFn: listOpsAudit, retry: false });
  const [filter, setFilter] = useState("");

  const refresh = () => qc.invalidateQueries({ queryKey: ["ops"] });
  const act = useMutation({ mutationFn: (fn: () => Promise<unknown>) => fn(), onSettled: refresh });

  if (overview.error) {
    return (
      <div className="mx-auto max-w-xl pt-16 text-center">
        <Text as="h2">Not available</Text>
        <Text as="p" className="mt-1 text-muted-foreground">This page is for the LeadEcho operator only.</Text>
      </div>
    );
  }

  const o = overview.data;
  const rows = (workspaces.data?.data ?? []).filter(
    (w) => !filter || `${w.name} ${w.owner_email}`.toLowerCase().includes(filter.toLowerCase()),
  );

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-primary-ink" />
          <Text as="h2">Operator console</Text>
        </div>
        {o && (
          <label className="flex items-center gap-2 text-sm">
            <Toggle
              on={!o.paid_data_paused}
              label="Paid data on"
              disabled={act.isPending}
              onChange={(on) => act.mutate(() => setPaidDataPaused(!on))}
            />
            {o.paid_data_paused ? "Paid data paused for everyone" : "Paid data on"}
          </label>
        )}
      </div>

      {o && (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Stat label="treg balance" value={o.treg_balance_usd !== undefined ? usd(o.treg_balance_usd) : "—"} sub={o.treg_balance_error} />
          <Stat label="treg today" value={usd(o.spend_today.treg_usd)} sub={`${o.spend_today.treg_calls.toLocaleString()} calls`} />
          <Stat label="AI today" value={usd(o.spend_today.ai_usd)} />
          <Stat label="Last 30 days" value={usd(o.spend_30d.treg_usd + o.spend_30d.ai_usd)} sub={`treg ${usd(o.spend_30d.treg_usd)} · AI ${usd(o.spend_30d.ai_usd)}`} />
        </div>
      )}
      {o && !o.credits_enforced && (
        <Text as="p" className="text-xs text-amber-600 dark:text-amber-400">
          Credits are not enforced on this server (set CREDITS_ENFORCED=true or DODO_API_KEY), so paid-data access and balances are not gating anything yet.
        </Text>
      )}

      <Card>
        <CardContent className="space-y-3 p-4">
          <div className="flex items-center justify-between gap-3">
            <Text as="p" className="font-medium">Workspaces ({rows.length})</Text>
            <Input className="h-8 max-w-xs" placeholder="Filter by name or email" value={filter} onChange={(e) => setFilter(e.target.value)} />
          </div>
          {act.error && <Text as="p" className="text-sm text-destructive">{act.error.message}</Text>}
          <div className="overflow-x-auto">
            <table className="w-full text-left text-[13px]">
              <thead className="text-xs text-muted-foreground">
                <tr className="border-b border-border">
                  <th className="py-2 pr-3 font-medium">Workspace</th>
                  <th className="py-2 pr-3 font-medium">Plan</th>
                  <th className="py-2 pr-3 font-medium">Credits</th>
                  <th className="py-2 pr-3 font-medium" title="May spend your treg balance">Paid data</th>
                  <th className="py-2 pr-3 font-medium">Scanning</th>
                  <th className="py-2 pr-3 font-medium">Keywords</th>
                  <th className="py-2 pr-3 font-medium">30d: credits · treg · AI</th>
                  <th className="py-2 font-medium">Last post</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((w) => (
                  <tr key={w.id} className="border-b border-border/60 align-middle">
                    <td className="py-2 pr-3">
                      <div className="font-medium">{w.name}</div>
                      <div className="text-xs text-muted-foreground">{w.owner_email || "no users"}</div>
                    </td>
                    <td className="py-2 pr-3">
                      <div className="flex items-center gap-1.5">
                        <select
                          aria-label={`Plan for ${w.name}`}
                          className="h-7 rounded-md border border-border bg-background px-1.5 text-xs"
                          value={w.plan}
                          disabled={act.isPending}
                          onChange={(e) => {
                            // Read the choice now: by the time the mutation runs, React
                            // has reset this controlled select to the old plan.
                            const plan = e.target.value;
                            act.mutate(() => setWorkspacePlan(w.id, plan));
                          }}
                        >
                          <option value="free">Free</option>
                          <option value="starter">Starter</option>
                          <option value="pro">Pro</option>
                        </select>
                        <Badge variant="outline" size="sm">{w.plan_source}</Badge>
                      </div>
                    </td>
                    <td className="py-2 pr-3">
                      <CreditsCell ws={w} busy={act.isPending} onAdjust={(d) => act.mutate(() => adjustCredits(w.id, d, "ops console"))} />
                    </td>
                    <td className="py-2 pr-3">
                      <Toggle on={w.paid_data} label={`Paid data for ${w.name}`} disabled={act.isPending} onChange={(v) => act.mutate(() => setPaidData(w.id, v))} />
                    </td>
                    <td className="py-2 pr-3">
                      <Toggle on={!w.scanning_paused} label={`Scanning for ${w.name}`} disabled={act.isPending} onChange={(on) => act.mutate(() => setScanning(w.id, !on))} />
                      <ScanNowCell ws={w} busy={act.isPending} onScan={(kind) => act.mutate(() => scanNow(w.id, kind))} />
                    </td>
                    <td className="py-2 pr-3 tabular-nums">{w.active_keywords} / {w.keyword_limit}</td>
                    <td className="py-2 pr-3 tabular-nums text-muted-foreground">
                      {w.credits_used_30d.toLocaleString()} · {usd(w.treg_usd_30d)} · {usd(w.ai_usd_30d)}
                    </td>
                    <td className="py-2 text-xs text-muted-foreground">{new Date(w.last_post_at).toLocaleDateString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-2 p-4">
          <Text as="p" className="font-medium">Audit log</Text>
          {(audit.data?.data ?? []).length === 0 ? (
            <Text as="p" className="text-sm text-muted-foreground">No actions yet.</Text>
          ) : (
            <ul className="space-y-1 text-[13px]">
              {audit.data!.data.map((a, i) => (
                <li key={i} className="flex flex-wrap gap-x-2 text-muted-foreground">
                  <span className="tabular-nums">{new Date(a.created_at).toLocaleString()}</span>
                  <span className="text-foreground">{a.action}</span>
                  {a.workspace_name && <span>on {a.workspace_name}</span>}
                  <code className="text-xs">{JSON.stringify(a.detail)}</code>
                  <span>by {a.actor_email}</span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
