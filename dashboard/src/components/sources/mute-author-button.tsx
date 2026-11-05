import { useMutation, useQueryClient } from "@tanstack/react-query";
import { BellOff, Check } from "lucide-react";
import { createMute, MUTES_KEY } from "@/lib/sources-api";
import type { Mention } from "@/lib/types";

/** Mutes the mention's author so future mentions from them are dropped. */
export function MuteAuthorButton({ mention }: { mention: Mention }) {
  const queryClient = useQueryClient();
  const author = mention.author_username;
  const mutation = useMutation({
    mutationFn: () => createMute("author", author ?? ""),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [MUTES_KEY] }),
  });
  if (!author) return null;
  const done = mutation.isSuccess; // callers key this by mention id, so state resets per mention
  return (
    <button
      type="button"
      onClick={() => mutation.mutate()}
      disabled={mutation.isPending || done}
      className="h-7 w-7 rounded-md border border-border hover:bg-accent disabled:opacity-60 flex items-center justify-center cursor-pointer"
      aria-label={`Mute author @${author}`}
      title={mutation.isError ? mutation.error.message : done ? `@${author} muted` : `Mute @${author}`}
    >
      {done ? <Check className="h-3.5 w-3.5" /> : <BellOff className="h-3.5 w-3.5" />}
    </button>
  );
}
