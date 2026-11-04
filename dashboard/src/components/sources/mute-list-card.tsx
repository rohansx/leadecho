import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellOff, Trash2, Plus } from "lucide-react";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Text } from "@/components/ui/text";
import { createMute, deleteMute, listMutes, MUTES_KEY, type MuteKind } from "@/lib/sources-api";

const KINDS: { value: MuteKind; label: string; placeholder: string }[] = [
  { value: "author", label: "Author", placeholder: "username or @handle" },
  { value: "domain", label: "Domain", placeholder: "example.com (matches subdomains)" },
  { value: "keyword", label: "Keyword", placeholder: "word or phrase" },
  { value: "subreddit", label: "Subreddit", placeholder: "r/subreddit" },
];

/** Mute list: mentions matching any entry are dropped before they reach the inbox. */
export function MuteListCard() {
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<MuteKind>("author");
  const [value, setValue] = useState("");
  const { data: mutes, isLoading } = useQuery({ queryKey: [MUTES_KEY], queryFn: listMutes });
  const invalidate = () => queryClient.invalidateQueries({ queryKey: [MUTES_KEY] });

  const add = useMutation({
    mutationFn: () => createMute(kind, value),
    onSuccess: () => {
      setValue("");
      void invalidate();
    },
  });
  const remove = useMutation({ mutationFn: deleteMute, onSuccess: invalidate });
  const submit = () => value.trim() && add.mutate();

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <BellOff className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>Mute List</CardTitle>
            <CardDescription>
              New mentions from these authors, domains, keywords or subreddits are skipped.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex gap-2">
          <select
            value={kind}
            onChange={(e) => setKind(e.target.value as MuteKind)}
            aria-label="Mute type"
            className="h-9 rounded-lg border border-border bg-card px-2 text-sm"
          >
            {KINDS.map((k) => (
              <option key={k.value} value={k.value}>
                {k.label}
              </option>
            ))}
          </select>
          <Input
            placeholder={KINDS.find((k) => k.value === kind)?.placeholder}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
          />
          <Button onClick={submit} disabled={!value.trim() || add.isPending}>
            <Plus className="h-3.5 w-3.5 mr-1" />
            Mute
          </Button>
        </div>
        {add.isError && <Text as="p" className="text-xs text-destructive">{add.error.message}</Text>}
        {isLoading ? (
          <Text as="p" className="text-sm text-muted-foreground">Loading...</Text>
        ) : !mutes?.length ? (
          <Text as="p" className="text-sm text-muted-foreground">Nothing muted yet.</Text>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {mutes.map((m) => (
              <Badge key={m.id} variant="outline" size="sm" className="text-xs gap-1">
                <span className="text-muted-foreground">{m.kind}:</span> {m.value}
                <button
                  type="button"
                  onClick={() => remove.mutate(m.id)}
                  aria-label={`Unmute ${m.value}`}
                  className="ml-0.5 cursor-pointer hover:text-destructive"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </Badge>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
