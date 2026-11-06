// billing: Settings > Billing: plan, credit balance, usage, upgrade and top-up.
import { useEffect, useRef } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Check, CreditCard, ExternalLink } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { cn } from "@/lib/utils";
import { creditState, formatPrice } from "@/lib/credits";
import {
  BILLING_QUERY,
  USAGE_LABELS,
  getBilling,
  openBillingPortal,
  startCheckout,
  type BillingState,
  type Plan,
} from "@/lib/billing-api";

interface PlanTileProps {
  plan: Plan;
  current: boolean;
  canBuy: boolean;
  busy: boolean;
  onChoose: () => void;
}

function PlanTile({ plan, current, canBuy, busy, onChoose }: PlanTileProps) {
  return (
    <div className={cn("flex flex-col gap-2 rounded-lg border p-4", current ? "border-primary bg-primary/5" : "border-border")}>
      <div className="flex items-baseline justify-between">
        <Text as="p" className="font-medium">{plan.name}</Text>
        <Text as="p" className="text-sm">
          {formatPrice(plan.price_cents)}
          {plan.price_cents > 0 && <span className="text-muted-foreground">/mo</span>}
        </Text>
      </div>
      <ul className="space-y-1 text-[13px] text-muted-foreground">
        <li>{plan.monthly_credits.toLocaleString()} credits / month</li>
        <li>{plan.keyword_limit} active keywords</li>
      </ul>
      {current ? (
        <Badge variant="outline" size="sm" className="mt-auto w-fit"><Check className="h-3 w-3" /> Current plan</Badge>
      ) : plan.price_cents > 0 && canBuy ? (
        <Button size="sm" variant="accent" className="mt-auto" disabled={busy} onClick={onChoose}>
          {busy ? "Opening checkout…" : `Choose ${plan.name}`}
        </Button>
      ) : null}
    </div>
  );
}

function UsageList({ usage }: { usage: BillingState["usage_30d"] }) {
  const rows = usage.filter((u) => u.credits > 0);
  if (rows.length === 0) {
    return <Text as="p" className="text-sm text-muted-foreground">No credits used in the last 30 days.</Text>;
  }
  return (
    <ul className="space-y-1 text-sm">
      {rows.map((u) => (
        <li key={u.reason} className="flex justify-between">
          <span className="text-muted-foreground">{USAGE_LABELS[u.reason] ?? u.reason}</span>
          <span className="tabular-nums">{u.credits.toLocaleString()}</span>
        </li>
      ))}
    </ul>
  );
}

export function BillingCard() {
  const ref = useRef<HTMLDivElement>(null);
  const { data, isLoading, error } = useQuery({ queryKey: BILLING_QUERY, queryFn: getBilling });

  const checkout = useMutation({
    mutationFn: startCheckout,
    onSuccess: (url) => window.location.assign(url),
  });
  const portal = useMutation({
    mutationFn: openBillingPortal,
    onSuccess: (url) => window.open(url, "_blank", "noopener"),
  });

  // Back from Dodo checkout (?tab=billing): bring this card into view.
  useEffect(() => {
    if (new URLSearchParams(window.location.search).get("tab") === "billing") {
      ref.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [data]);

  const plan = data?.plans.find((p) => p.id === data.plan);
  const state = data && plan ? creditState(data.credit_balance, plan.monthly_credits, data.enforced) : "ok";
  const busyItem = checkout.isPending ? checkout.variables : undefined;

  return (
    <div ref={ref} id="billing">
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <CreditCard className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>Billing &amp; credits</CardTitle>
            <CardDescription>
              Credits pay for AI scoring, paid searches, reply drafts and chat. Free sources (Hacker News, Dev.to, Bluesky) cost nothing.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        {isLoading && <Text as="p" className="text-sm text-muted-foreground">Loading…</Text>}
        {error && <Text as="p" className="text-sm text-destructive">Could not load billing: {error.message}</Text>}
        {data && (
          <>
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="rounded-lg border border-border p-3">
                <Text as="p" className="text-xs text-muted-foreground">Credits left</Text>
                <Text as="p" className={cn("text-2xl font-semibold tabular-nums", state === "out" && "text-destructive", state === "low" && "text-amber-600 dark:text-amber-400")}>
                  {data.enforced ? data.credit_balance.toLocaleString() : "Unlimited"}
                </Text>
              </div>
              <div className="rounded-lg border border-border p-3">
                <Text as="p" className="text-xs text-muted-foreground">Plan</Text>
                <Text as="p" className="text-2xl font-semibold">{plan?.name ?? data.plan}</Text>
                {data.plan_renews_at && (
                  <Text as="p" className="text-xs text-muted-foreground">Renews {new Date(data.plan_renews_at).toLocaleDateString()}</Text>
                )}
              </div>
              <div className="rounded-lg border border-border p-3">
                <Text as="p" className="text-xs text-muted-foreground">Active keywords</Text>
                <Text as="p" className="text-2xl font-semibold tabular-nums">
                  {data.keywords_used}
                  {data.enforced && <span className="text-base text-muted-foreground"> / {data.keyword_limit}</span>}
                </Text>
              </div>
            </div>

            <div className="space-y-2">
              <Text as="p" className="text-sm font-medium">Plans</Text>
              <div className="grid gap-3 sm:grid-cols-3">
                {data.plans.map((p) => (
                  <PlanTile
                    key={p.id}
                    plan={p}
                    current={p.id === data.plan}
                    canBuy={data.can_checkout}
                    busy={busyItem === p.id}
                    onChoose={() => checkout.mutate(p.id as "starter" | "pro")}
                  />
                ))}
              </div>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border p-4">
              <div>
                <Text as="p" className="text-sm font-medium">Top up {data.topup.credits.toLocaleString()} credits</Text>
                <Text as="p" className="text-xs text-muted-foreground">
                  {formatPrice(data.topup.price_cents)} one-time. Top-up credits never expire.
                </Text>
              </div>
              <div className="flex gap-2">
                {data.has_portal && (
                  <Button size="sm" variant="outline" disabled={portal.isPending} onClick={() => portal.mutate()}>
                    Manage billing <ExternalLink className="h-3.5 w-3.5" />
                  </Button>
                )}
                <Button size="sm" variant="accent" disabled={!data.can_checkout || busyItem === "topup"} onClick={() => checkout.mutate("topup")}>
                  {busyItem === "topup" ? "Opening checkout…" : `Buy for ${formatPrice(data.topup.price_cents)}`}
                </Button>
              </div>
            </div>

            {(checkout.error || portal.error) && (
              <Text as="p" className="text-sm text-destructive">{(checkout.error ?? portal.error)?.message}</Text>
            )}
            {!data.can_checkout && (
              <Text as="p" className="text-xs text-muted-foreground">Payments aren't configured on this server.</Text>
            )}

            <div className="space-y-2">
              <Text as="p" className="text-sm font-medium">Last 30 days</Text>
              <UsageList usage={data.usage_30d} />
              <Text as="p" className="text-xs text-muted-foreground">
                Costs: {data.costs.post} credit per post scored, {data.costs.search} per paid search,{" "}
                {data.costs.draft} per reply draft, {data.costs.chat} per chat question.
              </Text>
            </div>
          </>
        )}
      </CardContent>
    </Card>
    </div>
  );
}
