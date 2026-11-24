import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Loader2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { sendMentionFeedback } from "@/lib/api";
import { QUERY_KEYS } from "@/lib/constants";
import type { Mention } from "@/lib/types";
import { tagTone, voteTone } from "@/lib/tones";
import { lastVote, VOTE_TEXT, voteAriaLabel, type Vote } from "@/lib/lead-vote";
import { cn } from "@/lib/utils";

// scoring: intent tag chips, sentiment marker and the Lead / Not a lead vote.

// Coloured by what the tag means for a seller (see tagTone); noise stays neutral.

const sentimentTone: Record<string, string> = {
  positive: "text-green-600 dark:text-green-400",
  neutral: "text-muted-foreground",
  negative: "text-red-600 dark:text-red-400",
};

export function tagLabel(tag: string): string {
  return tag.replace(/_/g, " ");
}

interface TagChipsProps {
  tags?: string[];
  sentiment?: string | null;
  max?: number;
}

export function TagChips({ tags, sentiment, max = 3 }: TagChipsProps) {
  const list = tags ?? [];
  if (list.length === 0 && !sentiment) return null;
  const shown = list.slice(0, max);
  return (
    <div className="flex items-center gap-1 flex-wrap">
      {shown.map((t) => (
        <span
          key={t}
          className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${tagTone(t)}`}
        >
          {tagLabel(t)}
        </span>
      ))}
      {list.length > shown.length && (
        <span className="text-[10px] text-muted-foreground">+{list.length - shown.length}</span>
      )}
      {sentiment && (
        <span className="text-[10px] text-muted-foreground">
          <span className={sentimentTone[sentiment] ?? ""}>●</span> {sentiment}
        </span>
      )}
    </div>
  );
}

export const INBOX_KEYS = [QUERY_KEYS.mentions, QUERY_KEYS.mentionCounts, QUERY_KEYS.mentionQueueCounts, QUERY_KEYS.scoringPrecision];

/** Mutations on one lead share this scope, so they run one after another. */
export const leadScope = (id: string | undefined) => (id ? { id: `lead:${id}` } : undefined);

/**
 * Sends a vote; stays pending until the lists have refetched, so the row shows
 * the new state. Pass the lead's id to queue votes on it behind each other.
 */
export function useVote(mentionId?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    scope: leadScope(mentionId),
    mutationFn: ({ id, label, note }: { id: string; label: Vote; note?: string }) => sendMentionFeedback(id, label, note),
    onSuccess: () => Promise.all(INBOX_KEYS.map((key) => queryClient.invalidateQueries({ queryKey: [key] }))),
  });
}

const VOTE_HINT: Record<Vote, { title: string; key: string }> = {
  up: { title: "Lead: score more like this", key: "y" },
  down: { title: "Not a lead: score fewer like this", key: "n" },
};

/**
 * "Lead" / "Not a lead" on a mention; each vote becomes a few-shot example for
 * its listener. `shortcuts` shows the y / n keys, which act on the open lead.
 */
export function VoteButtons({ mention, shortcuts = false }: { mention: Mention; shortcuts?: boolean }) {
  const vote = useVote(mention.id);
  const pending = vote.isPending ? vote.variables.label : null;
  const current = pending ?? lastVote(mention);

  const button = (label: Vote) => {
    const Icon = label === "up" ? Check : X;
    const active = current === label;
    const hint = VOTE_HINT[label];
    return (
      <Button
        size="sm"
        variant="outline"
        className={cn(active && `${voteTone(label)} border-transparent`, vote.isPending && "opacity-60 cursor-wait")}
        onClick={(e) => {
          e.stopPropagation();
          if (!vote.isPending) vote.mutate({ id: mention.id, label });
        }}
        // aria-disabled, not disabled: a disabled button drops keyboard focus.
        aria-disabled={vote.isPending}
        aria-pressed={active}
        aria-label={shortcuts ? undefined : voteAriaLabel(label, mention)}
        title={shortcuts ? `${hint.title} (${hint.key})` : hint.title}
        aria-keyshortcuts={shortcuts ? hint.key : undefined}
      >
        {pending === label ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Icon className="h-3.5 w-3.5" />}
        {VOTE_TEXT[label]}
      </Button>
    );
  };

  return (
    <div className="flex items-center gap-1.5">
      {button("up")}
      {button("down")}
      {vote.isError && (
        <span role="alert" className="text-xs text-destructive">
          {vote.error.message || "Failed to save"}
        </span>
      )}
    </div>
  );
}
