import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, ThumbsDown, ThumbsUp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { sendMentionFeedback } from "@/lib/api";
import { QUERY_KEYS } from "@/lib/constants";
import type { Mention } from "@/lib/types";
import { tagTone } from "@/lib/tones";

// scoring: intent tag chips, sentiment marker and 👍/👎 scorer feedback.

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

function lastVote(m: Mention): "up" | "down" | null {
  const last = (m.scoring_metadata?.last_feedback ?? null) as { label?: string } | null;
  return last?.label === "up" || last?.label === "down" ? last.label : null;
}

interface FeedbackButtonsProps {
  mention: Mention;
}

/** 👍/👎 on a mention; each vote becomes a few-shot example for its listener. */
export function FeedbackButtons({ mention }: FeedbackButtonsProps) {
  const queryClient = useQueryClient();
  const [vote, setVote] = useState<"up" | "down" | null>(lastVote(mention));
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: (label: "up" | "down") => sendMentionFeedback(mention.id, label),
    onSuccess: (_, label) => {
      setVote(label);
      setError(null);
      // Feedback can move a mention between queues: refresh the badges too.
      for (const key of [QUERY_KEYS.mentions, QUERY_KEYS.mentionCounts, QUERY_KEYS.mentionQueueCounts, QUERY_KEYS.scoringPrecision])
        queryClient.invalidateQueries({ queryKey: [key] });
    },
    onError: (err) => setError(err instanceof Error ? err.message : "Failed to save feedback"),
  });

  const button = (label: "up" | "down") => {
    const Icon = label === "up" ? ThumbsUp : ThumbsDown;
    const active = vote === label;
    return (
      <Button
        size="sm"
        variant={active ? "default" : "outline"}
        onClick={() => mutation.mutate(label)}
        disabled={mutation.isPending}
        aria-pressed={active}
        aria-label={label === "up" ? "Good lead" : "Not relevant"}
        title={label === "up" ? "Good lead — score more like this" : "Not relevant — score fewer like this"}
      >
        {mutation.isPending && mutation.variables === label ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <Icon className="h-3.5 w-3.5" />
        )}
      </Button>
    );
  };

  return (
    <div className="flex items-center gap-1.5">
      <span className="text-xs text-muted-foreground">Good lead?</span>
      {button("up")}
      {button("down")}
      {error && <span className="text-xs text-destructive">{error}</span>}
    </div>
  );
}
