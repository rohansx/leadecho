import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Sparkles, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { getKeywordReview } from "@/lib/keyword-judge-api";
import { topUpCount } from "@/lib/keyword-top-up";
import { GenerateKeywordsDialog } from "./generate-keywords-dialog";

const SNOOZE_KEY = "leadecho.keyword-top-up.snoozed-until";
const SNOOZE_MS = 3 * 24 * 60 * 60 * 1000;

function snoozed(): boolean {
  try {
    return Date.now() < Number(localStorage.getItem(SNOOZE_KEY) ?? 0);
  } catch {
    return false;
  }
}

function snooze(): void {
  try {
    localStorage.setItem(SNOOZE_KEY, String(Date.now() + SNOOZE_MS));
  } catch {
    // private window: it shows again next visit
  }
}

/**
 * Pro workspaces under 30 active keywords get a pop-up offering to write the
 * rest. "Not now" hides it for three days.
 */
export function KeywordTopUp() {
  const [hidden, setHidden] = useState(snoozed);
  const [open, setOpen] = useState(false);
  const { data } = useQuery({ queryKey: ["keyword-review"], queryFn: getKeywordReview, staleTime: 5 * 60_000, retry: false });

  if (!data?.top_up) return null;
  const count = topUpCount(data);
  if (count <= 0) return null;
  const close = () => {
    snooze();
    setHidden(true);
    setOpen(false);
  };
  if (open) {
    return (
      <GenerateKeywordsDialog
        count={count}
        intro={`You track ${data.active_keywords} keywords; Pro workspaces find the most buyers with ${data.target} or more. Untick any you don't want.`}
        onClose={close}
      />
    );
  }
  if (hidden) return null;
  return (
    <div className="fixed bottom-4 right-4 z-40 w-[22rem] max-w-[calc(100vw-2rem)] rounded-xl border border-border bg-card p-4 shadow-lg" role="dialog" aria-labelledby="kw-topup-title">
      <div className="flex items-start gap-3">
        <Sparkles className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <Text as="p" id="kw-topup-title" className="text-sm font-semibold">Find more buyers with more keywords</Text>
          <Text as="p" className="mt-1 text-xs text-muted-foreground">
            You track {data.active_keywords} of the {data.target}+ keywords that work best on Pro. We can write {count} more from your product profile, and you pick which to add.
          </Text>
          <div className="mt-3 flex gap-2">
            <Button size="sm" variant="accent" onClick={() => setOpen(true)}>Generate {count} keywords</Button>
            <Button size="sm" variant="ghost" onClick={close}>Not now</Button>
          </div>
        </div>
        <button type="button" onClick={close} aria-label="Close" className="rounded p-1 text-muted-foreground hover:bg-muted">
          <X className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
