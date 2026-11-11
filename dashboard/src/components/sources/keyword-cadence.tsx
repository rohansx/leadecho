import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import {
  COST_PREVIEW_KEY,
  formatUSD,
  getCostPreview,
  updateKeywordCadence,
  type CadenceMap,
} from "@/lib/sources-api";
import {
  PAID_DEFAULT,
  effectiveCadence,
  keywordCost,
  paidSlowdownPlan,
  platformCost,
  scheduleSummary,
} from "@/lib/keyword-schedule";
import type { Keyword } from "@/lib/types";

export function useCostPreview() {
  return useQuery({ queryKey: [COST_PREVIEW_KEY], queryFn: getCostPreview });
}

/** Total estimated data cost across active keywords. */
export function MonthlyCostSummary() {
  const { data } = useCostPreview();
  if (!data) return null;
  return (
    <span className="text-xs text-muted-foreground">
      Est. data cost: <b className="text-foreground">{formatUSD(data.usd_per_month)}</b>
    </span>
  );
}

/** One-line schedule summary; per-platform cadence selects behind a disclosure. */
export function KeywordCadence({ keyword }: { keyword: Keyword }) {
  const queryClient = useQueryClient();
  const { data } = useCostPreview();
  const [open, setOpen] = useState(false);
  const save = useMutation({
    mutationFn: (cadence: CadenceMap) => updateKeywordCadence(keyword.id, cadence),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [COST_PREVIEW_KEY] }),
  });
  if (!data) return null;
  const stored = data.keywords.find((k) => k.keyword_id === keyword.id)?.cadence ?? {};
  const current = (!save.isError && save.variables) || stored;
  const total = keywordCost(keyword.platforms, current, data.rates);

  return (
    <div className="mt-2 min-w-0">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex max-w-full items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
      >
        <Clock className="h-3 w-3 shrink-0" />
        <span className="truncate">
          {scheduleSummary(keyword.platforms, current, data.rates)} · ~{formatUSD(total)}
        </span>
        <span className="shrink-0 underline underline-offset-2">{open ? "Hide schedule" : "Edit schedule"}</span>
        <ChevronDown className={`h-3 w-3 shrink-0 transition ${open ? "rotate-180" : ""}`} />
      </button>
      {open && (
        <div className="mt-2 grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-x-3 gap-y-1.5 rounded-lg bg-muted/50 p-2">
          {keyword.platforms.map((p) => {
            const c = effectiveCadence(p, current, data.rates);
            const cost = platformCost(p, c, data.rates);
            return (
              <label key={p} className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                <span className="truncate">{p}</span>
                <select
                  value={c}
                  onChange={(e) => save.mutate({ ...current, [p]: e.target.value })}
                  disabled={save.isPending}
                  aria-label={`Scan cadence for ${p}`}
                  className="h-6 shrink-0 rounded-md border border-border bg-card px-1 text-xs text-foreground"
                >
                  {data.options.map((o) => (
                    <option key={o} value={o}>
                      every {o}
                    </option>
                  ))}
                </select>
                {cost > 0 && <span className="shrink-0">{formatUSD(cost)}</span>}
              </label>
            );
          })}
        </div>
      )}
      {save.isError && <Text as="p" className="mt-1 text-xs text-destructive">{save.error.message}</Text>}
    </div>
  );
}

/** Slows every paid platform scanning faster than 24h down to 24h, across all keywords. */
export function BulkPaidCadenceButton({ keywords }: { keywords: Keyword[] }) {
  const queryClient = useQueryClient();
  const { data } = useCostPreview();
  const run = useMutation({
    // ponytail: sequential per-keyword PUTs; add a bulk endpoint if keyword counts reach the hundreds.
    mutationFn: async (updates: { id: string; cadence: CadenceMap }[]) => {
      for (const u of updates) await updateKeywordCadence(u.id, u.cadence);
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: [COST_PREVIEW_KEY] }),
  });
  if (!data) return null;
  const plan = paidSlowdownPlan(
    keywords.map((k) => ({
      id: k.id,
      platforms: k.platforms,
      isActive: k.is_active,
      cadence: data.keywords.find((c) => c.keyword_id === k.id)?.cadence ?? {},
    })),
    data.rates,
  );
  if (!plan.updates.length && !run.isError) return null;

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/50 p-2.5 text-xs">
      <Text as="span" className="min-w-0 flex-1 text-xs text-muted-foreground">
        {plan.updates.length} keyword{plan.updates.length !== 1 ? "s" : ""} scan paid sources more often than every {PAID_DEFAULT}
        {plan.saving > 0 && <> · save ~<b className="text-foreground">{formatUSD(plan.saving)}</b></>}
      </Text>
      <Button size="sm" variant="outline" disabled={run.isPending || !plan.updates.length} onClick={() => run.mutate(plan.updates)}>
        {run.isPending ? "Updating..." : `Set paid sources to every ${PAID_DEFAULT}`}
      </Button>
      {run.isError && <Text as="span" className="w-full text-xs text-destructive">{run.error.message}</Text>}
    </div>
  );
}
