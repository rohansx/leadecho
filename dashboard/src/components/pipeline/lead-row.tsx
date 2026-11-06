import { Link } from "@tanstack/react-router";
import { Check, ExternalLink, PenLine } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { Lead, LeadStage } from "@/lib/types";
import { platformTone, stageTone } from "@/lib/tones";
import {
  STAGES,
  intentReason,
  leadIntent,
  leadPostedAt,
  leadScore,
  stageLabel,
  timeAgo,
} from "@/lib/pipeline";

interface LeadRowProps {
  lead: Lead;
  onStage: (id: string, stage: LeadStage) => void;
  pending: boolean;
}

export function LeadRow({ lead, onStage, pending }: LeadRowProps) {
  const m = lead.mention;
  const score = leadScore(lead);
  const who = lead.contact_name ?? lead.username ?? "Unknown";
  const postUrl = m?.url ?? lead.profile_url;

  return (
    <li className="p-4 space-y-2">
      <div className="flex items-center gap-2 min-w-0 text-xs text-muted-foreground">
        <span className="font-medium text-foreground text-sm truncate">{who}</span>
        {lead.platform && (
          <Badge size="sm" className={platformTone(lead.platform)}>
            {lead.platform}
          </Badge>
        )}
        <span className="shrink-0">· {timeAgo(leadPostedAt(lead))}</span>
        <Badge size="sm" className={`ml-auto shrink-0 ${stageTone(lead.stage)}`}>
          {stageLabel(lead.stage)}
        </Badge>
      </div>

      {m ? (
        <p className="text-sm text-foreground-soft line-clamp-2 break-words">
          {m.title && <span className="font-medium text-foreground">{m.title} — </span>}
          {m.snippet}
        </p>
      ) : (
        lead.notes && <p className="text-sm text-foreground-soft line-clamp-2 break-words">{lead.notes}</p>
      )}

      <p className="text-xs text-muted-foreground">
        <span className="text-foreground font-medium">{intentReason(leadIntent(lead))}</span>
        {score != null && <span className="tabular-nums"> · score {score.toFixed(1)}</span>}
      </p>

      <div className="flex flex-wrap items-center gap-2 pt-1">
        {lead.stage === "prospect" && (
          <Button size="sm" variant="accent" disabled={pending} onClick={() => onStage(lead.id, "qualified")}>
            <Check className="h-3.5 w-3.5" />
            Mark replied
          </Button>
        )}
        {lead.mention_id && (
          <Button size="sm" variant="outline" asChild>
            <Link to="/inbox" search={{ id: lead.mention_id, queue: "all" }}>
              <PenLine className="h-3.5 w-3.5" />
              Draft reply
            </Link>
          </Button>
        )}
        {postUrl && (
          <Button size="sm" variant="ghost" asChild>
            <a href={postUrl} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="h-3.5 w-3.5" />
              Open post
            </a>
          </Button>
        )}
        <select
          aria-label="Move to stage"
          className="ml-auto h-8 rounded-lg border border-border bg-card px-2 text-[13px] text-foreground"
          value={lead.stage}
          disabled={pending}
          onChange={(e) => onStage(lead.id, e.target.value as LeadStage)}
        >
          {STAGES.map((s) => (
            <option key={s.key} value={s.key}>
              {s.label}
            </option>
          ))}
        </select>
      </div>
    </li>
  );
}
