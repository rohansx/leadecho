// demand: what people in the customer's market are asking for, grouped into
// themes, with who asked and a follow-up once a theme ships.
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronDown, ChevronRight, ExternalLink, Loader2, RefreshCw, Rocket } from "lucide-react";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { INBOX_QUEUES } from "@/lib/constants";
import { DEMAND_KEY, getDemand, refreshDemand, shipTheme } from "@/lib/demand-api";
import { askSource, coverageText, followUpEstimate, peopleLine, splitThemes, type CoverageTone, type DemandResponse, type DemandTheme } from "@/lib/demand";

export const Route = createFileRoute("/_dashboard/demand")({
  component: DemandPage,
});

const toneClass: Record<CoverageTone, string> = {
  good: "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  warn: "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  gap: "border-foreground/30 bg-foreground/5 text-foreground",
  muted: "border-border bg-transparent text-muted-foreground",
};

function daysAgo(iso: string): string {
  const d = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 86_400_000));
  return d === 0 ? "today" : d === 1 ? "yesterday" : `${d} days ago`;
}

function ShipPanel({ theme, meta, onDone }: { theme: DemandTheme; meta: DemandResponse; onDone: () => void }) {
  const qc = useQueryClient();
  const [note, setNote] = useState("");
  const [drafts, setDrafts] = useState(true);
  const est = followUpEstimate(theme, meta.max_follow_ups, meta.follow_up_cost);
  const ship = useMutation({
    mutationFn: () => shipTheme(theme.id, note.trim(), drafts && est.drafts > 0),
    onSuccess: onDone,
    // Also on failure: "already shipped" means the page is out of date.
    onSettled: () => qc.invalidateQueries({ queryKey: DEMAND_KEY }),
  });

  return (
    <div className="mt-3 space-y-3 rounded-lg border border-border bg-background p-3">
      <label className="block text-sm font-medium" htmlFor={`ship-${theme.id}`}>
        What did you ship for this?
      </label>
      <Textarea
        id={`ship-${theme.id}`}
        value={note}
        onChange={(e) => setNote(e.target.value)}
        maxLength={600}
        rows={3}
        placeholder="One or two sentences, in plain words. The follow-ups only claim what you write here."
      />
      {est.drafts > 0 && (
        <label className="flex items-start gap-2 text-sm">
          <input type="checkbox" className="mt-1" checked={drafts} onChange={(e) => setDrafts(e.target.checked)} />
          <span>
            Draft a follow-up for the {est.drafts === 1 ? "buyer" : `${est.drafts} buyers`} who asked (up to {est.credits} credits).
            <span className="block text-xs text-muted-foreground">
              A person is skipped, at no charge, when what you shipped does not answer what they asked. Drafts are saved on each lead for you to review. Nothing is sent
              automatically, and a reply to an old post can read as unwelcome, so check the post and the community's rules first.
            </span>
          </span>
        </label>
      )}
      {ship.isError && <p className="text-xs text-destructive">{ship.error instanceof Error ? ship.error.message : "Could not save. Try again."}</p>}
      <div className="flex gap-2">
        <Button size="sm" onClick={() => ship.mutate()} disabled={!note.trim() || ship.isPending}>
          {ship.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
          Mark as shipped
        </Button>
        <Button size="sm" variant="ghost" onClick={onDone} disabled={ship.isPending}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

function ThemeCard({ theme, meta }: { theme: DemandTheme; meta: DemandResponse }) {
  const [open, setOpen] = useState(false);
  const [shipping, setShipping] = useState(false);
  const cov = coverageText(theme.coverage);
  const shipped = theme.status === "shipped";
  const quotes = theme.asks.slice(0, 3);

  return (
    <article className={cn("rounded-xl border border-border bg-card p-5", shipped && "opacity-80")}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="font-[family-name:var(--font-head)] text-lg font-medium leading-snug">{theme.title}</h3>
          <p className="mt-0.5 text-sm text-muted-foreground">{peopleLine(theme)}</p>
        </div>
        <span className={cn("shrink-0 rounded-full border px-2.5 py-1 text-xs font-medium", shipped ? toneClass.good : toneClass[cov.tone])}>
          {shipped ? `Shipped ${theme.shipped_at ? daysAgo(theme.shipped_at) : ""}`.trim() : cov.label}
        </span>
      </div>

      {theme.summary && <p className="mt-3 text-sm leading-relaxed text-foreground-soft">{theme.summary}</p>}
      {!shipped && theme.why && (
        <p className="mt-1 text-xs text-muted-foreground">
          {cov.label}: {theme.why}
        </p>
      )}
      {shipped && theme.shipped_note && (
        <p className="mt-2 text-sm text-foreground-soft">
          <span className="font-medium text-foreground">What you shipped:</span> {theme.shipped_note}
        </p>
      )}
      {shipped && (
        <p className="mt-1 text-xs text-muted-foreground">
          {theme.drafted > 0
            ? `${theme.drafted} follow-up ${theme.drafted === 1 ? "draft" : "drafts"} saved. Open each lead below to review and send.`
            : "No follow-ups were drafted."}
        </p>
      )}
      {!shipped && theme.coverage === "covered" && (
        <p className="mt-2 text-xs text-muted-foreground">This looks close to what you already offer. Read each post before you reply: not everyone here is looking for a product.</p>
      )}

      <ul className="mt-3 space-y-2">
        {quotes.map((a) => (
          <li key={a.mention_id} className="border-l-2 border-border pl-3 text-sm">
            <span className="text-foreground">{a.quote}</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              {askSource(a)}, {daysAgo(a.at)}
            </span>
          </li>
        ))}
      </ul>

      {theme.competitors.length > 0 && (
        <p className="mt-3 text-xs text-muted-foreground">
          Products people named: <span className="text-foreground">{theme.competitors.join(", ")}</span>
        </p>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
          {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          {open ? "Hide" : "See"} who asked ({theme.people})
        </Button>
        {!shipped && !shipping && theme.coverage !== "covered" && (
          <Button size="sm" variant="ghost" onClick={() => setShipping(true)}>
            <Rocket className="h-3.5 w-3.5" />
            We shipped this
          </Button>
        )}
      </div>

      {shipping && <ShipPanel theme={theme} meta={meta} onDone={() => setShipping(false)} />}

      {open && (
        <ul className="mt-3 divide-y divide-border rounded-lg border border-border">
          {theme.asks.map((a) => (
            <li key={a.mention_id} className="flex items-start justify-between gap-3 px-3 py-2 text-sm">
              <div className="min-w-0">
                <p className="text-foreground">{a.quote}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {a.author ? `${a.author}, ` : ""}
                  {a.buyer ? "fits your buyer profile" : "role unknown"}, {a.platform}, {daysAgo(a.at)}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-3 text-xs">
                <Link to="/inbox" search={{ queue: INBOX_QUEUES.ALL, id: a.mention_id }} className="text-primary-ink underline underline-offset-2">
                  Open lead
                </Link>
                {/^https?:\/\//i.test(a.url) && (
                  <a href={a.url} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground">
                    Source <ExternalLink className="h-3 w-3" />
                  </a>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </article>
  );
}

function DemandPage() {
  const qc = useQueryClient();
  const demand = useQuery({ queryKey: DEMAND_KEY, queryFn: getDemand });
  const refresh = useMutation({
    mutationFn: refreshDemand,
    onSettled: () => qc.invalidateQueries({ queryKey: DEMAND_KEY }),
  });
  const d = demand.data;
  const { open, once, outside, shipped } = splitThemes(d?.themes ?? [], d?.min_theme_people ?? 2);
  const canGroup = !!d && d.waiting >= d.min_new_asks;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <Text as="h2">Demand</Text>
          <Text as="p" className="mt-1 text-muted-foreground">
            What people in your market are asking for, summarised from the posts LeadEcho reads. See what keeps coming up, and tell the buyers who asked once you ship it.
          </Text>
        </div>
        {d && (
          <div className="text-right">
            <Button size="sm" variant="outline" onClick={() => refresh.mutate()} disabled={!canGroup || refresh.isPending}>
              {refresh.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
              Group new asks{d.waiting > 0 ? ` (${d.waiting})` : ""}
            </Button>
            <p className="mt-1 text-xs text-muted-foreground">
              {d.last_run ? `Last grouped ${daysAgo(d.last_run)}. Runs daily.` : "Runs daily once there are a few asks."}
            </p>
          </div>
        )}
      </div>

      {refresh.isError && (
        <p className="text-sm text-destructive">{refresh.error instanceof Error ? refresh.error.message : "Could not group the asks. Try again."}</p>
      )}
      {demand.isLoading && <p className="text-sm text-muted-foreground">Loading…</p>}
      {demand.isError && <p className="text-sm text-destructive">Could not load demand themes. Reload the page to try again.</p>}

      {d && d.themes.length === 0 && (
        <div className="rounded-xl border border-dashed border-border p-8 text-center">
          <Text as="p" className="font-medium">No themes yet</Text>
          <Text as="p" className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
            {d.waiting >= d.min_new_asks
              ? `${d.waiting} asks are waiting. Group them now, or they will be grouped on the next daily run.`
              : `Themes appear once LeadEcho has scored at least ${d.min_new_asks} posts where someone states a need. ${d.waiting} so far.`}
          </Text>
        </div>
      )}

      {open.length > 0 && (
        <section className="space-y-3" aria-label="Open themes">
          {open.map((t) => (
            <ThemeCard key={t.id} theme={t} meta={d as DemandResponse} />
          ))}
        </section>
      )}

      {once.length > 0 && (
        <section aria-label="Raised once">
          <Text as="h3" className="text-sm font-medium text-muted-foreground">Raised once</Text>
          <p className="mt-1 text-xs text-muted-foreground">One person each. Not a pattern yet; they move up when someone else asks for the same thing.</p>
          <div className="mt-3 space-y-3">
            {once.map((t) => (
              <ThemeCard key={t.id} theme={t} meta={d as DemandResponse} />
            ))}
          </div>
        </section>
      )}

      {outside.length > 0 && (
        <details>
          <summary className="cursor-pointer text-sm font-medium text-muted-foreground">Outside your product ({outside.length})</summary>
          <p className="mt-1 text-xs text-muted-foreground">Needs that call for a different kind of product. Shown so you can check the call, not as a to-do.</p>
          <div className="mt-3 space-y-3">
            {outside.map((t) => (
              <ThemeCard key={t.id} theme={t} meta={d as DemandResponse} />
            ))}
          </div>
        </details>
      )}

      {shipped.length > 0 && (
        <section className="space-y-3" aria-label="Shipped themes">
          <Text as="h3" className="text-sm font-medium text-muted-foreground">Shipped</Text>
          {shipped.map((t) => (
            <ThemeCard key={t.id} theme={t} meta={d as DemandResponse} />
          ))}
        </section>
      )}
    </div>
  );
}
