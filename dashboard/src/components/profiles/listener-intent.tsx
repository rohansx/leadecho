import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { updateProfile } from "@/lib/api";
import type { MonitoringProfile } from "@/lib/types";

// scoring: inline editor for a listener's plain-English "Looks for" / "Skips".

const MAX_LEN = 2000;
const textareaClass =
  "w-full min-h-[64px] rounded-lg border border-border bg-background px-3 py-2 text-sm font-[family-name:var(--font-sans)] shadow-xs focus:outline-none focus:ring-2 focus:ring-ring";

interface ListenerIntentProps {
  profile: MonitoringProfile;
}

export function ListenerIntent({ profile }: ListenerIntentProps) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [looksFor, setLooksFor] = useState(profile.looks_for ?? "");
  const [skips, setSkips] = useState(profile.skips ?? "");

  const mutation = useMutation({
    mutationFn: () => updateProfile(profile.id, { looks_for: looksFor, skips }),
    onSuccess: () => {
      setEditing(false);
      queryClient.invalidateQueries({ queryKey: ["profiles"] });
    },
  });

  if (!editing) {
    return (
      <div className="mt-2 space-y-1 text-xs">
        <div className="flex gap-2">
          <span className="text-muted-foreground w-16 shrink-0">Looks for</span>
          <span className="text-foreground-soft whitespace-pre-wrap">{profile.looks_for || "—"}</span>
        </div>
        <div className="flex gap-2">
          <span className="text-muted-foreground w-16 shrink-0">Skips</span>
          <span className="text-foreground-soft whitespace-pre-wrap">{profile.skips || "—"}</span>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-xs"
          onClick={() => {
            setLooksFor(profile.looks_for ?? "");
            setSkips(profile.skips ?? "");
            setEditing(true);
          }}
        >
          <Pencil className="h-3 w-3 mr-1" />
          Edit Looks for / Skips
        </Button>
      </div>
    );
  }

  return (
    <div className="mt-3 space-y-3">
      <div className="space-y-1.5">
        <Text as="p" className="text-xs font-medium">Looks for</Text>
        <textarea
          className={textareaClass}
          maxLength={MAX_LEN}
          placeholder="e.g. Teams asking for a cheaper uptime monitor, or frustrated with Pingdom alerts"
          value={looksFor}
          onChange={(e) => setLooksFor(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Text as="p" className="text-xs font-medium">Skips</Text>
        <textarea
          className={textareaClass}
          maxLength={MAX_LEN}
          placeholder="e.g. Job posts, students' homework, vendors promoting their own tools"
          value={skips}
          onChange={(e) => setSkips(e.target.value)}
        />
        <Text as="p" className="text-xs text-muted-foreground">
          The scorer uses these next to the pain-point match. Posts that match Skips are scored down.
        </Text>
      </div>
      {mutation.isError && (
        <Text as="p" className="text-xs text-destructive">
          {mutation.error instanceof Error ? mutation.error.message : "Failed to save"}
        </Text>
      )}
      <div className="flex gap-2">
        <Button size="sm" onClick={() => mutation.mutate()} disabled={mutation.isPending}>
          {mutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" />}
          Save
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setEditing(false)} disabled={mutation.isPending}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
