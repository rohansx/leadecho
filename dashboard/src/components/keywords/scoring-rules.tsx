import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { setScoringRules, type ScoringRules as Rules } from "@/lib/keyword-judge-api";

interface ScoringRulesProps {
  rules: Rules;
}

const MAX_RULES = 10;

/**
 * The scoring rules the keyword judge learned for this workspace. The
 * scorer follows them on every post; the customer can remove or add rules
 * (a removed rule is never proposed again).
 */
export function ScoringRules({ rules }: ScoringRulesProps) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState("");
  const save = useMutation({
    mutationFn: setScoringRules,
    onSuccess: () => {
      setDraft("");
      qc.invalidateQueries({ queryKey: ["keyword-review"] });
    },
  });
  const texts = rules.rules.map((r) => r.text);
  const test = rules.last_test;

  return (
    <div className="space-y-2">
      <div>
        <Text as="p" className="text-sm font-medium">Scoring rules</Text>
        <Text as="p" className="text-xs text-muted-foreground">
          Learned from the weekly review and tested before use. Every new post is scored with them. Remove any you disagree with.
        </Text>
      </div>
      {texts.length === 0 ? (
        <Text as="p" className="text-xs text-muted-foreground">No rules yet. The next review will propose some.</Text>
      ) : (
        <ul className="space-y-1.5">
          {rules.rules.map((r) => (
            <li key={r.id} className="flex items-start gap-2 rounded-lg border border-border p-2">
              <Text as="span" className="min-w-0 flex-1 text-sm">{r.text}</Text>
              {r.source === "customer" && <Text as="span" className="shrink-0 text-[11px] text-muted-foreground">yours</Text>}
              <button
                type="button"
                aria-label={`Remove rule: ${r.text}`}
                disabled={save.isPending}
                onClick={() => save.mutate(texts.filter((t) => t !== r.text))}
                className="shrink-0 rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {texts.length < MAX_RULES && (
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (draft.trim()) save.mutate([...texts, draft.trim()]);
          }}
        >
          <Input value={draft} onChange={(e) => setDraft(e.target.value)} maxLength={240} placeholder="Add a rule, e.g. Posts from students are not leads" className="min-w-0 flex-1" />
          <Button type="submit" size="sm" variant="outline" disabled={!draft.trim() || save.isPending}>
            <Plus className="h-3.5 w-3.5" />
          </Button>
        </form>
      )}
      {save.isError && <Text as="p" className="text-xs text-destructive">{save.error.message}</Text>}
      {test && <Text as="p" className="text-xs text-muted-foreground">Last proposal ({new Date(test.at).toLocaleDateString()}): {test.note}.</Text>}
    </div>
  );
}
