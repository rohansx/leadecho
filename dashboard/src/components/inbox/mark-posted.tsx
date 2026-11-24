import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { markReplyPosted } from "@/lib/api";
import { QUERY_KEYS } from "@/lib/constants";
import type { Reply } from "@/lib/types";

// LeadEcho never posts: once the customer has posted the reply, they say so
// here (with the comment's link if they have it) so its clicks and sign-ups
// show under "Replies that paid off".
export function MarkPosted({ reply, onPosted }: { reply: Reply; onPosted: (r: Reply) => void }) {
  const queryClient = useQueryClient();
  const [postedURL, setPostedURL] = useState("");
  const mutation = useMutation({
    mutationFn: () => markReplyPosted(reply.id, postedURL.trim()),
    onSuccess: (r) => {
      onPosted(r);
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.replies, reply.mention_id] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.replyPayoff] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.replyAttribution] });
    },
  });

  return (
    <div className="mt-2 flex flex-wrap items-center gap-2">
      <Input
        value={postedURL}
        onChange={(e) => setPostedURL(e.target.value)}
        placeholder="Link to your posted comment (optional)"
        aria-label="Link to your posted comment"
        className="h-8 flex-1 min-w-[200px] text-xs"
      />
      <Button size="sm" variant="outline" onClick={() => mutation.mutate()} disabled={mutation.isPending}>
        {mutation.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
        I posted it
      </Button>
      {mutation.isError && (
        <p className="w-full text-xs text-destructive">{(mutation.error as Error).message}</p>
      )}
    </div>
  );
}
