import { ExternalLink, Loader2, ThumbsDown, ThumbsUp } from "lucide-react";
import { Textarea } from "@/components/ui/textarea";
import { ChipList } from "./chip-list";
import { goalLabel, type ListenerState, type Proposal, type Sample } from "./types";

interface ListenerCardProps {
  state: ListenerState;
  onProposalChange: (p: Proposal) => void;
  onToggle: () => void;
  onFeedback: (s: Sample, label: "up" | "down") => void;
}

/** One goal's listener: live query counts, sample posts, editable intent. */
export function ListenerCard({ state, onProposalChange, onToggle, onFeedback }: ListenerCardProps) {
  const p = state.proposal;
  const total = state.tests.reduce((n, t) => n + t.count, 0);

  return (
    <div className={`border border-border rounded-lg p-4 ${state.included ? "" : "opacity-60"}`}>
      <div className="flex items-center justify-between mb-2">
        <div>
          <p className="text-xs font-semibold text-primary uppercase tracking-wider">{goalLabel(state.goal)}</p>
          <p className="font-semibold">{p?.name ?? "Building listener..."}</p>
        </div>
        {p ? (
          <label className="flex items-center gap-1.5 text-sm cursor-pointer">
            <input type="checkbox" checked={state.included} onChange={onToggle} /> Create
          </label>
        ) : (
          <Loader2 className="size-4 animate-spin text-primary" />
        )}
      </div>

      {!p && state.steps.length > 0 && <p className="text-xs text-muted-foreground mb-2">{state.steps[state.steps.length - 1]}</p>}

      {state.tests.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mb-3">
          {state.tests.map((t) => (
            <span
              key={`${t.platform}-${t.keyword}`}
              title={t.error ?? ""}
              className={`px-2 py-0.5 text-xs border border-border rounded ${t.count > 0 ? "bg-muted" : "text-muted-foreground"}`}
            >
              {t.keyword} · {t.platform} · {t.error ? "n/a" : t.count}
            </span>
          ))}
          <span className="text-xs text-muted-foreground self-center">{total} posts in the last 7 days</span>
        </div>
      )}

      {state.samples.length > 0 && (
        <div className="space-y-2 mb-3">
          {state.samples.map((s) => (
            <SampleRow key={s.url} sample={s} label={state.feedback[s.url]} onFeedback={(l) => onFeedback(s, l)} />
          ))}
          <p className="text-xs text-muted-foreground">👍 / 👎 teach the scorer what a good match looks like.</p>
        </div>
      )}
      {p && state.samples.length === 0 && (
        <p className="text-xs text-muted-foreground mb-3">No recent public posts yet: the 7-day backfill and scheduled scans keep looking.</p>
      )}

      {p && state.included && (
        <div className="space-y-2">
          <ChipList items={p.keywords} onChange={(keywords) => onProposalChange({ ...p, keywords })} placeholder="Add keyword..." />
          <label className="block text-xs text-muted-foreground uppercase tracking-wider font-medium">Looks for</label>
          <Textarea className="min-h-14" value={p.looks_for} onChange={(e) => onProposalChange({ ...p, looks_for: e.target.value })} />
          <label className="block text-xs text-muted-foreground uppercase tracking-wider font-medium">Skips</label>
          <Textarea className="min-h-14" value={p.skips} onChange={(e) => onProposalChange({ ...p, skips: e.target.value })} />
        </div>
      )}
    </div>
  );
}

function SampleRow({ sample, label, onFeedback }: { sample: Sample; label?: "up" | "down"; onFeedback: (l: "up" | "down") => void }) {
  return (
    <div className="flex gap-2 items-start p-2 bg-muted/50 border border-border rounded">
      <div className="flex-1 min-w-0">
        <a href={sample.url} target="_blank" rel="noopener noreferrer" className="text-sm font-medium hover:underline inline-flex items-center gap-1">
          <span className="truncate">{sample.title || sample.text.slice(0, 80)}</span>
          <ExternalLink className="size-3 shrink-0" />
        </a>
        <p className="text-xs text-muted-foreground line-clamp-2">{sample.text}</p>
        <p className="text-[11px] text-muted-foreground">
          {sample.platform}
          {sample.author ? ` · ${sample.author}` : ""}
        </p>
      </div>
      <div className="flex gap-1">
        <button
          type="button"
          aria-label="Good match"
          onClick={() => onFeedback("up")}
          className={`p-1 rounded border border-border cursor-pointer ${label === "up" ? "bg-primary text-primary-foreground" : "hover:bg-muted"}`}
        >
          <ThumbsUp className="size-3.5" />
        </button>
        <button
          type="button"
          aria-label="Bad match"
          onClick={() => onFeedback("down")}
          className={`p-1 rounded border border-border cursor-pointer ${label === "down" ? "bg-destructive text-destructive-foreground" : "hover:bg-muted"}`}
        >
          <ThumbsDown className="size-3.5" />
        </button>
      </div>
    </div>
  );
}
