import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Text } from "@/components/ui/text";
import { createKeyword } from "@/lib/api";
import { suggestKeywords, type SuggestedKeyword } from "@/lib/keyword-judge-api";

interface GenerateKeywordsDialogProps {
  count: number;
  /** Why the dialog opened, shown under the title. */
  intro?: string;
  onClose: () => void;
}

/** Writes keywords from the product profile; the customer ticks the ones to add. */
export function GenerateKeywordsDialog({ count, intro, onClose }: GenerateKeywordsDialogProps) {
  const qc = useQueryClient();
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [added, setAdded] = useState(0);
  const [addError, setAddError] = useState("");

  const gen = useMutation({
    mutationFn: () => suggestKeywords(count),
    onSuccess: (r) => setPicked(new Set(r.keywords.map((k) => k.term))),
  });
  // Write the first batch as soon as the dialog opens.
  const { mutate: write } = gen;
  useEffect(() => {
    write();
  }, [write]);

  const add = useMutation({
    mutationFn: async (list: SuggestedKeyword[]) => {
      let n = 0;
      let stopped = false;
      for (const k of list) {
        try {
          await createKeyword({ term: k.term, platforms: k.platforms, match_type: "broad" });
          n++;
          setAdded(n);
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          if (msg.includes("already exists")) continue; // added meanwhile: fine
          setAddError(`Added ${n}. Stopped at "${k.term}": ${msg}`);
          stopped = true;
          break;
        }
      }
      return { n, stopped };
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["keywords"] });
      qc.invalidateQueries({ queryKey: ["keyword-review"] });
      qc.invalidateQueries({ queryKey: ["cost-preview"] });
    },
    onSuccess: ({ n, stopped }) => {
      if (n > 0 && !stopped) onClose();
    },
  });

  const list = gen.data?.keywords ?? [];
  const chosen = list.filter((k) => picked.has(k.term));
  const toggle = (term: string) =>
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(term)) next.delete(term);
      else next.add(term);
      return next;
    });

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true" aria-labelledby="gen-kw-title">
      <div className="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-card shadow-lg">
        <div className="flex items-start gap-3 border-b border-border p-4">
          <Sparkles className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
          <div className="min-w-0 flex-1">
            <Text as="h3" id="gen-kw-title" className="text-base font-semibold">Generate more keywords</Text>
            <Text as="p" className="text-sm text-muted-foreground">
              {intro ?? "Written from your product profile, including what makes it different. Untick any you don't want."}
            </Text>
          </div>
          <button type="button" onClick={onClose} aria-label="Close" className="rounded p-1 text-muted-foreground hover:bg-muted">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="flex-1 space-y-2 overflow-y-auto p-4">
          {gen.isPending && <Text as="p" className="py-8 text-center text-sm text-muted-foreground">Writing {count} keywords for your product… this takes about half a minute.</Text>}
          {gen.isError && (
            <div className="py-6 text-center">
              <Text as="p" className="text-sm text-destructive">{gen.error.message}</Text>
              <Button size="sm" variant="outline" className="mt-3" onClick={() => gen.mutate()}>Try again</Button>
            </div>
          )}
          {list.map((k) => (
            <label key={k.term} className="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-3 hover:bg-muted/50">
              <input type="checkbox" className="mt-1" checked={picked.has(k.term)} onChange={() => toggle(k.term)} />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5">
                  <Text as="span" className="text-sm font-medium">{k.term}</Text>
                  {k.angle && <Badge variant="outline" size="sm">{k.angle}</Badge>}
                </div>
                <Text as="p" className="text-xs text-muted-foreground">{k.why}</Text>
                <Text as="p" className="mt-0.5 text-xs text-muted-foreground">{k.platforms.join(" · ")}</Text>
              </div>
            </label>
          ))}
          {addError && <Text as="p" className="text-sm text-destructive">{addError}</Text>}
        </div>

        <div className="flex items-center justify-between gap-2 border-t border-border p-4">
          <Button size="sm" variant="ghost" disabled={gen.isPending || add.isPending} onClick={() => gen.mutate()}>
            Write a new batch
          </Button>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" onClick={onClose}>Not now</Button>
            <Button size="sm" disabled={chosen.length === 0 || add.isPending} onClick={() => { setAddError(""); add.mutate(chosen); }}>
              {add.isPending ? `Adding ${added}/${chosen.length}…` : `Add ${chosen.length} keyword${chosen.length === 1 ? "" : "s"}`}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
