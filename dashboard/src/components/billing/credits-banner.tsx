// billing: inbox banner when credits are out (scans paused) or running low.
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { BILLING_QUERY, getBilling } from "@/lib/billing-api";
import { creditState } from "@/lib/credits";
import { cn } from "@/lib/utils";

export function CreditsBanner() {
  const { data } = useQuery({ queryKey: BILLING_QUERY, queryFn: getBilling, staleTime: 60_000 });
  if (!data) return null;
  const plan = data.plans.find((p) => p.id === data.plan);
  const state = creditState(data.credit_balance, plan?.monthly_credits ?? 0, data.enforced);
  if (state === "ok") return null;

  return (
    <div
      role="status"
      className={cn(
        "fixed left-1/2 top-[calc(var(--header-height)+0.75rem)] z-40 flex w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 flex-wrap items-center gap-2 rounded-lg border bg-card px-4 py-3 text-sm shadow-lg",
        state === "out" ? "border-destructive/50" : "border-amber-500/50",
      )}
    >
      <AlertTriangle className={cn("h-4 w-4 shrink-0", state === "out" ? "text-destructive" : "text-amber-600 dark:text-amber-400")} />
      <span className="flex-1">
        {state === "out"
          ? "You're out of credits. Scanning and AI scoring are paused; new posts will be scored once you top up."
          : `Only ${data.credit_balance.toLocaleString()} credits left this month.`}
      </span>
      <a href="/settings?tab=billing" className="font-medium underline underline-offset-2">
        {state === "out" ? "Top up or upgrade" : "View plans"}
      </a>
    </div>
  );
}
