import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import {
  COST_PREVIEW_KEY,
  formatUSD,
  getCostPreview,
  updateKeywordCadence,
  type CadenceMap,
} from "@/lib/sources-api";
import type { Keyword } from "@/lib/types";

const HOURS: Record<string, number> = { "15m": 0.25, "30m": 0.5, "1h": 1, "3h": 3, "6h": 6, "12h": 12, "24h": 24 };

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

/** Per-platform scan cadence selects with a live monthly cost preview. */
export function KeywordCadence({ keyword }: { keyword: Keyword }) {
  const queryClient = useQueryClient();
  const { data } = useCostPreview();
  const save = useMutation({
    mutationFn: (cadence: CadenceMap) => updateKeywordCadence(keyword.id, cadence),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [COST_PREVIEW_KEY] }),
  });
  if (!data) return null;
  const stored = data.keywords.find((k) => k.keyword_id === keyword.id)?.cadence ?? {};
  const current = (!save.isError && save.variables) || stored;

  const cost = (platform: string) => {
    const hours = HOURS[current[platform] ?? data.default] ?? 1;
    return Math.round(((30 * 24) / hours) * (data.rates[platform] ?? 0) * 1e4) / 1e4;
  };
  const total = keyword.platforms.reduce((sum, p) => sum + cost(p), 0);

  return (
    <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1.5">
      {keyword.platforms.map((p) => (
        <label key={p} className="flex items-center gap-1 text-xs text-muted-foreground">
          {p}
          <select
            value={current[p] ?? data.default}
            onChange={(e) => save.mutate({ ...current, [p]: e.target.value })}
            disabled={save.isPending}
            aria-label={`Scan cadence for ${p}`}
            className="h-6 rounded-md border border-border bg-card px-1 text-xs text-foreground"
          >
            {data.options.map((o) => (
              <option key={o} value={o}>
                every {o}
              </option>
            ))}
          </select>
          {cost(p) > 0 && <span>{formatUSD(cost(p))}</span>}
        </label>
      ))}
      <Text as="span" className="text-xs text-muted-foreground ml-auto">
        {formatUSD(total)}
      </Text>
      {save.isError && <Text as="span" className="text-xs text-destructive">{save.error.message}</Text>}
    </div>
  );
}
