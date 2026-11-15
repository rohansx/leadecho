// competitor watch: the competitors LeadEcho found, what each one did lately,
// what it means for the customer, and who is unhappy with them.
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ChevronDown, ChevronRight, ExternalLink, Loader2, Lock, Pause, Play, Plus, RefreshCw, Trash2, X } from "lucide-react";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { INBOX_QUEUES } from "@/lib/constants";
import {
  COMPETITORS_KEY,
  MOVE_LABEL,
  SOURCE_LABEL,
  addCompetitor,
  addSource,
  competitorLine,
  findCompetitors,
  getCompetitorPeople,
  getCompetitors,
  removeCompetitor,
  removeSource,
  sourceState,
  updateCompetitor,
  type Competitor,
  type CompetitorMove,
  type CompetitorSource,
  type SourceKind,
} from "@/lib/competitors-api";

export const Route = createFileRoute("/_dashboard/competitors")({
  component: CompetitorsPage,
});

const isHttp = (u: string) => /^https?:\/\//i.test(u);

function shortDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function errorText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

function MoveRow({ move }: { move: CompetitorMove }) {
  const high = move.importance === "high";
  const source = move.urls.find(isHttp);
  return (
    <li className={cn("border-l-2 pl-3", high ? "border-amber-500" : "border-border")}>
      <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
        {high && <span className="rounded-full border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 font-medium text-amber-700 dark:text-amber-300">Important</span>}
        <span className="font-medium text-foreground">{MOVE_LABEL[move.kind]}</span>
        <span>{shortDate(move.at)}</span>
        {source && (
          <a href={source} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1 hover:text-foreground">
            Source <ExternalLink className="h-3 w-3" />
          </a>
        )}
      </p>
      <p className="mt-1 text-sm font-medium text-foreground">{move.title}</p>
      {move.summary && <p className="mt-0.5 text-sm text-foreground-soft">{move.summary}</p>}
      {move.impact && (
        <p className="mt-1 text-sm">
          <span className="font-medium">For you:</span> {move.impact}
        </p>
      )}
      {move.action && (
        <p className="text-sm">
          <span className="font-medium">Do:</span> {move.action}
        </p>
      )}
      {move.theme_title && (
        <p className="mt-0.5 text-xs text-muted-foreground">
          People are asking for this:{" "}
          <Link to="/demand" className="text-primary-ink underline underline-offset-2">
            {move.theme_title}
          </Link>
        </p>
      )}
    </li>
  );
}

function SourceRow({ competitorId, source }: { competitorId: string; source: CompetitorSource }) {
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: () => removeSource(competitorId, source.id),
    onSettled: () => qc.invalidateQueries({ queryKey: COMPETITORS_KEY }),
  });
  const state = sourceState(source);
  return (
    <li className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
      <div className="min-w-0">
        <span className="font-medium">{SOURCE_LABEL[source.kind]}</span>{" "}
        {isHttp(source.url) ? (
          <a href={source.url} target="_blank" rel="noreferrer noopener" className="break-all text-muted-foreground hover:text-foreground">
            {source.url.replace(/^https?:\/\/(www\.)?/, "")}
          </a>
        ) : (
          <span className="break-all text-muted-foreground">{source.url}</span>
        )}
        <p className="text-xs text-muted-foreground">
          {state === "broken" && (
            <span className="inline-flex items-center gap-1 text-destructive">
              <AlertTriangle className="h-3 w-3" /> Couldn't be read five times in a row, so it is no longer checked. Remove it, or add the right link.
            </span>
          )}
          {state === "locked" && (
            <span className="inline-flex items-center gap-1">
              <Lock className="h-3 w-3" /> X and LinkedIn need a paid plan.{" "}
              <Link to="/settings" search={{ tab: "billing" }} className="text-primary-ink underline underline-offset-2">
                Upgrade
              </Link>
            </span>
          )}
          {state === "waiting" && "Not checked yet. The first check only records where things stand."}
          {state === "ok" && source.last_checked_at && `Checked ${shortDate(source.last_checked_at)}. Checked once a day.`}
        </p>
      </div>
      <Button size="sm" variant="ghost" aria-label={`Remove ${SOURCE_LABEL[source.kind]}`} onClick={() => remove.mutate()} disabled={remove.isPending}>
        <X className="h-3.5 w-3.5" />
      </Button>
    </li>
  );
}

const ADDABLE: SourceKind[] = ["page_pricing", "changelog", "blog", "github", "x", "linkedin", "page_landing"];

function Sources({ competitor }: { competitor: Competitor }) {
  const qc = useQueryClient();
  const [kind, setKind] = useState<SourceKind>("page_pricing");
  const [url, setUrl] = useState("");
  const add = useMutation({
    mutationFn: () => addSource(competitor.id, kind, url.trim()),
    onSuccess: () => setUrl(""),
    onSettled: () => qc.invalidateQueries({ queryKey: COMPETITORS_KEY }),
  });
  return (
    <div className="mt-3 rounded-lg border border-border">
      {competitor.sources.length === 0 ? (
        <p className="px-3 py-2 text-sm text-muted-foreground">No sources found on their site. Add one below.</p>
      ) : (
        <ul className="divide-y divide-border">
          {competitor.sources.map((s) => (
            <SourceRow key={s.id} competitorId={competitor.id} source={s} />
          ))}
        </ul>
      )}
      <form
        className="flex flex-wrap items-center gap-2 border-t border-border px-3 py-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (url.trim()) add.mutate();
        }}
      >
        <label className="sr-only" htmlFor={`kind-${competitor.id}`}>
          Kind of source
        </label>
        <select
          id={`kind-${competitor.id}`}
          value={kind}
          onChange={(e) => setKind(e.target.value as SourceKind)}
          className="h-9 rounded-md border border-border bg-background px-2 text-sm"
        >
          {ADDABLE.map((k) => (
            <option key={k} value={k}>
              {SOURCE_LABEL[k]}
            </option>
          ))}
        </select>
        <label className="sr-only" htmlFor={`url-${competitor.id}`}>
          Link
        </label>
        <Input id={`url-${competitor.id}`} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="Link to watch" className="h-9 min-w-48 flex-1" />
        <Button size="sm" variant="outline" type="submit" disabled={!url.trim() || add.isPending}>
          {add.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
          Add
        </Button>
        {add.isError && <p className="w-full text-xs text-destructive">{errorText(add.error, "Could not add that link.")}</p>}
      </form>
    </div>
  );
}

function People({ competitor }: { competitor: Competitor }) {
  const people = useQuery({ queryKey: [...COMPETITORS_KEY, competitor.id, "people"], queryFn: () => getCompetitorPeople(competitor.id) });
  if (people.isLoading) return <p className="mt-3 text-sm text-muted-foreground">Loading…</p>;
  if (people.isError) return <p className="mt-3 text-sm text-destructive">Could not load these people. Try again.</p>;
  return (
    <ul className="mt-3 divide-y divide-border rounded-lg border border-border">
      {(people.data?.data ?? []).map((p) => (
        <li key={p.mention_id} className="flex items-start justify-between gap-3 px-3 py-2 text-sm">
          <div className="min-w-0">
            <p className="text-foreground">{p.quote}</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {p.author ? `${p.author}, ` : ""}
              {p.platform}, {shortDate(p.at)}
            </p>
          </div>
          <Link to="/inbox" search={{ queue: INBOX_QUEUES.ALL, id: p.mention_id }} className="shrink-0 text-xs text-primary-ink underline underline-offset-2">
            Open lead
          </Link>
        </li>
      ))}
    </ul>
  );
}

function CompetitorCard({ competitor }: { competitor: Competitor }) {
  const qc = useQueryClient();
  const [panel, setPanel] = useState<"sources" | "people" | null>(null);
  const [allMoves, setAllMoves] = useState(false);
  const refresh = () => qc.invalidateQueries({ queryKey: COMPETITORS_KEY });
  const toggle = useMutation({
    mutationFn: () => updateCompetitor(competitor.id, { status: competitor.status === "paused" ? "active" : "paused" }),
    onSettled: refresh,
  });
  const remove = useMutation({ mutationFn: () => removeCompetitor(competitor.id), onSettled: refresh });
  const paused = competitor.status === "paused";
  const moves = allMoves ? competitor.moves : competitor.moves.slice(0, 3);
  const Chevron = (open: boolean) => (open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />);

  return (
    <article className={cn("rounded-xl border border-border bg-card p-5", paused && "opacity-80")}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="font-[family-name:var(--font-head)] text-lg font-medium leading-snug">{competitor.name}</h3>
          <p className="mt-0.5 text-sm text-muted-foreground">
            {isHttp(competitor.website) && (
              <>
                <a href={competitor.website} target="_blank" rel="noreferrer noopener" className="hover:text-foreground">
                  {competitor.website.replace(/^https?:\/\/(www\.)?/, "")}
                </a>
                {" · "}
              </>
            )}
            {competitorLine(competitor)}
          </p>
        </div>
        <div className="flex shrink-0 gap-1">
          <Button size="sm" variant="ghost" onClick={() => toggle.mutate()} disabled={toggle.isPending}>
            {paused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
            {paused ? "Resume" : "Pause"}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-label={`Stop watching ${competitor.name}`}
            disabled={remove.isPending}
            onClick={() => {
              if (window.confirm(`Stop watching ${competitor.name}? Its moves and sources are deleted.`)) remove.mutate();
            }}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {competitor.summary && (
        <p className="mt-3 text-sm leading-relaxed text-foreground-soft">
          <span className="font-medium text-foreground">What they're building:</span> {competitor.summary}
        </p>
      )}

      {competitor.moves.length > 0 ? (
        <ul className="mt-4 space-y-4">
          {moves.map((m) => (
            <MoveRow key={m.id} move={m} />
          ))}
        </ul>
      ) : (
        <p className="mt-3 text-sm text-muted-foreground">
          Nothing has moved since we started watching. You'll see launches, pricing changes and repositioning here as they happen.
        </p>
      )}
      {competitor.moves.length > 3 && (
        <button type="button" className="mt-3 text-xs text-primary-ink underline underline-offset-2" onClick={() => setAllMoves((v) => !v)}>
          {allMoves ? "Show fewer" : `Show all ${competitor.moves.length} moves`}
        </button>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" onClick={() => setPanel(panel === "sources" ? null : "sources")} aria-expanded={panel === "sources"}>
          {Chevron(panel === "sources")}
          What we watch ({competitor.sources.length})
        </Button>
        {competitor.people > 0 && (
          <Button size="sm" variant="outline" onClick={() => setPanel(panel === "people" ? null : "people")} aria-expanded={panel === "people"}>
            {Chevron(panel === "people")}
            Unhappy with them ({competitor.people})
          </Button>
        )}
      </div>
      {(toggle.isError || remove.isError) && <p className="mt-2 text-xs text-destructive">Could not save that change. Try again.</p>}
      {panel === "sources" && <Sources competitor={competitor} />}
      {panel === "people" && <People competitor={competitor} />}
    </article>
  );
}

function AddCompetitor({ onDone }: { onDone: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [website, setWebsite] = useState("");
  const add = useMutation({
    mutationFn: () => addCompetitor(name.trim(), website.trim()),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: COMPETITORS_KEY });
      onDone();
    },
  });
  return (
    <form
      className="space-y-3 rounded-xl border border-border bg-card p-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (name.trim()) add.mutate();
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label className="text-sm font-medium" htmlFor="competitor-name">
            Name
          </label>
          <Input id="competitor-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} className="mt-1" />
        </div>
        <div>
          <label className="text-sm font-medium" htmlFor="competitor-website">
            Website <span className="font-normal text-muted-foreground">(optional, we'll look for it)</span>
          </label>
          <Input id="competitor-website" value={website} onChange={(e) => setWebsite(e.target.value)} placeholder="acme.com" className="mt-1" />
        </div>
      </div>
      {add.isError && <p className="text-xs text-destructive">{errorText(add.error, "Could not add this competitor.")}</p>}
      <div className="flex gap-2">
        <Button size="sm" type="submit" disabled={!name.trim() || add.isPending}>
          {add.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          Start watching
        </Button>
        <Button size="sm" variant="ghost" type="button" onClick={onDone} disabled={add.isPending}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

/** A name we could not resolve: the customer can give the website. */
function Unresolved({ name }: { name: string }) {
  const qc = useQueryClient();
  const [website, setWebsite] = useState("");
  const add = useMutation({
    mutationFn: () => addCompetitor(name, website.trim()),
    onSuccess: () => qc.invalidateQueries({ queryKey: COMPETITORS_KEY }),
  });
  return (
    <li>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (website.trim()) add.mutate();
        }}
      >
        <span className="min-w-32 text-sm font-medium">{name}</span>
        <label className="sr-only" htmlFor={`site-${name}`}>
          Website of {name}
        </label>
        <Input id={`site-${name}`} value={website} onChange={(e) => setWebsite(e.target.value)} placeholder="Their website" className="h-9 min-w-48 flex-1" />
        <Button size="sm" variant="outline" type="submit" disabled={!website.trim() || add.isPending}>
          {add.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
          Watch
        </Button>
      </form>
      {add.isError && <p className="mt-1 text-xs text-destructive">{errorText(add.error, "Could not add this competitor.")}</p>}
    </li>
  );
}

function Suggestion({ name }: { name: string }) {
  const qc = useQueryClient();
  const add = useMutation({
    mutationFn: () => addCompetitor(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: COMPETITORS_KEY }),
  });
  return (
    <span className="inline-flex flex-col">
      <Button size="sm" variant="outline" onClick={() => add.mutate()} disabled={add.isPending}>
        {add.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
        Watch {name}
      </Button>
      {add.isError && <span className="mt-1 max-w-56 text-xs text-destructive">{errorText(add.error, "Could not add.")}</span>}
    </span>
  );
}

function CompetitorsPage() {
  const qc = useQueryClient();
  const [adding, setAdding] = useState(false);
  const list = useQuery({ queryKey: COMPETITORS_KEY, queryFn: getCompetitors });
  const find = useMutation({ mutationFn: findCompetitors, onSettled: () => qc.invalidateQueries({ queryKey: COMPETITORS_KEY }) });
  const d = list.data;
  const full = !!d && d.data.length >= d.limit;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <Text as="h2">Competitors</Text>
          <Text as="p" className="mt-1 text-muted-foreground">
            What your competitors are shipping, changing and charging, what it means for you, and what to do about it. LeadEcho found them from your site and checks each
            one daily.
          </Text>
        </div>
        {d && (
          <div className="text-right">
            <Button size="sm" variant="outline" onClick={() => setAdding(true)} disabled={adding || full}>
              <Plus className="h-3.5 w-3.5" />
              Add competitor
            </Button>
            <p className="mt-1 text-xs text-muted-foreground">
              Watching {d.data.length} of {d.limit}.
            </p>
          </div>
        )}
      </div>

      {list.isLoading && <p className="text-sm text-muted-foreground">Loading…</p>}
      {list.isError && <p className="text-sm text-destructive">Could not load your competitors. Reload the page to try again.</p>}
      {adding && <AddCompetitor onDone={() => setAdding(false)} />}

      {d && d.data.length === 0 && (
        <div className="rounded-xl border border-dashed border-border p-8 text-center">
          <Text as="p" className="font-medium">{d.set_up ? "No competitors found yet" : "Finding your competitors"}</Text>
          <Text as="p" className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
            {d.set_up
              ? "We couldn't match the competitors from your brand profile to their websites. Add one yourself, or look again after updating your brand profile."
              : "LeadEcho is reading the competitors from your brand profile and looking up where each one publishes. This takes a few minutes."}
          </Text>
          <Button size="sm" variant="outline" className="mt-4" onClick={() => find.mutate()} disabled={find.isPending}>
            {find.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            Look now
          </Button>
          {find.isError && <p className="mt-2 text-xs text-destructive">{errorText(find.error, "Could not look for competitors. Try again.")}</p>}
        </div>
      )}

      {d && d.data.length > 0 && (
        <section className="space-y-3" aria-label="Competitors">
          {d.data.map((c) => (
            <CompetitorCard key={c.id} competitor={c} />
          ))}
        </section>
      )}

      {d && d.unresolved.length > 0 && (
        <section aria-label="Competitors we couldn't find">
          <Text as="h3" className="text-sm font-medium text-muted-foreground">Couldn't find their site</Text>
          <p className="mt-1 text-xs text-muted-foreground">These are named in your brand profile, but we weren't sure which website is theirs, so we are not watching them.</p>
          <ul className="mt-3 space-y-2">
            {d.unresolved.map((n) => (
              <Unresolved key={n} name={n} />
            ))}
          </ul>
        </section>
      )}

      {d && d.suggestions.length > 0 && !full && (
        <section aria-label="Suggested competitors">
          <Text as="h3" className="text-sm font-medium text-muted-foreground">People keep naming these</Text>
          <p className="mt-1 text-xs text-muted-foreground">Products that come up when people in your market describe what they need.</p>
          <div className="mt-3 flex flex-wrap gap-2">
            {d.suggestions.map((n) => (
              <Suggestion key={n} name={n} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
