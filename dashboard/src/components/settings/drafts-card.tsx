import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PenLine } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Text } from "@/components/ui/text";
import { getDraftSettings, saveDraftSettings, type DraftSettings } from "@/lib/api";

// What drafts may say on the customer's behalf. A draft never makes up a
// proof point or a signature: empty here means the draft has none.

const QUERY_KEY = ["draft-settings"];

function DraftsForm({ initial }: { initial: DraftSettings }) {
  const queryClient = useQueryClient();
  const [s, setS] = useState<DraftSettings>(initial);
  const save = useMutation({
    mutationFn: () => saveDraftSettings(s),
    onSuccess: (next) => {
      queryClient.setQueryData(QUERY_KEY, next);
      setS(next);
    },
  });

  return (
    <CardContent className="space-y-4">
      <label className="block space-y-1">
        <Text as="span" className="text-sm font-medium">Proof point</Text>
        <Text as="p" className="text-xs text-muted-foreground">
          One result or customer you can stand behind, in your words. Outreach emails use it once, as written. Leave it empty and no draft will mention a customer or a number.
        </Text>
        <Textarea
          rows={2}
          maxLength={400}
          value={s.proof_point}
          onChange={(e) => setS({ ...s, proof_point: e.target.value })}
          placeholder="Acme cut engineer interview hours by a third in their first month."
        />
      </label>

      <label className="block space-y-1">
        <Text as="span" className="text-sm font-medium">Sign-off</Text>
        <Text as="p" className="text-xs text-muted-foreground">
          Added to the end of every outreach email exactly as written. Leave it empty for no signature.
        </Text>
        <Textarea
          rows={2}
          maxLength={200}
          value={s.sign_off}
          onChange={(e) => setS({ ...s, sign_off: e.target.value })}
          placeholder={"Your name\nYour role, your company"}
        />
      </label>

      <label className="block space-y-1">
        <Text as="span" className="text-sm font-medium">Replies drafted for you each day</Text>
        <Text as="p" className="text-xs text-muted-foreground">
          Posts by buyers scored 7 or higher arrive with a reply already drafted. These drafts are free. Set 0 to draft only when you ask.
        </Text>
        <Input
          type="number"
          min={0}
          max={20}
          className="w-24"
          value={s.auto_draft_daily}
          onChange={(e) => setS({ ...s, auto_draft_daily: Math.min(20, Math.max(0, Number(e.target.value) || 0)) })}
        />
      </label>

      <label className="flex items-start gap-3 cursor-pointer p-3 rounded-lg border border-border hover:bg-accent transition">
        <input
          type="checkbox"
          checked={s.public_reply_links}
          onChange={(e) => setS({ ...s, public_reply_links: e.target.checked })}
          className="w-4 h-4 accent-primary mt-0.5"
        />
        <div>
          <Text as="span" className="text-sm font-medium">Add a tracked link to public replies</Text>
          <Text as="p" className="text-xs text-muted-foreground">
            Off by default: a short link under a comment often reads as spam and can get an account flagged. You can still add a link to a single reply when you approve it.
          </Text>
        </div>
      </label>

      <div className="flex items-center gap-3">
        <Button size="sm" variant="accent" disabled={save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? "Saving..." : "Save"}
        </Button>
        {save.isSuccess && <Text as="span" className="text-xs text-muted-foreground">Saved. New drafts use this.</Text>}
        {save.error && <Text as="span" className="text-xs text-destructive">{save.error.message}</Text>}
      </div>
    </CardContent>
  );
}

export function DraftsCard() {
  const { data, isError } = useQuery({ queryKey: QUERY_KEY, queryFn: getDraftSettings });

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <PenLine className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>Drafts</CardTitle>
            <CardDescription>
              What a draft may say for you: your proof point, your sign-off, and whether replies carry a link.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      {data ? (
        <DraftsForm initial={data} />
      ) : (
        <CardContent>
          <Text as="p" className="text-sm text-muted-foreground">
            {isError ? "Could not load draft settings." : "Loading..."}
          </Text>
        </CardContent>
      )}
    </Card>
  );
}
